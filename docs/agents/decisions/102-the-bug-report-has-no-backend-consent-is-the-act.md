# 102. The bug report has no backend: consent is the act of attaching the file

**The rule:** nothing in the app sends a report anywhere. The daemon builds a redacted bundle
locally (`GET /api/v1/diagnostics`), the report screen shows it in full, downloads it as a `.txt`
and opens the GitHub issue form pre-filled through the URL. The user attaches the file by hand.

- **Why no backend.** An upload endpoint is a server to run, a place where logs of other people's
  machines pile up, and an abuse target — for a self-hosted app with no account system. GitHub
  already has identity, moderation and a conversation with the reporter.
- **Why the file isn't in the URL.** GitHub cuts long URLs off, and a URL ends up in browser
  history and server logs. Only the short `environment` summary and the user's own text go there
  (the text truncated to a 4000-char encoded budget, `report.ts`).
- **Consent = attaching.** The preview is exactly the file (the titles checkbox regenerates both),
  and the screen says the issue is public. There is no separate "I agree" step to keep in sync.
- **Reading from disk, not from the daemon** (`diagnostics` package): the same bundle can be built
  by a CLI command with the daemon dead (planned for `v2.6.0`).

**Cost accepted:** a user who doesn't attach the file sends an issue with only the environment
summary. The issue form's required "I attached the report file" checkbox is the only nudge.

**Don't "fix" by:** adding an upload/"send report" button, putting the bundle in the issue URL, or
having the endpoint read state from the running daemon instead of the files.
