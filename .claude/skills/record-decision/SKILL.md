---
name: record-decision
description: Add an entry to AutoAnimeDownloader's docs/agents/decisions.md — next free number, the Index line, the entry in the house format, the anchor to cite it with, and the pointer in the code. Use whenever a change introduces a non-obvious or unusual pattern (something a later reader would "fix"), when the user asks to record/registrar a decision, or when amending or reversing an existing entry.
---

# Record a decision

`docs/agents/decisions.md` holds patterns that look wrong but are intentional. Entries are **not in
numeric order** in the file, and the Index at the top is how they are found — so never trust a
number written anywhere else; ask the helper.

```bash
S=.claude/skills/record-decision/decision.sh
$S next                     # next free number
$S anchor 43                # anchor of an existing entry
$S anchor "123. <title>"    # anchor a new heading will get
$S check                    # every entry has one Index line, every Index link resolves
```

## Is it a decision?

Yes when a later reader would ask "why this, when the obvious way is the other one?" and the
answer isn't in the code itself. No for a plain description of how something works (that is
`architecture.md`, and only if the code can't answer it) or for a bug fix with nothing surprising
left behind. Changing an existing decision → amend or reverse **that** entry (below), don't add a
competing one. Search first: `grep -n "<symbol>" docs/agents/decisions.md`.

## 1. Write the entry

Append it at the **end of the file** (one blank line before), heading `### N. <title>`. The title
states the rule, not the topic — `The blacklist deletes what the daemon downloaded and spares what
the user downloaded by hand`, not `Blacklist deletion`. Body, in this order:

```markdown
### N. <the rule, as a sentence>

**Location:** `internal/<pkg>/<file>.go` — `<func>` (what in it carries the rule); the tests that
pin it by name.

**What it looks like:** the reading that makes someone want to "fix" it.

**Why it's right:** the reason, the real case that produced it (anime, ID, symptom), what was
measured, and the trade-off accepted.

**Don't "fix" by:** each tempting change, and why it breaks.
```

Plain English, short sentences, wrapped near 100 columns like the rest of the file. Cite other
entries by anchor: `[#79](#79-...)` inside decisions.md, `[decisions.md #79](decisions.md#79-...)`
from another doc under `docs/agents/`, with the right relative path from elsewhere. Get every anchor
from `$S anchor`, never by hand.

## 2. Index line

Add, in numeric order, right after the previous number's line in `## Index`:

```markdown
- [**#N** — <title exactly as in the heading>](<$S anchor N>)
```

## 3. Pointer in the code

Next to the code that looks wrong, a comment that says the why in a few words and names the entry
in the house format — `(decisions.md #N)`, which is what `grep` finds:

```go
// Converts saved AnimeIDs from list-entry ids to media ids (decisions.md #43).
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
  body to the new rule, say what it reverses and why, and run `$S check` — a changed heading
  changes the anchor, so find and update every reference:
  `grep -rn "decisions.md#N-\|(#N-" --include=*.md --include=*.go .`

Tell the user the number and the one-line rule you recorded.
