# Cost journal — append-only

One line per session and per iteration. Kept by the hook `~/.claude/hooks/costs-autolog.cjs`
from transcripts (dedup by `message.id`); a live session's line is updated until it closes.
Paid = input + cache write + output; money is over all four counters, at Opus rates.

| Timestamp | Repository | Session or executor | Turns | Input | Cache write | Cache read | Output | Paid | Time | Money |
| -- | -- | -- | --: | --: | --: | --: | --: | --: | --: | --: |

| 2026-11-16T04:32:53+0300 | venom | lead a39a670d (2026-11-16) | 122 | 274 | 240957 | 20186014 | 80586 | 321817 | 04:32:53-05:11:26 | $40.85 |