# p1-01 — `venom run <suites> --testcase <name>`: run chosen testcases of chosen testsuites

## Iteration budget

| Turns | Context lift, tokens | Execution, tokens | Total, tokens | Lift share | Money, $ | Source of the number |
| -- | -- | -- | -- | -- | -- | -- |
| up to 30 turns | 5k tokens | 110k tokens | 115k tokens | 4.3 % | ≈ $16 | estimate: costs/COSTS.md, lead 0343be1c 2026-11-17 — 161k paid for a one-flag feature with its design; ready text here removes the design, ≈ 70 %; money at the COSTS.md ratio $0.14 per 1k paid |

## What to do

Repository `/Users/brilliantov_anton/Source/github.com/brilliant-almazov/venom`, branch
`feat/env-set-and-testcase-selection`. Six files: two new, four changed.

### 1. New file `select.go`

```go
package venom

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gosimple/slug"
)

// ErrNoSuchTestCase is returned when a name given by --testcase matches no testcase of the testsuites read.
var ErrNoSuchTestCase = errors.New("no such testcase")

// selectTestCases keeps, in every testsuite, the testcases named in v.TestCaseNames, then the setup… and teardown…
// testcases of that testsuite, then every earlier testcase whose exported vars a kept testcase reads
// ({{.<testcase>.<var>}}), transitively. A testsuite holding none of the names is dropped from the run. A name
// matching no testcase of any testsuite is an error naming it and the testsuites read. No name keeps everything.
// A name matches a testcase name as written or as its slug.
func (v *Venom) selectTestCases() error {
	if len(v.TestCaseNames) == 0 {
		return nil
	}

	found := make(map[string]bool, len(v.TestCaseNames))
	kept := make([]TestSuite, 0, len(v.Tests.TestSuites))
	files := make([]string, 0, len(v.Tests.TestSuites))

	for i := range v.Tests.TestSuites {
		ts := v.Tests.TestSuites[i]
		files = append(files, ts.Filepath)

		cases := selectedTestCases(&ts, v.TestCaseNames, found)
		if len(cases) == 0 {
			continue
		}

		ts.TestCases = cases
		kept = append(kept, ts)
	}

	for _, name := range v.TestCaseNames {
		if !found[name] {
			return fmt.Errorf("--testcase %q: %w in %s", name, ErrNoSuchTestCase, strings.Join(files, ", "))
		}
	}

	v.Tests.TestSuites = kept

	return nil
}

// selectedTestCases returns the testcases of ts kept for names, in file order; none when no name matches in ts.
// Every name matched is set in found.
func selectedTestCases(ts *TestSuite, names []string, found map[string]bool) []TestCase {
	keep := make([]bool, len(ts.TestCases))
	named := false

	for i := range ts.TestCases {
		for _, name := range names {
			if testCaseNamed(ts.TestCases[i].Name, name) {
				keep[i], named = true, true
				found[name] = true
			}
		}
	}

	if !named {
		return nil
	}

	for i := range ts.TestCases {
		if isStandCase(ts, &ts.TestCases[i]) {
			keep[i] = true
		}
	}

	keepDependencies(ts, keep)

	cases := make([]TestCase, 0, len(ts.TestCases))
	for i := range ts.TestCases {
		if keep[i] {
			cases = append(cases, ts.TestCases[i])
		}
	}

	return cases
}

// keepDependencies keeps every earlier testcase whose exported vars a kept testcase reads, until nothing changes.
// Exported vars are named "<testcase slug>.<var>", so a read holds ".<testcase slug>.".
func keepDependencies(ts *TestSuite, keep []bool) {
	for changed := true; changed; {
		changed = false

		for i := range ts.TestCases {
			if !keep[i] {
				continue
			}

			text := testCaseText(&ts.TestCases[i])

			for j := 0; j < i; j++ {
				name := slug.Make(ts.TestCases[j].Name)
				if !keep[j] && name != "" && strings.Contains(text, "."+name+".") {
					keep[j], changed = true, true
				}
			}
		}
	}
}

// testCaseNamed reports whether a testcase name matches a name of --testcase, as written or as slug.
func testCaseNamed(testCase, name string) bool {
	return testCase == name || slug.Make(testCase) == slug.Make(name)
}
```

### 2. New file `select_test.go`

```go
package venom

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func selectCase(name string, steps ...string) TestCase {
	var tc TestCase
	tc.Name = name
	for _, s := range steps {
		tc.RawTestSteps = append(tc.RawTestSteps, json.RawMessage(s))
	}
	return tc
}

func selectRun(names []string, suites ...TestSuite) (*Venom, error) {
	v := New()
	v.TestCaseNames = names
	v.Tests.TestSuites = suites
	return v, v.selectTestCases()
}

func keptNames(v *Venom) map[string][]string {
	kept := map[string][]string{}
	for _, ts := range v.Tests.TestSuites {
		for _, tc := range ts.TestCases {
			kept[ts.Filepath] = append(kept[ts.Filepath], tc.Name)
		}
	}
	return kept
}

func selectFixture() []TestSuite {
	return []TestSuite{
		{Filepath: "a.yml", TestCases: []TestCase{
			selectCase("setup stand"),
			selectCase("C1 login", `{"type":"http","vars":{"token":{"from":"result.body"}}}`),
			selectCase("C2 profile", `{"type":"http","url":"{{.c1-login.token}}"}`),
			selectCase("C3 other"),
			selectCase("teardown stand"),
		}},
		{Filepath: "b.yml", TestCases: []TestCase{selectCase("C4 billing")}},
	}
}

func TestSelectNoNameKeepsEverything(t *testing.T) {
	v, err := selectRun(nil, selectFixture()...)
	require.NoError(t, err)
	require.Equal(t, map[string][]string{
		"a.yml": {"setup stand", "C1 login", "C2 profile", "C3 other", "teardown stand"},
		"b.yml": {"C4 billing"},
	}, keptNames(v))
}

func TestSelectKeepsNamedSetupTeardownAndDependencies(t *testing.T) {
	v, err := selectRun([]string{"C2 profile"}, selectFixture()...)
	require.NoError(t, err)
	require.Equal(t, map[string][]string{
		"a.yml": {"setup stand", "C1 login", "C2 profile", "teardown stand"},
	}, keptNames(v))
}

func TestSelectBySlugAcrossSuites(t *testing.T) {
	v, err := selectRun([]string{"c3-other", "C4 billing"}, selectFixture()...)
	require.NoError(t, err)
	require.Equal(t, map[string][]string{
		"a.yml": {"setup stand", "C3 other", "teardown stand"},
		"b.yml": {"C4 billing"},
	}, keptNames(v))
}

func TestSelectKeepsDependenciesTransitively(t *testing.T) {
	v, err := selectRun([]string{"C3"}, TestSuite{Filepath: "c.yml", TestCases: []TestCase{
		selectCase("C1"),
		selectCase("C2", `{"type":"http","url":"{{.c1.id}}"}`),
		selectCase("C3", `{"type":"http","url":"{{.c2.id}}"}`),
		selectCase("C4"),
	}})
	require.NoError(t, err)
	require.Equal(t, map[string][]string{"c.yml": {"C1", "C2", "C3"}}, keptNames(v))
}

func TestSelectUnknownNameFails(t *testing.T) {
	_, err := selectRun([]string{"C2 profile", "C9 missing"}, selectFixture()...)
	require.ErrorIs(t, err, ErrNoSuchTestCase)
	require.ErrorContains(t, err, `--testcase "C9 missing"`)
	require.ErrorContains(t, err, "a.yml, b.yml")
}
```

### 3. `venom.go` — the field of the names

Before:

```go
	// AsWritten reads the testsuites with no var filled, each {{.x}} kept as the scenario writes it (plan.go).
	AsWritten bool
}
```

After:

```go
	// AsWritten reads the testsuites with no var filled, each {{.x}} kept as the scenario writes it (plan.go).
	AsWritten bool
	// TestCaseNames, when set, keeps only these testcases of the testsuites read, with what they need (select.go).
	TestCaseNames []string
}
```

### 4. `process.go` — selection right after the testsuites are read

Before:

```go
	if err := v.readFiles(ctx, filesPath); err != nil {
		return err
	}

	err = v.registerUserExecutors(ctx)
```

After:

```go
	if err := v.readFiles(ctx, filesPath); err != nil {
		return err
	}

	if err := v.selectTestCases(); err != nil {
		return err
	}

	err = v.registerUserExecutors(ctx)
```

### 5. `cmd/venom/run/cmd.go` — the flag `--testcase`

Four replacements, each block exactly as it stands in the file.

Before:

```go
	runDir         string
	htmlReport     bool
```

After:

```go
	runDir         string
	testCaseNames  []string
	htmlReport     bool
```

Before:

```go
	runDirFlag         *string
	stopOnFailureFlag  *bool
```

After:

```go
	runDirFlag         *string
	testCaseNamesFlag  *[]string
	stopOnFailureFlag  *bool
```

Before:

```go
	variablesFlag = Cmd.Flags().StringArray("var", nil, "--var cds='cds -f config.json' --var cds2='cds -f config.json'")
```

After:

```go
	variablesFlag = Cmd.Flags().StringArray("var", nil, "--var cds='cds -f config.json' --var cds2='cds -f config.json'")
	testCaseNamesFlag = Cmd.Flags().StringArray("testcase", nil, "--testcase <name>: run only this testcase of the testsuites given, by its name; "+
		"repeat the flag for several. The setup… and teardown… testcases of its testsuite and every earlier testcase whose vars it reads run too; "+
		"a testsuite holding none of the names does not run. A name matching no testcase exits 2")
```

Before:

```go
	case "var":
		if variablesFlag != nil {
			for _, varFlag := range *variablesFlag {
				variables = mergeVariables(varFlag, variables)
			}
		}
	}
}
```

After:

```go
	case "var":
		if variablesFlag != nil {
			for _, varFlag := range *variablesFlag {
				variables = mergeVariables(varFlag, variables)
			}
		}
	case "testcase":
		if testCaseNamesFlag != nil {
			testCaseNames = *testCaseNamesFlag
		}
	}
}
```

Before:

```go
		v.DryRun = dryRun
```

After:

```go
		v.DryRun = dryRun
		v.TestCaseNames = testCaseNames
```

### 6. `README-LOCAL.md` — section 32 before `## Build`

Before:

````markdown
`TestSchemesDrawnFromTheRun`, `TestNoEmptyFolders`), `runartifacts/run_test.go` (`TestRunOfEveryOutcome`: a whole run
of pass, Should, Must and skip), `TestUserExecutorCallKeptOnFailure`, `TestProtocolDBReadsInPlaceOfTrace`.

## Build
````

After:

````markdown
`TestSchemesDrawnFromTheRun`, `TestNoEmptyFolders`), `runartifacts/run_test.go` (`TestRunOfEveryOutcome`: a whole run
of pass, Should, Must and skip), `TestUserExecutorCallKeptOnFailure`, `TestProtocolDBReadsInPlaceOfTrace`.

## 32. Chosen testcases of chosen testsuites: `--testcase <name>`

A run names only what it runs: a testsuite, several testsuites, or testcases.

```bash
venom run cases/billing/plans.yml                                   # one testsuite
venom run cases/billing/plans.yml cases/billing/invoices.yml        # several
venom run cases/billing/ --testcase "BIL-12 plan upgrade"           # one testcase
venom run cases/billing/ --testcase BIL-12-plan-upgrade --testcase "BIL-14 refund"
```

| rule | |
| -- | -- |
| name | the `name:` of the testcase as written, or its slug |
| brought along | the `setup…` and `teardown…` testcases of its testsuite, every earlier testcase whose vars it reads (`{{.<testcase>.<var>}}`), transitively; a testsuite under `scenarios/preflight/` whole |
| other testsuites | a testsuite holding none of the names does not run |
| unknown name | exit 2: `--testcase "<name>": no such testcase in <testsuite files>` |

Point tests: `select_test.go`.

## Build
````

## Canon

| ID | Requirement | Skill or hook | How to check |
| -- | -- | -- | -- |
| K1 | one source per value: the stand-case rule is `isStandCase` (stand_red.go), the testcase text is `testCaseText` (parallel.go); no copy | `cgw-no-duplication` | `select.go` calls both, defines neither |
| K2 | no dead code, no unused argument | `cgw-no-unused-code` | every function of `select.go` has a caller |
| K3 | the error carries a sentinel, checked with `require.ErrorIs` | `cgw-failure-taxonomy` | `TestSelectUnknownNameFails` |
| K4 | no new ENV key: the flag is a flag only | `env-key-names-are-owners.cjs` | `cmd.go` diff adds no `os.Getenv` and no key to `initFromEnv` |
| K5 | nothing runs locally: no tests, build, lint | `local-test-runs-forbidden.cjs` | no command in this iteration but `## Status` |
| K6 | no AI attribution anywhere | `no-ai-attribution-anywhere.cjs` | the hook passed every write |
| K7 | absolute paths in Bash | `bash-absolute-paths.cjs` | the hook passed every command |

## CHECK

| ID | Claim | Proof | State |
| -- | -- | -- | -- |
| C1 | no `--testcase` runs every testcase of every testsuite (positive) | `TestSelectNoNameKeepsEverything` green in CI of the PR | ⬜ |
| C2 | a named testcase runs with setup, teardown and the testcase whose vars it reads, and nothing else (positive) | `TestSelectKeepsNamedSetupTeardownAndDependencies`, `TestSelectKeepsDependenciesTransitively` green in CI | ⬜ |
| C3 | a testsuite with none of the names does not run (negative) | `TestSelectKeepsNamedSetupTeardownAndDependencies`: `b.yml` absent | ⬜ |
| C4 | an unknown name stops the run before any step, exit 2 (negative) | `TestSelectUnknownNameFails` green in CI; Parse returns the error, `cmd.go` `folder.fail("setup failed", err)` exits 2 | ⬜ |

## Done when

- `select.go` and `select_test.go` lie at the root of the repository with the text of §1 and §2;
- `venom.go`, `process.go`, `cmd/venom/run/cmd.go`, `README-LOCAL.md` hold every "After" block of §3–§6 in place of its
  "Before" block.

## Status

```
node ~/.claude/hooks/lib/prompt-status.js /Users/brilliantov_anton/Source/github.com/brilliant-almazov/venom/prompts/in_progress/00-scenario-env-and-testcase-selection/p1-01-in_progress-testcase-selection.md done
```
