# 86. The Test button answers the service's status, not "the preset exists"

**Location:** `notifications/notifications.go` — `fireWebhook`, `FireTestWebhook`,
`ErrWebhookNotFound`; `api/endpoint_notifications.go` — `handleNotificationWebhookTest`.

**What it looks like:** `fireWebhook` returning `error` is useless, because the only path that
really fires — `fireBatch`, in a goroutine — ignores the return and already logs everything that
goes wrong. A `return` would be enough.

**Why it exists:** `POST /notifications/webhooks/{name}/test` answered `200 "Webhook fired
successfully"` for any preset that existed, and the front showed a success toast — with Jellyfin
returning `401` for a wrong token, or with the URL down. The only record of the failure was a
`warn` line in `daemon.log`, which nobody will read to know whether a Test button worked. Testing
delivery is the only reason that endpoint exists; without the return it only confirmed that the
name was spelled right.

Hence the split: `ErrWebhookNotFound` (sentinel, `errors.Is`) → `404`; any other error →
`502 WEBHOOK_FAILED`, with the service's status in the message. Automatic firing keeps ignoring
the return on purpose — there is nobody to report to, and the log already covers it.

**Don't "fix" by:** putting `fireWebhook` back to a `func(...)` with no return because "only the
test uses it"; or mapping a delivery failure to `404`, which would make the front say "webhook
not found" for a refused token.
