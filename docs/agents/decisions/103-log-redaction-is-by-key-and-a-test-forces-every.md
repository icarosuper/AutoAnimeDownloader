# 103. Log redaction is by key, and a test forces every new key to be classified

**The rule:** the bundle's log lines are JSON, and each field is redacted by its **key** through
`keyClasses` in `diagnostics/redact.go`: `keep`, `drop` (`remote_addr`), `pseudonymize` (account
names → `user1`, `user2`…, stable within one report) or `title` (anime names, torrent names, paths,
ids → `<title-N>` when the user unticks "include titles"). Free text — `message`, `error` and kept
strings — then goes through the text scrubber: webhook URLs and header values anywhere →
`<webhook-N>`, the home dir → `~`, URLs to hosts outside the allowlist → `<url>`, known usernames →
`userN`. `"HTTP request"` lines are dropped whole (they carry the LAN IPs of whoever opened the UI).

- **Why by key and not by regex over the text.** A title has no shape a regex can find; the key is
  the only reliable signal of what a value is. The text scrubber only catches what does have a shape
  (URLs, the configured webhooks, the configured usernames, the home path).
- **A key missing from the table is treated as a `title`** — the strictest class that still keeps
  the line readable. That's the runtime safety net; the real guard is the test.
- **`keys_test.go` (`TestEveryLogKeyIsClassified`)** greps every non-test `.go` file under `src/`
  (minus `src/tests`) for zerolog field calls with a literal key (`.Str("anime", …)`, `.Int(…)`,
  `.Interface(…)`…) and fails listing each key `keyClasses` doesn't have. A new log field can't
  reach a report without someone deciding what it is.

**Cost accepted:** a key built at runtime (not a string literal) escapes the test and falls back to
`title`. None exists today.

**Don't "fix" by:** defaulting unknown keys to `keep`, replacing the table with a regex over the
whole line, or silencing the test by adding a key as `keep` without checking what it carries.
