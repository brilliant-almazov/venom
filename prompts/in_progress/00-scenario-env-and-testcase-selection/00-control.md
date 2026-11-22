# 00 — scenario ENV and testcase selection: control prompt

## Why the set

Owner, 2026-11-22, relayed by cgw-architecture-c8 (plan of the head: cgw-architecture `planning/in_progress/56-*/INDEX.md`,
local environment, rows T3, T4, T7):

- T7: a run names only a testsuite, several testsuites or testcases. Today `venom run` takes testsuite paths only
  (`cmd/venom/run/cmd.go:415-419`, `path = args[0:]`); no flag of `cmd/venom/run/cmd.go:60-82` selects a testcase.
- T3: a testcase sets an ENV value of a service and the service restarts, as a step of the testcase. Today the step can
  call `bin/cli` through `type: cli` (`executors/cli/cli.go:122`), but nothing brings the value back.
- T4: after the testcase the stand is back at the base state of its scenario, the testcase failed too. Today a testcase
  ends in `runTestCase` (`process_testcase.go:136-153`) with no step after its last one, and SIGINT with no `--run-dir`
  kills the run with no handler (`cmd/venom/run/run_folder.go:45-48`).

## Inventory

| File | Function | Line | Kind |
| -- | -- | -- | -- |
| `process.go` | `Parse` — testsuites read, then parsed | 82-97 | selection goes in after `readFiles` |
| `process_files.go` | `readFiles` — `TestSuite.TestCases` built | 153-240 | source of the testcases |
| `stand_red.go` | `isStandCase` — setup…, teardown…, scenarios/preflight/ | 30-35 | reused, no copy |
| `parallel.go` | `testCaseText`, `parallelFor`, `parallelSuitesFor` | 21, 50, 119 | reused; env_set forces 1 |
| `process_testcase.go` | `runTestCase` | 136-153 | reset deferred here |
| `process_testsuite.go` | `runTestCases`, `markRemainingSkipped` | 112-145 | dirty stand stops the run |
| `run_artifacts_view.go` | `protocolStepViews` | 139-150 | reset steps numbered after the scenario |
| `executors/cli/cli.go` | `Executor.Run` | 122-166 | process start moved to `run`, shared with env_set |
| `executors/registry.go` | `Registry` | 37-63 | env_set registered |
| `cmd/venom/run/run_folder.go` | `onSignal` | 43-63 | resets on SIGINT/SIGTERM |
| `cmd/venom/run/cmd.go` | flags, `RunE` | 25-82, 426-443 | `--testcase`, folder gets `v` |
| `README-LOCAL.md` | sections 1-31 | 604-751 | sections 32, 33 before `## Build` |

## Where it lands

`venom run <suite>… [--testcase <name>]…` runs exactly what is named. A step `type: env_set` sets values of one service
through `bin/cli var set … --restart`, and Venom brings it back with `bin/cli var reset … --restart` after the testcase on
any outcome and on SIGINT/SIGTERM; a failed reset stops the run. The set is done when p9-01 is green in CI.

## Order and dependencies

| # | Iteration | Prompt file | Depends on | Budget | Model |
| -- | -- | -- | -- | -- | -- |
| 1 | `--testcase` selection | `p1-01-done-testcase-selection.md` | — | 115k tokens | sonnet |
| 2 | `env_set` and guaranteed reset | `p1-02-done-env-set-and-guaranteed-reset.md` | p1-01 (same `venom.go`, `README-LOCAL.md` blocks) | 209k tokens | sonnet |
| 3 | closing: forbidding test, CI read, red fixed | `p9-01-done-closing-ci-and-forbidding-test.md` | p1-01, p1-02 | 156k tokens | opus |
| 4 | service run: writing the set | this folder | — | 420k tokens | opus |
| 5 | service run: audit and economics report in `retro/` | `retro/` | p9-01 | 60k tokens | opus |

Budget of the set: 960k tokens planned. Sources: costs/COSTS.md (venom lead sessions 2026-11-16..18, 52k-468k paid
each); row 4 — this lead session so far, by the cost journal hook.

## Launch batches

| Batch | Iterations | State |
| -- | -- | -- |
| A | p1-01, then p1-02, one executor, in order | ⬜ |
| B | p9-01 | ⬜ |

## Test layout

Tests sit next to the code as the repository keeps them: `select_test.go`, `env_reset_test.go` at the root,
`executors/cli/env_set_test.go`, `executors/cli/one_runner_test.go`.

## Style common to every iteration

Go of the repository: tabs, gofmt, lines under 200 (`.golangci.yml` `lll`), English comments, testify `require` for
errors, no `time.Sleep` in tests.

## Documentation common to every iteration

Each behaviour change is a numbered section of `README-LOCAL.md` before `## Build`, with its point tests named.

## Tests common to every iteration

Two cases per claim, positive and negative, in the CHECK table of the iteration; proof is the test green in CI of the
PR, never a local run.

## Closing

p9-01 reads CI of the PR (`.github/workflows/cgw-qa.yml`), fixes the red, comments every red run in the PR, and fills
the check of T3, T4, T7 against the code. No analytics document: the requirements are rows T3, T4, T7 of the plan of
the head; p9-01 checks them row by row.

## Facts of the set

| Fact | Value |
| -- | -- |
| stand utility argv, agreed with cgw-env-cli-5b 2026-11-22 | `bin/cli var set <service> KEY1=VAL1 KEY2=VAL2 --restart --env <env>`; `bin/cli var reset <service> --restart --env <env>` |
| rc of the utility | 0 healthy, 10 not up or not healthy, 2 no record or bad argument, 4 docker down, 1 declaration error |
| reset | idempotent: rc 0 and no restart when the values are already base; brings the scenario values back, not the profile ones |
| env of the suites, cgw-qa-scenarios-b3 2026-11-22 | the var `stand`, passed by every runner as `--var stand=<env>` |
| step form, agreed with cgw-qa-scenarios-b3 | `type: env_set`, `service`, `values`, `env` (default `{{.stand}}`); the suites write no reset step |
| testcase name of `--testcase` | the full `name:` of the testcase, starting with its case id, or its slug |
| stand testcase rule | `isStandCase`, `stand_red.go:30-35` |
| process start of the utility | `executors/cli/cli.go:136-147` before p1-02, `run` after |
| Go | `go 1.27.0` (`go.mod:3`): `context.WithoutCancel`, `slices.Sorted`, `maps.Keys` available |

## Set decisions — final

| # | Fork | Decision | Argument | Iteration |
| -- | -- | -- | -- | -- |
| 1 | language of the set in an English-only repository | English, headings `## Iteration budget`, `## Canon`, `## Status` | decision of the head cgw-architecture-c8, 2026-11-22, option (a) | all |
| 2 | testcase selection form | flag `--testcase <name>`, repeatable, next to testsuite paths | owner 2026-11-22 (T7) via the head; form accepted by the head and cgw-qa-scenarios-b3 | p1-01 |
| 3 | what a chosen testcase brings | setup…/teardown… of its testsuite and the earlier testcases whose vars it reads, transitively | a testcase reading `{{.<testcase>.<var>}}` does not run without its producer (`parallel.go:34-47`); stand testcases by `isStandCase` (`stand_red.go:30-35`) | p1-01 |
| 4 | argv of set and reset | `var set … --restart --env`, `var reset … --restart --env` | cgw-env-cli-5b 2026-11-22: the `env` group is taken by environments; ENV of a record is `var` (its docs/use/01-cli.md:186) | p1-02 |
| 5 | step name | `env_set` | accepted by the head and cgw-qa-scenarios-b3, 2026-11-22 | p1-02 |
| 6 | reset guarantee | registered before the set, run in a defer of `runTestCase` on a non-cancelable context, and on SIGINT/SIGTERM | owner 2026-11-22 (T4): back to base after the testcase; a set that restarted the service and failed health still changed the values | p1-02 |
| 7 | failed reset | testcase fails, the rest of the run is skipped | a stand off its base state makes every later result unreliable; owner T4 | p1-02 |
| 8 | parallel runs with env_set | testsuite in order and alone | the restarted service is shared by every testcase of the run | p1-02 |
