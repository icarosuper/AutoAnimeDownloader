# 63. Dismissing the first-steps card lives in `localStorage`, not in `config.json`

**Location:** `src/internal/frontend/src/lib/stores/onboarding.ts`; consumed by `routes/Status.svelte` (render condition) and `routes/Config.svelte` (the "Show again" button).

**What it looks like:** a user preference the whole app honours, stored in the browser instead of in the file the daemon already reads and writes. Looks like an inconsistency — the rest of configuration is in `config.json`, and the "obvious" fix is to create a `tutorial_dismissed` field there.

**Why it's right:** it is not daemon configuration; it is a UI preference, and the daemon has nothing to do with it. A field in `config.json` would enter the schema the server validates, the `PUT /config` body and — if someone were consistent — the Settings screen's `requiredChecks`, three places for a boolean only the browser consumes. And it would apply to **every** device at once: whoever dismissed it on desktop would never see the card on the phone, where they still do not know the app. `localStorage` is per browser, which is exactly the right scope, and it is the same choice `theme` and `locale` already make in this frontend.

**Accepted trade-off:** switching browsers brings the card back, and clearing the site's storage does too. For a single-user self-hosted app, seeing a three-line card again is cheap; the inverse — not being able to see it on a new device — is the expensive one.

Hiding the card with an empty library leaves the app inoperable, and even so the dismiss is permanent: the verification pass still opens `#/config?missingConfig=true` and the banner there still appears. That backstop is what makes a permanent dismiss safe to offer.

**Don't "fix" by:**
- Creating `tutorial_dismissed` in `config.json` "to follow the user" — puts a UI preference in a daemon schema and unifies devices that should be independent.
- Swapping the "Don't show again" button for an `×` "because that's the card pattern" — on a three-item card the `×` reads as "close until reload", and the behaviour is permanent. The label is the documentation of the behaviour.
- Making the card vanish on its own after N days/visits — it goes away when the user ticks the three, when they dismiss, or when the install is already configured and running; time means nothing.
