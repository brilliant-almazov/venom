# Cost journal — append-only

One line per session and per iteration. Kept by the hook `~/.claude/hooks/costs-autolog.cjs`
from transcripts (dedup by `message.id`); a live session's line is updated until it closes.
Paid = input + cache write + output; money is over all four counters, at Opus rates.

| Timestamp | Repository | Session or executor | Turns | Input | Cache write | Cache read | Output | Paid | Time | Money |
| -- | -- | -- | --: | --: | --: | --: | --: | --: | --: | --: |

| 2026-11-16T04:32:53+0300 | venom | lead a39a670d (2026-11-16) | 142 | 326 | 262243 | 25480207 | 94887 | 357456 | 04:32:53-05:21:31 | $50.26 |
| 2026-11-16T13:59:20+0300 | venom | lead 1504e6b3 (2026-11-16) | 47 | 94 | 164880 | 5988706 | 48327 | 213301 | 13:59:20-14:10:24 | $15.70 |
| 2026-11-17T13:57:48+0300 | venom | lead 879508ca (2026-11-17) | 82 | 176 | 226063 | 13685465 | 63235 | 289474 | 13:57:48-14:26:36 | $29.51 |
| 2026-11-17T17:30:53+0300 | venom | lead 6c68eb47 (2026-11-17) | 78 | 164 | 245752 | 14281179 | 80825 | 326741 | 17:30:53-17:52:22 | $32.09 |
| 2026-11-17T18:10:31+0300 | venom | lead a4e50296 (2026-11-17) | 73 | 162 | 145368 | 8106451 | 36054 | 181584 | 18:10:31-18:37:00 | $17.59 |
| 2026-11-17T18:45:12+0300 | venom | lead 72474680 (2026-11-17) | 60 | 138 | 152386 | 6976355 | 34409 | 186933 | 18:45:12-19:11:23 | $15.90 |