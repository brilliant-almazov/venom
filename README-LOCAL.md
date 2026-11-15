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

`{{traceint <trace_id>}}` is the first 12 hex digits of a 32-hex trace id as an int64 (at most 2^48-1): the n of
hashids and the operation_id of a write, keyed on the trace of the call just before it,
`'{{hashids .salt 8 (traceint .prev_trace)}}'`. An empty or non-hex trace id is an error, never 0. Point test:
`interpolate/traceint_test.go`.

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
`result.trace_saved` (`yes`, `late`, `no`), `result.trace_file`, `result.tree_file`, `result.spans`, `result.mismatch`.

The `tempo` step checks the saved trace with no shell: `contains`, `not_contains` (lists of substrings) and `matches`
(list of Go regexps) are matched over one line per span, `<service> · <span> · <status> · <key>=<value> · …`
(status `✅` or `❌ <message>`, attributes sorted by key). Each `contains` item must be in some span line, no
`not_contains` item in any, each `matches` regexp must match some line; `result.mismatch` names each item that does
not hold, one per line, and is empty when all hold or none is given (`the trace is not saved, nothing to check` when
the trace is not saved). Default assertions: `result.trace_saved ShouldEqual yes`, `result.mismatch ShouldBeEmpty`.
Point test: `executors/tempo/tempo_test.go`.

## 14. A wait with no shell: `type: wait`

```yaml
- type: wait
  seconds: '{{.absence_window_s}}'
  reason: the absence window before the read
```

`seconds` is a number or an interpolated string, fractions allowed, not negative; `reason` is optional. Result:
`result.seconds`, `result.reason`. No call, no trace_id, no default assertion; the wait ends early when the testcase
context is done. The protocol prints each wait under Request as `- wait: step N, <seconds> s, <reason>`. Point tests:
`executors/wait/wait_test.go`, `TestProtocolWaitLine`.

## 15. Declarative only: a testsuite or user executor with exec or script is rejected at load

A testsuite whose step has `type: exec` or a `script:` body, and a user executor file (lib) with such a step, fail
`venom run` at load, before any testcase runs, with every such step named: `<file>: testcase "<name>" step <n>: type:
exec, script:` and `<file>: executor "<name>" step <n>: …`; a lib file that does not parse before interpolation is read
line by line and named `<file>:<line>`. There is no allowlist (owner 2026-11-15: declarative only, with no
exception). The `Expected behaviour` section of a protocol is built only from the info lines `happens`, `expect`,
`right` and `norm`; the text of a `call:` info line is printed nowhere. Point tests: `declarative_test.go`,
`TestProtocolExpectedFromInfoKeysOnly`.

## 16. Request inputs built from a spec: grpc `request_from_reflection`, openapi plan, cli health `baseline`

grpc: `request_from_reflection: true` builds the request message of `method` from server reflection (or `protoset`,
`proto_files`): every singular field outside a oneof that is not proto3 optional; a value from `fill: {<field path>:
<value>}` (dot-separated path, proto or JSON field name) converted to the field type — 64-bit integers, enums and
strings as given, numbers, bools, JSON for a message or a repeated field — else the first enum value that is not
`*_UNSPECIFIED`, `1` for numbers (`"1"` for 64-bit), `"qa"` for strings, `false` for bools; nested messages the same
way, Timestamp `2026-01-01T00:00:00Z`, Duration `1s`, wrappers by their value, Struct, Any, FieldMask and Empty left
out. `data` or `data_json`, when given, are merged over it. Every grpc call returns the message sent as
`result.request`. Point test: `executors/grpc/request_from_reflection_test.go`.

openapi: `fill: [names]` non-required body fields filled by example or enum; `omit: [names]` body fields and query
params left out; `static: {name: value}` body fields set as given; `rows: '<JSON of a list answer>'` gives a field or
parameter with no example or enum the same field (name compared without case and `_`) of the first item of
`data.items`, `data` or `items`; `query_fields: [names]` optional query params added, each by its minimum, then its
value. Result keys stay `body` and `query`. Point test: `executors/openapi/openapi_test.go`.

cli: an action `health` returns `result.red`, its ✗ rows of systemout and systemerr with the leading spaces and the
trailing duration removed, sorted, and `result.red_new`, the red rows not red in `baseline: '<systemout of an earlier
health step>'`; one row per line, empty when none. Point test: `executors/cli/cli_test.go`.

## 17. Every match of a regexp: check `find_all`, `replace`, `duplicates`, `join`

```yaml
- type: check
  value: '{{.rules_body}}'
  find_all: '\{[^{}]*"origin":"own"[^{}]*\}'
  replace: [',"group":"[^"]*"', '']
  join: ','
  assertions:
  - result.count ShouldEqual 2
  - result.matches ShouldJSONContainWithKey id
```

`find_all` (Go regexp) takes every match over `value`, or its first capture group when the regexp has one. `replace:
[<regexp>, <replacement>]` is applied to each match (`$1` expands a group). Result: `result.count`; `result.matches`, a
JSON array text that a `range:` takes as it is (a match that is a JSON object or array is put as JSON, any other as a
string); `result.matchesjson`, the parsed array; with `duplicates: true`, `result.duplicates`, a JSON array of the
matches that occur more than once, each once, in the order of their first occurrence (`[]` when none); with `join:
<separator>`, `result.joined`, the matches as written joined by the separator. A bad regexp or a `replace` that is not
two items fails the step. Point test: `executors/check/check_test.go`, golden files in `executors/check/testdata`.

## 18. Two JSON values compared: check `equal_json`, `sort_lists`

```yaml
- type: check
  value: '{{.xmlriver_describe}}'
  equal_json: '{{.dataforseo_describe}}'
  sort_lists: true
  assertions:
  - result.json_equal ShouldBeTrue
```

`value` and `equal_json` are parsed as JSON and compared with keys in any order; with `sort_lists: true` every list,
at any depth, is compared as its items in any order. Result: `result.json_equal`; when they differ,
`result.json_diff` prints both values in canonical form. A value that is not JSON fails the step. Point test:
`TestCheckEqualJSONSortLists`.

## 19. Span lines with the scope, `not_matches` and `values_of` of a tempo step

Each span line of section 13 now carries the instrumentation scope name of its span among the attributes as
`otel.scope.name=<scope>` (from `scope.name`, or `instrumentationLibrary.name` of an older trace), sorted with them.
`not_matches` (list of Go regexps) holds when no span line matches any of them; `result.mismatch` names each span
line and regexp that match.

`values_of: {span: <regexp>, attr: <regexp>}` gives `result.values`, a JSON array of the values of the attributes
whose key matches `attr`, of the spans whose line matches `span`, each once, in span start order:

```yaml
- type: tempo
  trace_id: '{{.unit_trace}}'
  tempo_url: '{{.tempo_url}}'
  trace_dir: '{{.trace_dir}}'
  values_of: {span: 'INSERT INTO outbox\.outbox_slice', attr: '^db\.query\.text\.arg\.4$'}
  assertions:
  - result.values ShouldNotContainSubstring sent
```

Point tests: `executors/tempo/search_test.go` (`TestSpanLinesCarryScope`, `TestNotMatches`, `TestValuesOf`).

## 20. Traces found by search, spans by TraceQL: tempo `search`, `contains_any`, `traceql`

A `tempo` step with `search` and no `trace_id` calls `GET <tempo_url>/api/search`:

```yaml
- type: tempo
  tempo_url: '{{.tempo_url}}'
  trace_dir: '{{.trace_dir}}'
  search:
    tags: 'service.name=svc-a scenario_id={{.sid}}'   # or query: '{resource.service.name="svc-a"}' (TraceQL)
    start: '{{.window_start}}'                        # unix seconds; start and end go together
    end: '{{.window_end}}'
    limit: 50
  contains_any: ['{{.unit_id}}']
  contains: ['content.scrape.request process']
```

`search.query` (TraceQL) or `search.tags` (logfmt) is required. Every found trace is fetched; the traces with a span
line that contains an item of `contains_any` (all when it is not given) are kept. The match is the first kept trace
that holds `contains`, `not_contains`, `matches` and `not_matches`; with `trace_dir` it is saved with its tree as a
`trace_id` step saves it. Result: `result.trace_ids` (JSON array of the kept traces, 32 hex each), `result.count`,
`result.trace_id`, `result.span_names` (JSON array of the span names of the match, each once, in start order),
`result.values` of `values_of` over the match, `result.trace_saved`, `result.trace_file`, `result.tree_file`, and
`result.mismatch` (`no found trace holds the checks` with the first trace's mismatch when checks are given and none
holds). `trace_saved` is `no` without `trace_dir`, so such a step writes its own assertions.

A `tempo` step with `traceql: <query>` and no `trace_id` sends the query as it is (`q=`), over the window `search.start`,
`search.end`, `search.limit` and `search.spss` (spans per span set). Result: `result.span_list`, a JSON array of every
matched span, each once per trace, `{trace_id, span_id, name, start_unix_nano, end_unix_nano, duration_nanos,
attributes}`; `result.count`, the number of spans; `result.trace_ids`. A structural query, `{name="A"} >> {name="B"}`
(B a descendant of A) or `<<`, `>`, `<`, `~`, selects the spans; order and count are checked by ordinary assertions
and `check` steps over `result.span_list`. TraceQL cannot compare an attribute of one span set with an attribute of
another, so a join on an attribute value is not a `traceql` input. The step writes its own assertions: no trace is saved.

Point tests: `TestSearch`, `TestSearchNoMatch`, `TestTraceQL`.

## 21. A request field of N items: grpc `data_repeat`

```yaml
- type: grpc
  method: demo.v1.Orders/Place
  data_json: '{{.order_payload}}'
  data_repeat:
    path: order.keywords
    count: 1000
    template: '{"id":"ent{{.index}}g","text":"{{.keyword_text}}"}'
```

The field at `path` (dot-separated, made when missing) of the request (`data`, `data_json`, or the one built by
`request_from_reflection`) is set to `count` items, item i being `template`, a JSON text, with `{{.index}}` = i,
executed inside the executor. `{{.index}}` survives the step interpolation as a var that is not defined, so
`data_repeat` is not for a `range:` step, where `.index` is the range index. Every grpc call returns
`result.request_size` and `result.request_sha256`, the size in bytes and the sha256 of the JSON request body sent; with
`data_repeat` the result keeps no `result.request`, and the protocol file line carries the size and the sha256 in place
of the request. Point test: `executors/grpc/data_repeat_test.go`.

## 22. A passive exchange declare: rabbitmq `exchange_passive`

`exchange_passive: true` checks that `exchange` exists instead of declaring it; a missing exchange fails the step with
`exchange "<name>" does not exist: Exception (404) Reason: "NOT_FOUND - …"`. With `client_type` `publisher` or
`subscriber` the passive check takes the place of the declare; with no `client_type` the step does only the check:

```yaml
- type: rabbitmq
  addrs: '{{.amqp_url}}'
  exchange: crawler.tasks
  exchange_type: topic
  exchange_passive: true
```

Point test: `executors/rabbitmq/exchange_test.go`.

## 23. A file written with no shell: `type: writefile`

```yaml
- type: writefile
  path: '{{.stand_dir}}/S0-R1.ready'
  content: '{{.repair_lines}}'
```

`path` (required) is taken from the testsuite folder unless absolute; missing folders are made and an existing file is
replaced. Result: `result.path` (absolute), `result.size`. Point test: `executors/writefile/writefile_test.go`.

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
