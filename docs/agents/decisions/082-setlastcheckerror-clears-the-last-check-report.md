# 82. `SetLastCheckError` clears the last-check report

**Location:** `daemon/state.go` — `SetLastCheckError`; `daemon/verification.go` — the order `SetLastCheckError(nil)` → `SetLastCheckReport(...)`.

**What it looks like:** an error setter that also wipes an unrelated field, and a `SetLastCheckReport` that **must** come after a `SetLastCheckError(nil)` that already ran three lines earlier. Looks like accidental coupling, and the "obvious" fix is to split the two.

**Why it's right:** `AnimeVerification`'s early exits (incomplete config, hardlink probe, backend not initialized, `Ensure`, id migration, AniList — there were seven until the `save_path` migration left, see #31) already call `SetLastCheckError`. With the clear inside it, **none of them needs a new line** — and none can forget. And the semantics are the right ones: a pass that aborted before looking at any anime has no per-anime report, it has `pass_error`. Without the clear, the screen would show the *previous* pass's problems side by side with a new pass error, suggesting both came from the same verification.

The second consequence is equally wanted: cancellation (`verification.go`, in the `ctx.Done()` `select`) calls `SetLastCheckError(nil)` and returns, so **an interrupted pass leaves no report** — it was incomplete, and a partial report would say "only this anime had a problem" when the others were not even looked at.

**Accepted trade-off:** the order of the two calls at the end of `AnimeVerification` is meaningful and is not obvious reading only that stretch. That is why there is a dedicated test (`TestSetLastCheckReport_AfterClearingError`) pinning exactly that order.

**Don't "fix" by:**
- Taking the clear out of `SetLastCheckError` and putting an explicit call on every early exit — seven places to forget one.
- Moving `SetLastCheckReport` before `SetLastCheckError(nil)` "because it reads better" — the report is wiped the next instant and the whole feature becomes silence.
- Persisting the report to disk to "survive restart" — `createStartFunc` calls `AnimeVerification` immediately on start (`loop.go`, before the first `time.After`), so it rebuilds in seconds; a file would cost persistence, migration and the chance of showing a report from days ago as if it were the last pass.
