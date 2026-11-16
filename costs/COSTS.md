# Cost journal — append-only

One line per session and per iteration. Kept by the hook `~/.claude/hooks/costs-autolog.cjs`
from transcripts (dedup by `message.id`); a live session's line is updated until it closes.
Paid = input + cache write + output; money is over all four counters, at Opus rates.

| Timestamp | Repository | Session or executor | Turns | Input | Cache write | Cache read | Output | Paid | Time | Money |
| -- | -- | -- | --: | --: | --: | --: | --: | --: | --: | --: |

| 2026-11-16T04:32:53+0300 | venom | lead a39a670d (2026-11-16) | 133 | 300 | 252874 | 23053683 | 87746 | 340920 | 04:32:53-05:17:10 | $45.91 |