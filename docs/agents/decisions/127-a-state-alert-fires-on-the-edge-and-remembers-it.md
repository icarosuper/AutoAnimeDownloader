# 127. A state alert fires on the edge and remembers it in notification_state, not every pass the state holds

**Location:** `internal/daemon/health.go` — `trackCheckFailing` (and every later state alert);
`internal/notifications/state.go` — `AlertState`, `UpdateState`. Pinned by
`TestTrackCheckFailing` and `TestTrackCheckFailing_SurvivesRestart`.

**What it looks like:** a file and a small state machine just to send a notification. The obvious
version is "if the condition holds at the end of the pass, `Notify`".

**Why it's right:** the alert events (checks failing, disk low, an episode stuck…) are **states**,
true on every pass while they last. Firing on the condition sends one push per `check_interval` —
the same ntfy.sh quota blow-up [#47](047-notifications-are-grouped-by-window-and-the-body.md) measured for episodes. Each state alert therefore fires
when the state **starts** and re-arms only when it **ends**, and what was already said lives in a
data-dir file so a restart neither repeats the push nor forgets to send `check_failing`'s recovery.
The file is disposable: deleting it costs at most one round of repeated notifications, so a corrupted
one starts empty with a warning instead of failing the boot. `check_failing` is counted from a defer
in `AnimeVerification`, so a pass started by `POST /check` counts too, and a cancelled pass (which
clears the error) never reads as a recovery.

The first finished pass that finds the store without `seeded` **records without firing** the
alerts whose state may be months old (an anime at the episode limit, a finished anime already in
the library): without it, the upgrade's first pass would announce the whole library at once. Alerts
whose age is itself the news (a backlog of 60 days) are not seeded.

**Don't "fix" by:**
- Firing on the condition and relying on the batch window: the window only groups one minute; the
  state lasts hours.
- Keeping the edge in memory only: every restart (and an update is one) re-announces every state,
  and an outage that spans a restart never sends "back to normal".
- Moving the thresholds to `config.json` preemptively: they are `ponytail:` constants until
  someone asks for a different number.
