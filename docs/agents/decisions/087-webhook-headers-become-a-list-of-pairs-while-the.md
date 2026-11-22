# 87. Webhook headers become a list of pairs while the form is open

**Location:** `frontend/src/routes/Notifications.svelte` — `headerRows`, `toRows`, `fromRows`,
`confirmWebhook`. The on-disk format did not change: `WebhookPreset.headers` stays
`Record<string,string>` in the API and in `config.json`.

**What it looks like:** a useless conversion. The `Record` is already in hand, you can iterate
with `Object.entries` and write straight into `newWebhook.headers` — that is what the code did.

**Why it exists:** a `Record` is not editable keystroke by keystroke. Renaming a key is deleting
the entry and creating another, so each letter typed in the *Header* field rebuilds the object,
the row changes position in the iteration and the cursor jumps. That is why the old version only
had a pair of "pending" fields and a `+` button: without a list of pairs, already-saved headers
could not be inputs.

The `+` was the real trap — whoever typed in both fields and clicked **Confirm** lost the header
in silence, and the webhook went to disk with the `CHANGE_API_KEY` placeholder (that is how a
Jellyfin kept taking `401` with nobody understanding). With pairs, every header is an input, `+`
only adds an empty row and Confirm writes what is on the screen.

`fromRows` drops a row with an empty key: `+` creates a blank row, and giving up on filling it
must not become a `""` header. Covered by `tests/component/Notifications.headers.test.ts`.

**Don't "fix" by:** swapping `headerRows` for a direct bind on the `Record` "to drop the
conversion"; nor changing the saved format to a list of pairs, which would break everyone's
`config.json` and Go's `files.WebhookPreset` for no gain.
