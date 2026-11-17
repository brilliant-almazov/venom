package runartifacts

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ovh/venom"
)

// put writes the file rel under root with its folder.
func put(t *testing.T, root, rel, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(root, rel)), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, rel), []byte(body), 0o644))
}

// planTree is a scenario tree of two paths: accounts with an http call, a user executor that reads a table and a queue
// publish; ledger with a testsuite of caseFilesFrom testcases. The lib folder holds the user executor.
func planTree(t *testing.T, root string) {
	put(t, root, "scenarios/lib/dbcheck.yml", "executor: dbcheck\ninput:\n  table: none\nsteps:\n"+
		"  - type: sql\n    driver: postgres\n    dsn: postgres://u:secret@db:5432/billing\n    commands:\n"+
		"      - \"SELECT id FROM {{.input.table}} WHERE id = 1\"\n")
	put(t, root, "scenarios/accounts/SB-01-accounts.yml", "name: SB-01 accounts\ntestcases:\n"+
		"  - name: SB-01-01 create\n    steps:\n"+
		"      - type: http\n        method: POST\n        url: https://gw.stand/v1/accounts\n"+
		"        assertions:\n          - result.statuscode ShouldEqual {{.code}}\n"+
		"      - type: dbcheck\n        table: account\n"+
		"  - name: SB-01-02 publish\n    steps:\n"+
		"      - type: amqp\n        addrs: amqp://mq:5672\n        clientType: producer\n        exchange: billing\n"+
		"        routingKey: account.created\n")

	var b strings.Builder
	b.WriteString("name: SB-02 ledger\ntestcases:\n")

	for i := 1; i <= caseFilesFrom; i++ {
		fmt.Fprintf(&b, "  - name: SB-02-%02d entry %d\n    steps:\n      - type: dbcheck\n        table: ledger\n", i, i)
	}

	put(t, root, "scenarios/ledger/SB-02-ledger.yml", b.String())
}

// A scheme with no run: every testsuite of the tree as written, the steps of the user executors under their step, the
// bars of the last run that has each testcase, «no run» of a testsuite no run has, a large testsuite split by testcase.
func TestWritePlan(t *testing.T) {
	root := t.TempDir()
	planTree(t, root)
	runs := filepath.Join(root, "cases", "billing", "runs")
	put(t, runs, "2026-11/17-9/"+ReportFile, "# Run report\n\n- verdict: ❌ 0/2\n"+
		"- testcases: 2 testcases · ✅ 0 · ❌ 2 · ⏸ 0 · ok 0% · partial 0% · failed 100%\n"+
		"\n## Testsuites\n\n| testsuite | protocols | testcases | verdict | bar | fix in |\n| -- | -- | -- | -- | -- | -- |\n"+
		"| SB-01 accounts | [x/](x) | 2 testcases · ✅ 0 · ❌ 2 · ⏸ 0 · ok 0% · partial 0% · failed 100% | ❌ 0/2 | x | — |\n"+
		"\n## Schemes\n\n| slice | scheme | verdict | share |\n| -- | -- | -- | -- |\n"+
		"| case | [SB-01-01 create](x) | ❌ 500 | ✅ 0% · ❌ 50% · ⏸ 50% |\n"+
		"| case | [SB-01-02 publish](x) | ❌ | ✅ 0% · ❌ 100% · ⏸ 0% |\n")
	put(t, runs, "2026-11/17-13/"+ReportFile, "# Run report\n\n- verdict: ❌ 1/2\n"+
		"- testcases: 2 testcases · ✅ 1 · ❌ 1 · ⏸ 0 · ok 50% · partial 0% · failed 50%\n"+
		"\n## Testsuites\n\n| testsuite | protocols | testcases | verdict | bar | fix in |\n| -- | -- | -- | -- | -- | -- |\n"+
		"| SB-01 accounts | [x/](x) | 2 testcases · ✅ 1 · ❌ 1 · ⏸ 0 · ok 50% · partial 0% · failed 50% | ❌ 1/2 | x | — |\n"+
		"\n## Schemes\n\n| slice | scheme | verdict | share |\n| -- | -- | -- | -- |\n"+
		"| case | [SB-01-01 create](x) | ✅ 201 | ✅ 100% · ❌ 0% · ⏸ 0% |\n"+
		"| case | [SB-01-02 publish](x) | ❌ | ✅ 0% · ❌ 100% · ⏸ 0% |\n")
	put(t, runs, "2026-11/17-13/"+SchemesDir+"/"+ZoomSystemFile, "# System of the run\n\n"+headProcesses+"\n\n"+
		"| process | direction | point | testcases | verdict | bar | failed spans | cases | history |\n"+
		"| -- | -- | -- | -- | -- | -- | -- | -- | -- |\n"+
		"| cgw-gateway | in | http POST /v1/accounts | 1 | ✅ 1/1 | [x] 100 % | 0 | SB-01-01 | 100 % |\n"+
		"| cgw-billing-server | out | table account written | 1 | ✅ 1/1 | [x] 100 % | 2 | SB-01-01 | 100 % |\n")

	v := venom.New()
	v.LibDir = filepath.Join(root, "scenarios", "lib")
	ctx := context.Background()
	require.NoError(t, v.ReadPlan(ctx, []string{filepath.Join(root, "scenarios")}))
	require.Len(t, v.Tests.TestSuites, 2)

	out := filepath.Join(root, "plan")
	require.NoError(t, WritePlan(ctx, v, out, runs))

	read := func(rel string) string {
		body, err := os.ReadFile(filepath.Join(out, SchemesDir, rel))
		require.NoError(t, err, rel)

		return string(body)
	}

	run := read(ZoomRunFile)
	assert.Contains(t, run, "# Scheme\n")
	assert.Contains(t, run, "scheme  [██░░░░░░░░░░░░░░░░░░] 10 %  ❌ 1/10")
	assert.Contains(t, run, "testsuite SB-01 accounts  [██████████░░░░░░░░░░] 50 %  ❌ 1/2")
	assert.Contains(t, run, "testsuite SB-02 ledger  no run  ⏸ 0/8")
	assert.Contains(t, run, "2026-11/17-13  [██████████░░░░░░░░░░] 50 %  ❌ 1/2")
	assert.NotContains(t, run, "this run")
	assert.NotContains(t, run, ReportFile)

	system := read(ZoomSystemFile)
	assert.Contains(t, system, "# System of the scheme")
	assert.Contains(t, system, "► http gw.stand  [████████████████████] 100 %  ✅ 1/1")
	assert.Contains(t, system, "► sql postgres billing")
	assert.Contains(t, system, "table account")
	assert.Contains(t, system, "table ledger  no run")
	assert.Contains(t, system, "► amqp amqp://mq:5672  [░░░░░░░░░░░░░░░░░░░░] 0 %  ❌ 0/1  ◄ red")
	assert.Contains(t, system, "exchange billing")
	assert.Contains(t, system, "cgw-gateway  [████████████████████] 100 %  ✅ 1/1")
	assert.Contains(t, system, "| cgw-billing-server | out | table account written | 1 | ✅ 1/1 | [████████████████████] 100 % | 2 | SB-01-01 |")
	assert.NotContains(t, system, "secret")
	assert.NotContains(t, system, SystemFile)

	suite := read(filepath.Join(ZoomSuitesDir, "sb-01-accounts.md"))
	assert.Contains(t, suite, "result.statuscode ShouldEqual {{.code}}")
	assert.Contains(t, suite, "→ 1 \n")
	assert.Contains(t, suite, "SELECT id FROM account WHERE id = 1")
	assert.Contains(t, suite, "routing key account.created")
	assert.NotContains(t, suite, "- protocol:")

	ledger := read(filepath.Join(ZoomSuitesDir, "sb-02-ledger.md"))
	assert.Contains(t, ledger, "## History\n\nno run\n")
	assert.Contains(t, ledger, "(sb-02-ledger/01-sb-02-01-entry-1.md)")

	entry := read(filepath.Join(ZoomSuitesDir, "sb-02-ledger", "01-sb-02-01-entry-1.md"))
	assert.Contains(t, entry, "# Testcase SB-02-01 entry 1")
	assert.Contains(t, entry, "- verdict: ⏸ no run")
	assert.Contains(t, entry, "SELECT id FROM ledger WHERE id = 1")

	assert.Contains(t, read(filepath.Join(ZoomSuitesDir, "sb-02-ledger", IndexFile)), "| [SB-02-08 entry 8](08-sb-02-08-entry-8.md) |")
	assert.Contains(t, read(filepath.Join(ZoomPathsDir, IndexFile)), "| [ledger](ledger.md) | ")
	assert.Contains(t, read(filepath.Join(ZoomSuitesDir, IndexFile)), "| [SB-01 accounts](sb-01-accounts.md) | accounts | 2 |")
	assert.NotContains(t, read(IndexFile), "Schemes of the run")
}
