# 109. Flags after positionals are reordered before urfave parses

**Location:** `cmd/cli/args.go` (`reorderInterspersedFlags`); `run` in `main.go` is the only
entry that tests and `main` use.

**The rule:** Go's `flag.FlagSet.Parse` stops at the first non-flag argument. urfave/cli v2
uses that parser, so `animes add 21 --magnet URI` never sets `--magnet` — leftover tokens fail
`requireArgs`, or a bool flag after the id is silently ignored. `run` rewrites argv before
`App.Run`: command names stay in order, global flags move to the front, each command's flags
sit immediately after that command. `--` still ends flag parsing. A new CLI flag is picked up
from the command tree; do not maintain a second list of names. `--help` / `-h` is the exception
to "picked up from the tree": urfave adds it at runtime, so `indexFlags` registers it on every
scope as a command flag — it stays after the command it was typed after — and, once help is
asked for, the positionals are dropped (urfave would read `21` in `animes add 21 -h` as a help
topic).

**Why:** the documented order is `animes add <id> --magnet` / `candidates <id> --query` /
`replace <id> --magnet --yes`. Putting flags first works without this; Unix tools accept both.

**Don't "fix" by:** treating `--help` as global: moved to the front, `animes add --help`
became `--help animes add` and printed a truncated help of `animes`, with no subcommands and no
options; telling users to put flags first; calling `newApp().Run` from tests (that
skips the reorder); `SkipFlagParsing` (the leaf would stop seeing `--magnet` at all); bumping
to urfave v3 only for interspersed flags.
