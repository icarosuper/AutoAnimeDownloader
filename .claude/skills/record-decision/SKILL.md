---
name: record-decision
description: Add an entry to AutoAnimeDownloader's docs/agents/decisions/ — next free number, the NNN-<slug>.md file in the house format, the regenerated index, and the pointer in the code. Use whenever a change introduces a non-obvious or unusual pattern (something a later reader would "fix"), when the user asks to record/registrar a decision, or when amending or reversing an existing entry.
---

# Record a decision

`docs/agents/decisions/` holds patterns that look wrong but are intentional, one decision per file
(`NNN-<slug>.md`, zero-padded), with `index.md` as the index. The index is **generated** — never
edit it by hand. Why it is laid out this way: `decisions/134`.

```bash
S=.claude/skills/record-decision/decision.sh
$S next                     # next free number
$S new "<title>"            # create NNN-<slug>.md for the next number, print its path
$S path 43                  # file of an existing entry
$S refs 43                  # every reference to #43 in code and docs
$S about src/internal/nyaa/nyaa.go   # entries that name the file or that it cites (decision-checker uses it)
$S index                    # regenerate index.md from the files' headings
$S check                    # headings match filenames, index current, every link resolves
```

## Is it a decision?

Yes when a later reader would ask "why this, when the obvious way is the other one?" and the
answer isn't in the code itself. No for a plain description of how something works (that is
`architecture.md`, and only if the code can't answer it) or for a bug fix with nothing surprising
left behind. Changing an existing decision → amend or reverse **that** entry (below), don't add a
competing one. Search first: read `index.md`, then `grep -ril "<symbol>" docs/agents/decisions/`.

## 1. Write the entry

`$S new "<title>"` creates the file with its heading, `# N. <title>`. The title states the rule,
not the topic — `The blacklist deletes what the daemon downloaded and spares what the user
downloaded by hand`, not `Blacklist deletion`. Body, in this order:

```markdown
# N. <the rule, as a sentence>

**Location:** `internal/<pkg>/<file>.go` — `<func>` (what in it carries the rule); the tests that
pin it by name.

**What it looks like:** the reading that makes someone want to "fix" it.

**Why it's right:** the reason, the real case that produced it (anime, ID, symptom), what was
measured, and the trade-off accepted.

**Don't "fix" by:** each tempting change, and why it breaks.
```

Plain English, short sentences, wrapped near 100 columns like the other entries. Cite other
entries by their file: `[#79](079-pack-choice-asks-does-it-cover-the-window-not-is.md)` between
entries, `[decisions/079](decisions/079-pack-choice-asks-does-it-cover-the-window-not-is.md)` from
another doc under `docs/agents/`, with the right relative path from elsewhere. Get the filename
from `$S path N`, never by hand.

## 2. Index

```bash
$S index
```

## 3. Pointer in the code

Next to the code that looks wrong, a comment that says the why in a few words and names the entry
in the house format — `(decisions/NNN)`, three digits, which is what `grep` finds and what
`ls docs/agents/decisions/NNN-*` opens:

```go
// Converts saved AnimeIDs from list-entry ids to media ids (decisions/043).
```

Full rule in `docs/agents/conventions.md`, "Comments".

## 4. Check

```bash
$S check
```

## Amending or reversing

- **Amending** (a later decision refines this one): add `**Amendment (see #M):**` to the old entry,
  where #M is the entry that changed it, saying what still holds and what no longer does; update its
  `Location` and `Don't "fix" by`.
- **Reversing:** the entry stays (its number is cited in code and docs); rewrite the heading and
  body to the new rule, say what it reverses and why, then `$S index`. **Never rename the file** —
  every link uses the filename, so it stays even when the slug no longer matches the title. Review
  what cites it with `$S refs N`: a pointer whose sentence states the old rule needs rewording.

Tell the user the number and the one-line rule you recorded.
