# brilliant-almazov/venom: what differs from ovh/venom v1.3.0

Branch `feat/parallel-subsecond`, cut from tag `v1.3.0`. Every YAML that runs on v1.3.0 runs here
the same way; the two additions below change behaviour only when they are used.

## 1. Sub-second waits: `delay` and `timeout`

The per-step wait fields of v1.3.0 are `delay` (pause between two retries) and `timeout`
(step timeout). Both were whole seconds. Both now accept:

| Value | Means |
|---|---|
| `delay: 2`, `delay: "2"` | 2 seconds, as before |
| `delay: 0.2`, `delay: "0.2"` | 200 ms |
| `delay: 200ms`, `timeout: 1.5s`, `timeout: 1m` | a Go duration |

`retry` stays a count. A value that is neither a number nor a duration is an error, as a non-integer
was before. `Delay()` / `Timeout()` of `ExecutorRunner` still return whole seconds;
`DelayDuration()` / `TimeoutDuration()` return the exact value. A whole-second timeout still reports
`Timeout after N second(s)`; a sub-second one reports the duration (`Timeout after 200ms`).

## 2. Parallel testcases: `--parallel=N`

`venom run --parallel=N` runs up to N testcases of one testsuite at once. Default 1 is the v1.3.0
sequential run. Also `VENOM_PARALLEL=N` and `parallel: N` in the venom config file.

- Testsuites run one after another unless `--parallel-suites` is set (section 3).
- Each testcase keeps its own vars and results; results stay at their index, so JUnit/JSON/YAML/TAP
  reports list testcases in file order.
- Console output of a testcase is buffered and printed whole, in testcase order. `venom.log`
  lines are written whole by logrus.
- **Dependent vars rule.** A testsuite runs sequentially when a testcase's steps or `skip`
  conditions contain `.<name of an earlier testcase>.`, that is a reference such as
  `{{.create-user.id}}` to a var exported by an earlier testcase of the same suite. A false match
  only costs parallelism.
- `--stop-on-failure` forces a sequential run of the suite.
- Steps that share an external resource (a file, a database row, a port) are not detected; put
  such testcases in their own suite or keep `--parallel=1`.

## 3. Parallel testsuites: `--parallel-suites=N`

`venom run --parallel-suites=N` runs up to N testsuites (files) of one run at once. Default 1 is the
v1.3.0 sequential run. Also `VENOM_PARALLEL_SUITES=N` and `parallel_suites: N` in the venom config file.

- Each testsuite keeps its own vars, results and output files; results stay at their index, so
  the reports list testsuites in file order, and the run counters are added in file order.
- Console output of a testsuite is buffered and printed whole, in file order.
- **Shared state rule.** The whole run stays sequential with `--stop-on-failure`, with `-vvv`
  (CPU profiling is process-wide), or when a testsuite holds `.<testcase name of another
  testsuite>.`, that is a reference such as `{{.create-user.id}}` to a var named after another
  suite's testcase. v1.3.0 does not pass vars between testsuites, so such a reference never
  resolves; the check is conservative.
- **Effective concurrency.** `--parallel-suites=S --parallel=P` runs up to S testsuites at once and
  up to P testcases inside each of them: at most S x P testcases at once. Each of the two limits
  falls back to 1 on its own rule (above and section 2).
- The first error that stops a testsuite is returned in file order, as a sequential run returns it.

## 4. Concurrent calls of one grpc step: `parallel: N`

A `grpc` step with `parallel: N` (N > 1) sends the same request N times at once: N goroutines wait behind one
barrier and each runs the step without `parallel`. The result keeps the first call as the plain fields (`code`,
`status`, `systemout`, `trace_id`, …) and adds, in call order:

- `codes` — the status names joined by a comma, `OK,OK,OK,OK,OK`;
- `bodies` — every answer on one line (JSON compacted; an error call gives its status message), joined by a newline;
- `distinct_bodies` — how many different answers there were;
- `trace_ids` — the trace id of every call (with `trace: auto` each call has its own);
- `calls` — every call result.

Point test: `executors/grpc/parallel_test.go`.

## 5. A body cut short on purpose: http `truncate_body: N`

An `http` step with `truncate_body: N` (N > 0) sends the request over its own connection with a `Content-Length` N
bytes larger than the body, writes the body and closes the write side: the server meets the end of the body before
the promised length — an error of reading the body. The answer is read and returned as usual (`statuscode`, `body`,
`headers`). Proxy and unix socket settings are not used on this path. Point test: `executors/http/truncate_test.go`.

## 6. A traced http call: `trace`, `tempo_url`, `trace_dir`, `trace_wait`

The same fields as the grpc executor. `trace: auto` (a new W3C trace id) or a 32-hex id is sent as the `traceparent`
header and returned as `result.trace_id`; with `tempo_url` and `trace_dir` the trace is saved to
`<trace_dir>/<trace_id>.json` after the answer (`result.trace_saved`, `result.trace_file`). A `traceparent` in
`headers` wins over the generated one. Point test: `executors/http/trace_test.go`.

## 7. Raw output of a read-back step: sql, exec and s3 `output_file`

`output_file: <relative path>` on a `sql`, `exec` or `s3` step writes the raw output — `out` of the last query, the s3 result as JSON, or
`systemout` then `systemerr` — to `<venom output dir>/<path>` (the testsuite folder when the run has no output dir) and
returns the absolute path as `result.output_file`. The path stays inside the output dir. A read-back step that makes no
traced call leaves this file as the record of its verdict. Point tests: `executors/evidence/evidence_test.go`,
`executors/exec/output_file_test.go`.

## 8. Concurrent calls of one http step: `parallel: N`

An `http` step with `parallel: N` (N > 1, a number or its text, so a user executor can template it) sends the same
request N times at once behind one barrier, as the grpc step of section 4. The result keeps the first call as the
plain fields (`statuscode`, `body`, `bodyjson`, `headers`, `trace_id`) and adds, in call order: `codes` (HTTP statuses
joined by a comma), `bodies` (each body on one line, joined by a newline), `distinct_bodies`, `trace_ids`, `calls`.
Point test: `executors/http/parallel_test.go`.

## 9. Hashids of a number: template function `hashids`

`{{hashids <salt> <min_length> <n>}}` encodes the number n by the hashids algorithm (default alphabet) on the salt
and minimum length, as an API that hashes its ids and idempotency keys on a salt it gives out. Stock Venom has no such
function, so a suite reads the salt by an `http` step, keeps it by `vars: from: result.bodyjson.<path>` and writes
`X-Idempotency-Key: '{{hashids .salt 8 .n}}'` with n from vars or an API answer instead of a script. An empty salt gives an empty string.
Point test: `interpolate/hashids_test.go`.

## 10. A protocol per testcase: `--protocol-dir`

`--protocol-dir <dir>` (env `VENOM_PROTOCOL_DIR`, `.venomrc` key `protocol_dir`) writes `<dir>/<testsuite>/<testcase>.md`
for every testcase, whatever its outcome: passed, failed on a `Should…` assertion, failed on a `Must…` assertion, or
skipped by `--stop-on-failure`. Sections:

- `## Expected behaviour`: the `happens`, `expect`, `right` and `norm` info lines;
- `## What is wrong`: the `wrong` info lines;
- `## State before`: every precondition read of the testcase with its request and answer;
- `## Data presence`: one row per info line «precondition: <entity> · <key> present|absent — …» (the one form of
  scenarios/TEMPLATE.*.yml) of a precondition step, the read through the API that proves the data is there or is not:
  entity · key · request (the executor and its method and url, or url, service and method, or its declared fields) ·
  present/absent with ✅ when that read and the checks after it, up to the next call, held and «❌ not confirmed» when one
  failed · trace id · whether the tested call sent the key's value, with its request. No such line: «no precondition»;
- `## Request`: the `point` info line, the `call:` line rendered from the step itself (executor, method and url, or rpc,
  or the declared fields; never a script), the step, and what the
  call sent: method, full url, headers (Authorization masked, X-Idempotency-Key shown) and the body, JSON one key per
  line; a step that is not http shows the call fields of its interpolated step. The call is the step with the `call:`
  info line when the testcase has one, else a step with a trace id and a code: the last failed call, else the last call;
- `## Response`: the code, the trace id (result.trace_id, meta.trace_id of the body or the X-Trace-Id header), the
  X-Trace-Id, Traceparent, X-Request-Id and X-Build headers and the body verbatim, and the `fact` info lines;
- `## State after`: every later call of the testcase, the read-back after a write, with its request and answer;
- `## Trace`: the span tree `<trace_dir>/<trace_id>.md` of that call, looked up in every `trace_dir` and `traces` the
  steps were given;
- `## Verdict`: ✅, ❌ with every failed assertion of every step, or ⏭ with the skip reason.

A testcase whose call returned a trace id while no tree `<dir>/<trace_id>.md` was saved fails (`traceGate`). The fork is
English only: its code, tests, docs and the protocols it writes.

The file name is the testcase name as written, row id first (`GW-B0005-…`, `S-001-…`), with spaces, `/`, `:` and the
characters a file name cannot carry turned into `-`, at most 120 bytes; the testsuite folder is its slug.

Info lines are `key: value` and come from the steps (`info:` of a step) and from the testcase itself: a testcase takes
`info:` next to `name` and `steps` (`types.go`, `TestCaseInput.Info`).

Stock Venom has none of this. It has no testcase-level `info` key: `TestCaseInput` in v1.3.0 carries only `name`,
`vars`, `skip`, `steps` and `id`. It also cannot write such a file from a suite step:
- a failed `Must…` assertion and `--stop-on-failure` end the testcase before any later step (`process_testcase.go`,
  `return` after «At least one required assertion failed»);
- a failed step sets none of its `vars:` (`continue` before `processVariableAssignments`);
- no executor writes a file.
The protocol is therefore written by the runner after the status of the testcase is known (`process_testsuite.go`,
`runOneTestCase`). Point test: `protocol_test.go`, covering green, a failed Must, --stop-on-failure, and a failed Should
with skipped vars, the template shape (a call with vars only, then a failing check step), the presence table (data present, data absent, failed precondition, no precondition) and the file name.

## 11. A value checked with no shell: `type: check`

```yaml
- type: check
  value: '{{.call_code}}'
  assertions:
  - result.value ShouldEqual 200
  - result.valuejson.items.items0.id MustEqual 7
```

`value` (string, interpolated) is returned as `result.value`, and parsed as `result.valuejson` when it is JSON. The
step makes no call and returns no trace_id; a failed Must assertion stops the testcase. Point test:
`executors/check/check_test.go`.

## 12. The stand utility with no script: `type: cli`, `seed`, `stand_status`

| field | type | default | |
| -- | -- | -- | -- |
| `bin` | string | `bin/cli` | the utility, relative to the testsuite workdir; started directly, never through a shell |
| `action` | string | — | `cli` only: its words, `service on`, `health`, `var list` |
| `args` | list of strings | `[]` | after the action |
| `env` | string | — | required, passed as `--env <env>` |
| `block` | string | — | `seed` only, required, passed as `--block <block>` |
| `output_file` | string | — | systemout, then systemerr, under the venom output dir |

argv is `<action words> <args> [--block <block>] --env <env>`; `seed` is the action `seed`, `stand_status` the action
`status`. Result: `result.systemout`, `result.systemoutjson` (when systemout is JSON, the read-back of a seed),
`result.systemerr`, `result.code` (exit code, default assertion `result.code ShouldEqual 0`), `result.output_file`. No
trace_id. Point test: `executors/cli/cli_test.go`.

## 13. The span tree of a saved trace: `tempo` writes `<trace_id>.md`

The `tempo` executor (`trace_id`, `tempo_url`, `trace_dir`, `wait` seconds, default 20) and the traced http and grpc
calls save `<trace_dir>/<trace_id>.json` and, next to it, the span tree `<trace_dir>/<trace_id>.md`: one span per line
under its parent, `- <service> · <span> · <ms> ms · ✅`, `❌ <status message>` on an error status. The protocol prints
this file under Trace and the trace gate fails a call whose tree is missing. Result: `result.trace_id`,
`result.trace_saved` (`yes`, `late`, `no`), `result.trace_file`, `result.tree_file`, `result.spans`. Point test:
`executors/tempo/tempo_test.go`.

## Build

```bash
docker build -f docker/Dockerfile.cgw-qa --target test .                    # package tests
docker build -f docker/Dockerfile.cgw-qa -t cgw-qa-venom:v1.3.0-local .     # image
docker run --rm cgw-qa-venom:v1.3.0-local version
```

Pinned by commit, without a checkout:

```bash
docker build -f docker/Dockerfile.cgw-qa -t cgw-qa-venom:<sha> \
  https://github.com/brilliant-almazov/venom.git#<full sha>
```

The final stage is `alpine:3.21` (the base of `ovhcom/venom:v1.3.0`) plus `docker-cli`.
CI: `.github/workflows/cgw-qa.yml`, `runs-on: self-hosted`.
