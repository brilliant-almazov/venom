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

- Testsuites still run one after another; the testcases inside a suite run in parallel.
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
