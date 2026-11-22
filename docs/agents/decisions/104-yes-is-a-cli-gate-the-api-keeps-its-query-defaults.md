# 104. --yes is a CLI gate; the API keeps its query defaults

**The rule:** destructive CLI commands (`animes remove` / `redownload` / `delete` / `replace` /
`replace-episode`, `torrents delete`) refuse without `--yes`: exit 2, code `CONFIRMATION_REQUIRED`,
message lists the flags. No stdin, no prompt, no TTY check. After `--yes` they call the same
endpoints with the same query defaults (`parseBoolQueryParam`: missing → `false`). `--keep-data`,
`--block` and `--delete-episodes` map 1:1 onto those params; absent means `false`.

The web UI keeps its dialog. The API does not grow a confirmation parameter. An agent cannot hang
on stdin, and changing API defaults to match the web dialog would change the UI's "unchecked means
false" contract too.

**Don't "fix" by:** prompting when stdin is not a TTY, defaulting `--yes` on in scripts, or adding
`confirm=true` to the API.
