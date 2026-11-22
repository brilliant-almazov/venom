# p9-01 — closing: one runner of the stand utility held by a test, CI of the PR read and its red fixed

## Iteration budget

| Turns | Context lift, tokens | Execution, tokens | Total, tokens | Lift share | Money, $ | Source of the number |
| -- | -- | -- | -- | -- | -- | -- |
| up to 40 turns | 6k tokens | 150k tokens | 156k tokens | 4.0 % | ≈ $22 | measure: costs/COSTS.md, lead a7ebce91 2026-11-18 — 148k paid for a CI-red fix session of this repository; money at the COSTS.md ratio $0.14 per 1k paid |

## What to do

Repository `/Users/brilliantov_anton/Source/github.com/brilliant-almazov/venom`, branch
`feat/env-set-and-testcase-selection`, PR of that branch into `master`. p1-01 and p1-02 are done.

### 1. New file `executors/cli/one_runner_test.go`

The forbidding test: a second copy of the process start in the package (an `env_set` that starts `bin/cli` by itself)
turns it red.

```go
package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestOneRunnerOfTheStandUtility: cli, seed, stand_status and env_set start the utility through run only.
func TestOneRunnerOfTheStandUtility(t *testing.T) {
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)
	starts := map[string]int{}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		require.NoError(t, err)
		if n := strings.Count(string(b), "exec.CommandContext("); n > 0 {
			starts[f] = n
		}
	}
	require.Equal(t, map[string]int{"cli.go": 1}, starts)
}
```

### 2. CI of the PR

1. `gh pr checks feat/env-set-and-testcase-selection --repo brilliant-almazov/venom` — the job `cgw-qa` (workflow
   `.github/workflows/cgw-qa.yml`: `docker build -f docker/Dockerfile.cgw-qa --target test .`, then the image build).
2. Green on the HEAD sha: §3.
3. Red: `gh run view <run id> --repo brilliant-almazov/venom --log-failed`. Each failed test or compile error names a
   file of p1-01, p1-02 or §1; the fix goes into that file only, keeping the behaviour of the tables below. Each red run
   and its fix are one comment in the PR: `gh pr comment feat/env-set-and-testcase-selection --repo
   brilliant-almazov/venom --body "<run id>: <failed test or error, verbatim> — <file:line> fixed: <what changed>"`.

Behaviour that a fix keeps:

| Item | Behaviour |
| -- | -- |
| `--testcase` | named testcases plus `setup…`/`teardown…` of their testsuite plus earlier testcases whose vars they read, transitively; testsuite with none dropped; unknown name → `ErrNoSuchTestCase`, exit 2 |
| `env_set` | `bin/cli var set <service> KEY=VAL… --restart --env <env>` with keys sorted; env defaults to the var `stand` |
| reset | `bin/cli var reset <service> --restart --env <env>`, registered before the set, once per service, run last-first after the testcase on any outcome on a non-cancelable context, and on SIGINT/SIGTERM |
| failed reset | step `env reset <service>` fails with `base state not restored: <service>: rc=<code>`; every later testcase skipped with `===== base state not restored: the run stopped =====` |
| parallel | a testsuite with `env_set` runs in order and alone |

### 3. Check of the requirements against the code

The set has no analytics document: the requirements are rows T3, T4 and T7 of the plan of the head
(cgw-architecture, `planning/in_progress/56-*/INDEX.md`, local environment, 2026-11-22). Fill this table in the PR
comment that closes the iteration, one row per requirement, each with `file:line` of the code:

| Requirement | Code | Matches / differs |
| -- | -- | -- |
| T7 — a run names only testsuites, several testsuites or testcases | `select.go` `selectTestCases`; `cmd/venom/run/cmd.go` flag `testcase` | |
| T3 — a testcase sets an ENV value and the service restarts, a step of the testcase | `executors/cli/env_set.go` `Run` | |
| T4 — after the testcase the base state of the scenario is back, failed testcase too | `env_state.go` `restoreEnv`; `process_testcase.go` `runTestCase` defer; `cmd/venom/run/run_folder.go` `onSignal` | |

## Canon

| ID | Requirement | Skill or hook | How to check |
| -- | -- | -- | -- |
| K1 | the forbidding test holds the one runner | `cgw-no-duplication` | `TestOneRunnerOfTheStandUtility` green in CI |
| K2 | gates run only in CI, never locally | `local-test-runs-forbidden.cjs` | no `go test`, `go build`, `go vet`, `golangci-lint`, `make` here |
| K3 | every red run and its fix are a PR comment | `cgw-pr-sync` | the comments of the PR |
| K4 | no AI attribution in any comment | `no-ai-attribution-anywhere.cjs` | the hook passed every `gh pr comment` |
| K5 | `done` only on a green CI of the HEAD sha | `prompt-done-needs-green-ci.cjs` | the hook passed `## Status` |

## CHECK

| ID | Claim | Proof | State |
| -- | -- | -- | -- |
| C1 | one start of the utility in `executors/cli` (positive) | `TestOneRunnerOfTheStandUtility` green in CI | ⬜ |
| C2 | a second `exec.CommandContext(` in the package turns CI red (negative) | the map of the test then holds two files or a count of 2 | ⬜ |
| C3 | CI of the HEAD sha is green | `gh run list --branch feat/env-set-and-testcase-selection --limit 5 --json headSha,conclusion` | ⬜ |

## Done when

- `executors/cli/one_runner_test.go` lies at its path with the text of §1;
- every run of the HEAD sha of the branch is `success`, each red run before it is a PR comment;
- the table of §3 is a PR comment with every row filled.

## Status

```
node ~/.claude/hooks/lib/prompt-status.js /Users/brilliantov_anton/Source/github.com/brilliant-almazov/venom/prompts/in_progress/00-scenario-env-and-testcase-selection/p9-01-in_progress-closing-ci-and-forbidding-test.md done
```
