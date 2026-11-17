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

`--protocol-dir <dir>` (env `VENOM_PROTOCOL_DIR`, `.venomrc` key `protocol_dir`) writes `<dir>/<testsuite>/<testcase>--<outcome>.md`
for every testcase, whatever its outcome: passed, failed on a `Should…` assertion, failed on a `Must…` assertion, or
skipped by `--stop-on-failure`. The outcome in the file name is `ok` for a passed testcase, `partial` for a failed one
where some of its assertions held, `failed` for a failed one where none held; every protocol carries one of the three,
a skipped testcase too: `partial` when some of its assertions held, `failed` otherwise (`protocol_outcome.go`). Sections:

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
  call sent: method, full url, headers (Authorization `Bearer <secret>`, every credential `<secret>`, X-Idempotency-Key shown) and the body, JSON one key per
  line; a step that is not http shows the call fields of its interpolated step. The call is the step with the `call:`
  info line when the testcase has one, else a step with a trace id and a code: the last failed call, else the last call;
- `## Response`: the code, the trace id (result.trace_id, meta.trace_id of the body or the X-Trace-Id header), the
  X-Trace-Id, Traceparent, X-Request-Id and X-Build headers and the body verbatim, and the `fact` info lines;
- `## State after`: every later call of the testcase, the read-back after a write, with its request and answer;
- `## Trace`: the span tree `<trace_dir>/<trace_id>.md` of that call, looked up in every `trace_dir` and `traces` the
  steps were given, then the tree of every other step with a trace id of its own and a code, under `- step: N "<name>"`;
  when that call returned no trace id, the tree of every step with a trace id and a code the same way (a teardown whose
  stand step failed after its calls);
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
calls save `<trace_dir>/<trace_id>.json` and, next to it, the span tree `<trace_dir>/<trace_id>.md` (`tree_md.go`):
the heading ``### Trace `<id>` ``, a table `trace_id | root | duration, ms | spans | ❌ | source`, the line
`summary: ✅ N spans · ❌ M spans · first problem: <service> <span> — <problem>` (or `no problems`), and the tree in a
`text` block, `├─`/`└─` under the parent in start order, each span `<service> · <span> · <ms> ms · ✅|❌`. Under each
span: `problem: …` when it has one, every attribute `<key>: <value>` sorted by key, every event `event: <name>` with its
attributes, and the query `db.query.text` as `SQL: …`, one clause per line (FROM, WHERE, AND, OR, JOIN, VALUES, SET,
GROUP BY, ORDER BY, LIMIT) wrapped at 150 characters, or `SQL: same SQL as the span above` for a query already printed.
A span has a problem with an error status (`OTLP ERROR`), `http.response.status_code` of 400 and up (`http <code>`),
`rpc.response.status_code` other than `OK` (`grpc <code>`), or an `exception` event; the problem adds the status
message, every `exception.message`, and `<key>=<value>` of `fault.reason` and every `*.error.*reason` that is not
`none`. The protocol prints
this file under Trace and the trace gate fails a call whose tree is missing. Result: `result.trace_id`,
`result.trace_saved` (`yes`, `late`, `no`), `result.trace_file`, `result.tree_file`, `result.spans`, `result.mismatch`.

The tree of `executors/tempo/testdata/render_example.json` (golden test `TestRenderExample`):

````markdown
### Trace `4bf92f3577b34da6a3ce929d0e0e4736`

| trace_id | root | duration, ms | spans | ❌ | source |
| -- | -- | -- | -- | -- | -- |
| `4bf92f3577b34da6a3ce929d0e0e4736` | svc-edge · POST /v1/items | 42.37 | 8 | 4 | `4bf92f3577b34da6a3ce929d0e0e4736.json` |

summary: ✅ 4 spans · ❌ 4 spans · first problem: svc-edge POST /v1/items — OTLP ERROR · http 409 · item already exists · edge.error.reason=already_exists

```text
└─ svc-edge · POST /v1/items · 42.37 ms · ❌
   │  problem: OTLP ERROR · http 409 · item already exists · edge.error.reason=already_exists
   │  edge.error.reason: already_exists
   │  http.request.method: POST
   │  http.response.status_code: 409
   │  http.route: /v1/items
   ├─ svc-edge · auth.check · 0.25 ms · ✅
   │     auth.result: authenticated
   │     cache.hit: False
   └─ svc-store · store.v1.ItemService/Create · 38.6 ms · ❌
      │  problem: grpc ALREADY_EXISTS · fault.reason=already_exists
      │  fault.domain: store.item
      │  fault.reason: already_exists
      │  rpc.method: store.v1.ItemService/Create
      │  rpc.response.status_code: ALREADY_EXISTS
      │  event: rpc.payload.received
      │     rpc.request.body_size: 26
      └─ svc-store · tx main · 37.7 ms · ❌
         │  problem: OTLP ERROR · already_exists
         │  event: exception
         │     exception.message: already_exists
         │     exception.type: *fault.Fault
         ├─ svc-store · query · 1.7 ms · ✅
         │     db.rows_affected: 0
         │     db.system.name: postgresql
         │     SQL: begin
         ├─ svc-store · set · 0.04 ms · ✅
         │     db.system: redis
         │     server.address: cache.local
         │     server.port: 6379
         │     SQL: set lock:item:widget-1 0f8b2c6e-1d2a-4c3b-9e5f-7a6b5c4d3e2f ex 30 nx
         ├─ svc-store · query · 35.3 ms · ❌
         │     problem: OTLP ERROR · duplicate key value violates unique constraint "items_code_key"
         │     db.query.text.arg.1: 1001
         │     db.query.text.arg.2: 7
         │     db.query.text.arg.3: widget-1
         │     db.query.text.arg.4: 2026-01-01 00:00:00 +0000 UTC
         │     db.rows_affected: 0
         │     db.system.name: postgresql
         │     SQL: INSERT INTO catalog.items (id, kind_id, code, created_at)
         │          VALUES ($1, $2, $3, $4)
         └─ svc-store · query · 0.2 ms · ✅
               db.rows_affected: 0
               db.system.name: postgresql
               SQL: rollback
```
````

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

## 24. Two span sets of one trace joined: tempo `join`

A `tempo` step with `trace_id` and `join` joins two span sets of the saved trace in Go, with no TraceQL. Each side
takes the spans whose span line of section 13 matches `match` (Go regexp), the value of the attribute whose key is
`attr` (the key as it is), and of that value the one group of `extract` (Go regexp with one group, optional; a value
it does not match is left out). With `parent` (Go regexp, optional) a side takes only the spans whose parent's span
line matches it; a span whose parent is not in the trace has an empty parent line:

```yaml
- type: tempo
  trace_id: '{{.unit_trace}}'
  tempo_url: '{{.tempo_url}}'
  trace_dir: '{{.trace_dir}}'
  join:
    left:  {match: ' · Dial · ', attr: 'arg.req', extract: '"unit":"([^"]+)"'}
    right: {match: 'INSERT INTO', attr: 'arg.3', parent: 'CrawlUnitManager'}
```

Result, one value per line: `result.join_left` and `result.join_right` (the values found, in span order),
`result.join_pairs` (the left values with an equal right value), `result.join_missing` (the left values with no equal
right value, empty when every left value has one). A bad regexp, or an `extract` with other than one group, is a step
error. Default assertion: `result.join_missing ShouldBeEmpty`; a step that lists its own assertions adds it itself.
Point test: `executors/tempo/join_test.go`.

## 25. The order of spans in each subtree of one trace: tempo `order`

A `tempo` step with `trace_id` and `order` checks the order of spans in Go over the saved trace, with no TraceQL. Each
span whose span line of section 13 matches `scope` (Go regexp) is a scope; inside its subtree (its descendants, not the
scope span itself) `before` and `after` each take the spans whose span line matches `match`, whose parent's span line
matches `parent` and whose grandparent's span line matches `grandparent` (Go regexps; `parent` and `grandparent` are
optional). `at` is `start` or `end`: the time of a span compared. With `first: true` an end keeps only the span of its
set that starts first. A scope is in order when the latest `before` time is at or before the earliest `after` time:

```yaml
- type: tempo
  trace_id: '{{.crawl_trace}}'
  tempo_url: '{{.tempo_url}}'
  trace_dir: '{{.trace_dir}}'
  order:
    scope: ' · consumer/crawl · '
    before: {match: ' · publish', parent: 'ReserveManager.CreateBySent', grandparent: ' · handle · ', at: end}
    after:  {match: ' · DialProvider.Dial · ', at: start, first: true}
```

Result, one scope per line as `<service> · <span> · <span id>`, in start order: `result.order_scopes` (every scope),
`result.order_ok` (the scopes in order), `result.order_bad` (the scopes out of order, each as `<scope>: <before span> at
<unix nanos> is after <after span> at <unix nanos>`), `result.order_missing` (the scopes with no `before` or no `after`
span). A bad regexp, or an `at` other than `start` or `end`, is a step error. Default assertion: `result.order_bad
ShouldBeEmpty`; a step that lists its own assertions adds it itself, and adds `result.order_missing ShouldBeEmpty`
when a scope with no span of an end is a failure. Point test: `executors/tempo/order_test.go`.

## 26. User executors found up from the testsuite: no `--lib-dir` needed

The user executor files are read from these `lib` directories, in this order: every `--lib-dir` entry, then for each
testsuite every `lib` directory from its own directory up to the repository root (a directory holding `.git`) or the
working directory, whichever comes first, nearest first, then `lib` of the working directory. A suite in
`scenarios/billing/account/` finds the executors of `scenarios/lib/` (run 16-4: «user executor "trace_save" not found -
loaded executors are: []»). A testsuite under neither the repository root nor the working directory gets only its own
`lib`. A `lib` that is a symlink is resolved first, as `filepath.Walk` does not descend a symlinked root. Two executors of
one name in two lib directories: the earlier one in this order wins (the nearer lib, or `--lib-dir`); two of one name in
the same lib directory are still an error. Point tests: `TestUserExecutorLibFoundUpFromSuite`,
`TestUserExecutorNearestLibWins`, `TestUserExecutorSymlinkedLib`, `TestUserExecutorLibStopsAtRepoRoot`.

## 27. Credentials hidden in every output, declared secret or not: `<secret>`

Every secret becomes `<secret>` with no `secrets:` declared (`log.go`, `SecretMark`, `maskCredentials`):

| What | Example | Written |
| -- | -- | -- |
| a declared secret | `secrets: [token]` | `<secret>` |
| the password of a URL with userinfo | `postgresql://postgres:s3cr3t@pg-billing…` | `postgresql://postgres:<secret>@pg-billing…` |
| a JWT | `eyJ….….…` | `<secret>` |
| a Supabase API key | `sb_publishable_…`, `sb_secret_…` | `<secret>` |
| the value of a header, field or var named Apikey, X-Api-Key, Api-Key, Cookie, Set-Cookie or holding token, secret, password, passwd | `Apikey: sb_…`, `"access_token":"…"`, `password=…` | `Apikey: <secret>`, `"access_token":"<secret>"` |
| an Authorization | `Authorization: Bearer abc` | `Authorization: Bearer <secret>`, the scheme kept |

The forms `name: value`, `name=value`, `"name":"value"` and Go's `map[Name:[value]]` are read; a JSON `null`, `true`, `false`
stays, an unquoted JSON number becomes `"<secret>"`, so a JSON output stays JSON. `X-Idempotency-Key` is no credential
and is shown. A text hidden twice stays the same. It is applied to the vars of the testsuite, of the testcases and of
the steps (`inputVars`, `computedVars`), the raw and interpolated steps, systemout, systemerr, info lines and failures,
`test_results_*.{json,xml,yml,tap}` and the html report, the protocols, the span trees, `io.jsonl`, `venom.log` and
every file of `--run-dir`. Run 16-4 carried the database DSN and the gateway tokens 52 times in one result file; run 17-5
carried the Supabase Apikey in `protocols/SB-06-01-…/setup.md:32` and `raw/io.jsonl`. Point tests: `TestMaskCredentials`,
`TestOutputMasksCredentialsWithNoSecret`, `TestCredentialsBecomeSecretMark`, `TestCredentialInAGoMapOfLists`,
`runartifacts.TestRunOfEveryOutcome` (no key in any file of the run folder).

## 28. A log of every call: `<protocol-dir>/io.jsonl`

With `--protocol-dir`, every call of every testcase — a step with `result.statuscode` or `result.code`, whatever its
outcome — appends one JSON line to `<protocol-dir>/io.jsonl` (`protocol_io.go`): `testsuite`, `file`, `testcase`,
`step` (1-based), `name`, `executor`, `call` (the call line of the protocol), `method`, `url`, `request_headers`
(credentials `<secret>`) and `request` (the body) of an http call, else `request` as the declared fields of the step,
`code`, `trace_id`, `response_headers`, `response` (the body or systemout), `status`, `errors`, `start`, `duration_s`.
Testcases and testsuites run in parallel write whole lines. Point test: `TestProtocolIOLines`.

## 29. Every scenario file checked with no stand: `venom run --dry-run`

`venom run --dry-run <files>` loads every testsuite as a run does and runs no step, so nothing touches the network:

- the YAML is parsed; a file that does not parse fails as `<file>:<line>: error while unmarshal file: …`;
- the user executors are loaded (section 26) and every step type of a testsuite and of a user executor file is resolved
  to an executor; one that is not fails as `<file>:<line>: testcase "<name>" step N: user executor "<type>" not found …`
  or `<file>:<line>: executor "<name>" step N: unknown executor "<type>"`;
- every `{{.x}}` left in a step after the testsuite vars must be set by the testcase vars, by the `vars:` or `extracts:`
  of an earlier step of the testcase, by an earlier testcase (`{{.<testcase>.<var>}}`), by venom (`{{.venom.…}}`) or by
  `range:` (`index`, `key`, `value`); `{{.result.…}}` is allowed in the `vars:`, `info:` and `assertions:` of a step,
  read after the step ran; an expression with `default`, `ternary` or `coalesce` is not checked. An unset one fails as
  `<file>:<line>: testcase "<name>" step N: {{.x}} is set by no var …`, the line being the first from the step down that
  reads it; an expression the testsuite vars printed as `<no value>` fails on the step line.

The first failure is printed on stderr and the exit code is 2; a clean load prints `dry run: N testsuites, M
testcases, K steps, U user executors loaded; every executor resolved, every var set` and exits 0. The vars of
`--var`, `--var-from-file` and `VENOM_VAR_*` count as set. The step type and the YAML errors are named by
`<file>:<line>` in a normal run too. Point tests: `TestDryRunGreen`, `TestDryRunUnsetVarNamedByLine`,
`TestDryRunVarOfLaterStep`, `TestDryRunUnknownExecutorNamedByLine`, `TestDryRunUnknownExecutorInLib`,
`TestDryRunUnparseableYAMLNamedByLine`.

## 30. A testcase that can end with no call fails the dry run

Stock executors checked: none apply - this is a dry-run form check, not an executor.

Run 16-6 wrote 16 protocols with «no trace: the testcase has no call»: the testcase opened with `type: check` steps
asserting `Must…` on the inputs of its setup, a failed `Must…` stopped it before the call. `venom run --dry-run` now
checks the form of every testcase on the parsed YAML, before the templates are filled:

- the testcase holds a call step: a step whose `type` is not `check`, `wait` or `trace_save` (http, grpc, sql, amqp,
  rabbitmq, tempo, s3, a user executor, …); a testcase with none fails as
  `<file>:<line>: testcase "<name>": testcase has no call step`, the line of the testcase;
- no step before the first call asserts `Must…` in its `assertions:`; one that does fails as
  `<file>:<line>: testcase "<name>": step N asserts <MustOperator> before the first call`, the line of the step.

A testcase whose name starts with `setup` or `teardown` (any case) and every file under `scenarios/preflight/` are not
checked. A failure exits 2 as every dry run failure. Point tests: `TestDryRunTestCaseWithNoCall`,
`TestDryRunMustBeforeCall`, `TestDryRunCallThenMustGreen`, `TestDryRunPreflightNotChecked`, files of
`testdata/dryrun-form/`.

## 31. A run laid out in one folder: `--run-dir`, everything by Venom, no agent

`venom run --run-dir <run>` writes the whole run — protocols, schemes, report, problems, traces — with no copy, packing
or drawing by hand (package `runartifacts`):

| Path | What |
| -- | -- |
| `<run>/protocols/<row id>-<outcome>-<testsuite>/<row id>-<outcome>-<rest>.md` | the protocol of section 10 for every testcase, whatever its outcome, named by the testcase name as written with its outcome `ok`, `partial` or `failed` of section 10 between the row id and the rest of the name (`SB-01-01-ok-POST-webhook-….md`; first, `ok-<name>.md`, for a name with no row id; `ProtocolOutcomeFile`), with a `## Links` section after its file line; every protocol carries one of the three: a testcase skipped is `failed`, or `partial` when some of its assertions held, and a testcase interrupted, not run or pending is `failed` (`ProtocolOutcomeName` with `ProtocolOutcomeFailed`); the protocol written as the testcase began is renamed when it ends, one file per testcase, and INDEX.md, the schemes, REPORT.md and PROBLEMS.md link the new name; the folder of the testsuite is named by the testsuite short name with its outcome the same way (`SB-01-01-ok-receive-webhook-ok/`; `ok-<name>/` with no row id): `ok` when every testcase of it ended `ok`, `partial` when one is `ok` or `partial`, `failed` otherwise; it moves as testcases end and the whole run folder is redrawn with the new links |
| `<run>/protocols/traces/<trace_id>.md` | the span tree of every traced call of the run (section 13), secrets hidden |
| `<run>/protocols/INDEX.md` | one row per testcase in testsuite file order: `suite`, `testcase`, `verdict` (✅/❌/⏸ and the answer code, or `⏸ interrupted`, `⏸ not run`, `⏸ pending`), `trace` (a link to the tree, `trace not saved <trace_id>`, or `—` for a call with no trace), `link` to the protocol, `scenario` (`<file>:<line>`), `scheme` of the case, `path` scheme; above the table every path and the system with a link to its scheme, its verdict and the share of the outcomes |
| `<run>/schemes/SYSTEM.md` | the scheme of the system: every path, every case of it and every transition of the case, drawn from the run; the paths; the checked cases; the system and each path carry the share of the outcomes |
| `<run>/schemes/<path>/PATH.md` | the scheme of one path: its cases with their transitions, with the share of the outcomes; the checked cases |
| `<run>/schemes/<path>/<testcase>.md` | the scheme of one case: its outcome and the share of its transitions ✅ / ❌ / ⏸ in %, its transitions drawn, the table of the transitions, how the case works |
| `<run>/REPORT.md` | the verdict and the counts of the run, start and end, the reason a run ended early, every path, every testsuite with a link to its protocols folder, every slice of the scheme — system, path, case — with its link, verdict and share, every trace id with its tree; every count of testcases, in REPORT.md and the schemes, is followed by the share of the outcomes `ok N% · partial N% · failed N%` in whole % |
| `<run>/PROBLEMS.md` | every testcase that did not pass — failed, skipped, interrupted, not run — with its first failed step, the reason, the scenario line, the protocol, the scheme and the trace |
| `<run>/raw/` | every other output of Venom: the reports, `venom.log`, dumps, `io.jsonl` (section 28), the traces fetched from Tempo; `--run-dir` replaces `--output-dir` |

A path is the folder of a testsuite file (`scenarios/supabase/<path>/SB-NN-NN-*.yml`), named by its last element; two
paths of one last element are named by their whole folder as a slug. A transition is a step of the testcase: its
number, name and request, then ok ✅ / not-ok ❌ / not run ⏸ with the answer code and the first 8 characters of the trace
id, and the reason on the next line when it is not ok. A step after a failed `Must…` is drawn as
`⏸ not run: the testcase stopped before this step`; a testcase skipped by its `skip:` draws each step as skipped.

```text
SB-01-01-02 read link · ❌ 409 `4bf92f35`
├─ 1 call · http POST https://… ─► ❌ 409 `4bf92f35`
│     reason: expected 200 got 409
└─ 2 check · check ─► ⏸
      reason: not run: the testcase stopped before this step
```

Each scheme lists its checked cases with how each works: `testcase`, `path`, `verdict`, `request` (the call line of
section 10), `expected` (the info lines `happens`, `expect`, `right`, `norm`), `transitions` (`1 ✅ · 2 ❌ · 3 ⏸`),
`scenario`, `protocol`, `scheme`, `trace`.

**Links both ways.** system → path → case → protocol and back: SYSTEM.md links each path scheme and each case scheme;
PATH.md links SYSTEM.md, INDEX.md and each case; the case scheme links its path, the system, the protocol, the scenario
line of the testcase and of each step and every trace id with its tree; the protocol links the scenario line of the
testcase and of its call, every trace id (32 hex) with its tree, the schemes of its case, its path and the system, the
index, the report and the problems. A scenario line is written `<file>:<line>` and links `<file>#L<line>`.

**Every outcome, written as it happens.** A protocol is written the moment its testcase ends, and INDEX.md, REPORT.md,
PROBLEMS.md, SYSTEM.md, the scheme of its path and of its case are redrawn with it:

| Outcome | Protocol and verdict |
| -- | -- |
| passed, failed on Should or Must, a setup testcase failed, a precondition failed, a step timed out | the protocol of section 10, ✅ or ❌ |
| steps not run after a failed Must | the protocol of section 10; each step not run is `⏸ not run` in the scheme |
| skipped by `skip:` or by `--stop-on-failure` | the protocol of section 10, ⏭ / ⏸ |
| a testcase under way when the run is killed (SIGKILL too) | written as `⏸ interrupted: the run ended before the testcase did` the moment it starts, replaced when it ends |
| interrupted by SIGINT or SIGTERM | the testcase under way `⏸ interrupted: interrupted by signal <name>`, every testcase not started `⏸ not run: …`; exit 128 + the signal number |
| the run stopped by an error of its setup (YAML, vars, a testsuite that cannot start) or of the report | every testcase `⏸ not run: setup failed: …` / `run stopped: …`; REPORT.md and PROBLEMS.md carry the error |
| the runner panics | every testcase not ended `⏸ … runner crashed: panic: …`, then the panic goes on; a panic inside a testcase run in parallel ends the process at once, and the folder holds what was written as each testcase began and ended |

Every testcase of the parsed testsuites is in INDEX.md from the start of the run, `⏸ pending` until it starts.

**No empty folder.** A folder is made only with its first file: the `tempo` step and the trace fetch make the trace
folder only when a trace is saved (run 17-4 left an empty `raw/identifier-link/` and no `protocols/`), and the end of
the run removes every empty folder left under `<run>`.

- A span tree of a traced call that no step saved is fetched from Tempo before the trace gate, by the `tempo_url` of a
  step of the testcase — `tempo_url` of a native executor or `tempo` of a user executor (`http_call`, `trace_save`) — or
  its var `tempo_url` or `tempo`, up to the late wait of the `tempo` executor, 90 s (run 17-7: every step of the case
  said `tempo`, the tree was never fetched, `TestTraceFetchedByTempoOfUserExecutor`; run 17-12: the tree of the
  monolith webhook call came after 10 s, `TestEveryStepTraceInProtocol`); with no Tempo address `venom.log` says so; a trace Tempo does not have makes the line
  `- trace not saved: <trace_id>: <folders tried>` under Trace and fails the testcase (section 10, `traceGate`).
- A call that returns no trace id (the monolith) needs no trace: its Trace section lists every `sql` step of the
  testcase in its place, `- db read: step N "<name>" <status>`, each query and the rows read.
- A user executor that checks the answer inside itself and fails (an answer that is not 200) still gives its step the
  result of its inner call — `result.request.*`, `result.statuscode`, `result.body`, `result.headers.*`,
  `result.trace_id` — and the testcase the trace folders of its inner steps, so the protocol carries the request as
  sent, the answer and the trace (runs 17-1…17-3 lost them, `keepUserExecutorCall`). The output of an executor that
  passed is laid over it, its keys win.

Scheme templates of a case set, when one still has them: the first `schemes/` folder met going up from a testsuite
file, else `<run>/../../../schemes` (a run folder `<set>/runs/<YYYY-MM>/<DD-n>` next to `<set>/schemes`), is copied to
`<run>/schemes/` file by file with only its anchors replaced. `<case>` is a testcase name, or the row id its name or the
name of its testsuite starts with, followed by a space, `-`, `_`, `:`, `·` or `.` (`SB-01-02`); of several testcases of
one case the first failed one counts. `<hop>` is the key of a step info line `hop: <key>` (`hop: seed-<key>` for a seed
step).

| Anchor | Filled with |
| -- | -- |
| `⟦<case>⟧` | the verdict of the testcase ✅/❌/⏸, the code of its call, the first 8 characters of its trace id |
| `⟦<case>:<hop>⟧` | the same of the step with `hop: <hop>`; ⏸ when the step did not run |
| `⟦<case>:<hop>:reason⟧` | the first failure of that step, `—` when it passed, `not run` when it did not run |
| `⟦*:<hop>⟧` | the hop over every testcase that has it: ✅ when all passed, else ❌, then passed/all |
| `⟦<case>:protocol⟧` | a link to the protocol, relative to the scheme file |
| `⟦<case>:trace⟧` | a link to the span tree, `trace not saved <trace_id>`, or `—` |

A case the run does not have gives ⏸ (`not run` for a reason, `—` for a link). Point tests:
`runartifacts/writer_test.go` (`TestRunFolder`, `TestSchemesOfTheCaseSet`), `runartifacts/outcomes_test.go`
(`TestKilledRunLeavesInterrupted`, `TestAbortedRunNamesEveryTestcase`, `TestSetupFailureWritesTheRunFolder`,
`TestSchemesDrawnFromTheRun`, `TestNoEmptyFolders`), `runartifacts/run_test.go` (`TestRunOfEveryOutcome`: a whole run
of pass, Should, Must and skip), `TestUserExecutorCallKeptOnFailure`, `TestProtocolDBReadsInPlaceOfTrace`.

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
