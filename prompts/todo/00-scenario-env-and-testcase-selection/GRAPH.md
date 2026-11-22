# 00 — scenario ENV and testcase selection: dependency graph

Set 00: ⬜⬜⬜ … 0/3 closed, 0 in progress, 3 waiting  ·  480k plan / — fact (—)

```
WAVE 1                                      WAVE 2                                         WAVE 3

⬜ p1-01 --testcase selection · 115k ──────► ⬜ p1-02 env_set and reset · 209k ──────────► ⬜ p9-01 closing, CI · 156k
```

p1-02 follows p1-01 with no data dependency but two shared files: its "Before" blocks of `venom.go` and
`README-LOCAL.md` are the "After" blocks of p1-01. Both go to one executor, in order (batch A).

## Waves

| Wave | Iterations | Run together | Budget plan · fact · % |
| -- | -- | -- | -- |
| 1 | p1-01 | batch A, first | 115k · — · — |
| 2 | p1-02 | batch A, second | 209k · — · — |
| 3 | p9-01 | batch B | 156k · — · — |

## File crossings

| File | Iterations | How they are kept apart |
| -- | -- | -- |
| `venom.go` | p1-01, p1-02 | one executor, p1-02 after p1-01; p1-02 "Before" = p1-01 "After" |
| `README-LOCAL.md` | p1-01, p1-02 | same: section 32, then section 33, both before `## Build` |
| `cmd/venom/run/cmd.go` | p1-01, p1-02 | different blocks; same executor, in order |

## Outside the repository

```
↗ cgw-env-cli-5b: bin/cli var set|reset … --restart (set 07-one-command-up) ──► run of env_set on a live stand
↗ cgw-qa-scenarios-b3: suites write type: env_set (set 03-suite-env-declaration) ──► scenarios use it
```

Neither blocks a node of this set: the iterations test against a fake utility.

## Total

| Plan | Fact | % |
| -- | -- | -- |
| 480k tokens (iterations) + 480k tokens (service runs) | — | — |
