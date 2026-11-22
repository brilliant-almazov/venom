# Rules of the prompts of venom

The canon of prompt sets applies as written in the agent config of the machine (skill `cgw-prompt-sets`); this file
keeps only what differs in venom.

| Rule | venom |
| -- | -- |
| language | English only, every line and every file name (English-only repository) |
| headings of an iteration | `## Iteration budget`, `## What to do`, `## Canon`, `## CHECK`, `## Done when`, `## Status` |
| layout | `prompts/<todo|in_progress|done>/NN-<set>/`, `INDEX.md` in every folder, `00-control.md`, `GRAPH.md` per set |
| status | only through `node ~/.claude/hooks/lib/prompt-status.js` |
| gates | only in CI (`.github/workflows/cgw-qa.yml`), never locally |
