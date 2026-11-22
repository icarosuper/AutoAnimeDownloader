# 134. Each decision is its own numbered file, and the index is generated from their headings

**Location:** `docs/agents/decisions/` — one `NNN-<slug>.md` per decision, `index.md` as the
index; `.claude/skills/record-decision/decision.sh` (`next`, `new`, `path`, `refs`, `about`,
`index`, `check`) and its `SKILL.md`. Code and docs cite an entry as `decisions/NNN` (rule in
`conventions.md`, "Comments").

**What it looks like:** 134 small files where one `decisions.md` used to be — a folder that only
grows, an index that seems to duplicate the filenames, and filenames whose slug no longer matches
the title once an entry is rewritten. The obvious "cleanups" are to merge them back into one file,
or to drop the index and let `ls` be the list.

**Why it's right:** until 2026-11-22 every decision lived in one `docs/agents/decisions.md`: 133
entries, 4110 lines, about 405 KB, with a hand-kept Index at the top and about 580 references
across code and docs. It worked, but the two ways an agent reads decisions both paid for the
single file:

- **Following a pointer** (`decisions.md #43` in a comment). The agent needed the Index or a
  `grep -n` to find the line, then a `Read` with an offset and a guessed length — it didn't know
  where the entry ended, so it read too much or cut it short. Now `ls docs/agents/decisions/043-*`
  is the entry, whole and nothing else.
- **Checking a plan against earlier decisions**, with no pointer to follow. The agent has to read a
  list first. The Index was about 24 KB (~6k tokens), but it shared a file with ~100k tokens of
  entries: a `Read` without a `limit` pulled 2000 lines, half the file, ~50k tokens. Keeping that
  from happening rested on the agent remembering a line limit. A separate `index.md` costs the same
  ~6k tokens and cannot be over-read. A keyword search (`grep -ril`) now returns files to open, not
  line numbers to chase with offsets.

Also weighed:

- **Anchors were tied to titles.** A link was `decisions.md#43-<slug of the whole title>`, so
  rewording a title (which reversing an entry requires) broke every link to it. A link is now the
  filename, and the filename is frozen at creation — the slug is a label, the number is the identity.
- **Numbering was manual.** Entries were not in numeric order in the file, so the next number and
  any entry's position came from a helper script. Zero-padded filenames sort in `ls` and in the
  GitHub file view, and the next number is the last file + 1.
- **Git and parallel work.** One file mixed every decision's history in `git log`, and two chats
  adding entries collided on the same file, especially on the Index. Per-file history and
  conflicts are now per decision; the index is regenerated, so a conflict there is solved by
  running `decision.sh index`.
- **What the single file did better, accepted as lost:** reading the whole set top to bottom, and
  one-file search. Neither is how decisions are read here — `CLAUDE.md` already said the file never
  needs to be read end to end — and `grep -r` over the folder covers the search.
- **Why not a standard ADR layout** (Nygard/MADR: Status, Context, Decision, Consequences): the
  one-file-per-decision part was taken from it, but the entry format stays. These entries exist to
  stop a later reader from "fixing" something intentional, and `**Don't "fix" by:**` says that
  directly; a generic `Consequences` section does not. A `Status`/`Superseded by` field was not
  added — amending and reversing keep working as in the skill.
- **Why `index.md` and not `README.md`:** a `README.md` would render on its own when the folder
  is opened on GitHub, like `docs/superpowers/specs/README.md`, but the name promises a document
  written for a human to read. This is a generated list for an agent to scan before opening an
  entry, so the name says what it is.
- **Why not keep the single file with a rule** ("always read it with `limit`", or fixed
  `<a id="N">` anchors): it fixes the over-read and the broken anchors, but leaves the manual
  numbering, the long offset reads and the git collisions, and it depends on the agent obeying.

**Don't "fix" by:**

- Merging the files back into one: brings back every cost above.
- Dropping `index.md` and relying on `ls`: a slug is a truncated title (`043-an-animes-identity-
  is-mediaid-status-is-a-per.md` against the full rule), too thin to tell whether a plan
  conflicts with an entry.
- Editing `index.md` by hand: it is generated from each file's first line, and
  `decision.sh check` fails when it drifts.
- Renaming `index.md` to `README.md` so GitHub shows it: it is not written for a human reader.
- Renaming a file when its title changes: every link to it uses the filename. Change the heading,
  run `decision.sh index`, leave the filename alone.
- Dropping the zero padding: `ls` and the GitHub view would sort `100` before `11`, and the
  `decisions/NNN` pointers in code would stop matching filenames.
