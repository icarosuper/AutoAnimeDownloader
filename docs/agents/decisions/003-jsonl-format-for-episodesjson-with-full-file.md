# 3. JSONL format for `episodes.json` with full-file rewrite on save

**What it looks like:** The file is JSONL (one JSON object per line) but `saveEpisodesToFile` rewrites the entire file, not just appends new lines.

**Why it's right:** JSONL allows line-by-line parsing and provides backward compatibility with the old plain-text episode format (fallback parser in `parser.go`). Full rewrite on save is intentional: it avoids partial-write corruption — if a true append fails mid-write, the file is left in a mixed state. Read-modify-write with `WriteFile` is atomic at the OS level on the platforms we target.

**Don't "fix" by:** switching to true file-append (`os.O_APPEND`) — that breaks deduplication logic and leaves no room for deletions. Don't switch to a binary format — JSONL is human-readable for debugging.
