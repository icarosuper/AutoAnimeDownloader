# 64. The first-steps card steps are ticked by hand, not derived from daemon state

**Location:** `src/internal/frontend/src/lib/stores/onboarding.ts` (`onboardingDone`); painted by `routes/Status.svelte`.

**What it looks like:** the app **knows** whether the folder is configured and whether the pass has already run — `lib/domain/onboarding.ts` computes exactly that — and even so the card's checkmarks come from a list stored in `localStorage` that the user fills by clicking. Looks like duplicated state, and the "obvious" fix is to derive the three from data the screen already has.

**Why it's right:** that is what the first version did, and on a new install steps ① and ③ were born green on their own — the library folder has a default and the verification pass runs every 10 minutes with nobody asking. The tutorial appeared two thirds done before the user had read the first line, which at once lies about what they did and takes away the reason to read the rest. Ticking is confirmed reading, not disk state.

The derivation still exists, but with another role: `allDone(onboardingSteps(...))` is the gate of "this install is already configured and running, don't teach the obvious", so whoever already uses the app never sees the card. Two uses, one source of truth each.

**Don't "fix" by:**
- Deriving the checkmarks again "to not duplicate state" — that is exactly the bug this fixes.
- Ticking the step automatically when its action is done in the app (configured the folder → tick ①) — back to the same place for whoever already had the folder configured, and makes the card move on its own while it is being read.
- Swapping the numbers for status icons — the numbering is what says there is an order; the number doubles as a tick box precisely so there are not two markers for one state.
