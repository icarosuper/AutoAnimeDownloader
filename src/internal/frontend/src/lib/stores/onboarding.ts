import { writable } from 'svelte/store'

/**
 * The first-steps card: dismissal and the steps the user marked as done. Same shape as
 * `theme.ts` and `locale.ts`: `localStorage` with the `isBrowser` guard.
 *
 * Per BROWSER on purpose, not fields in `config.json` — see decisions.md. A config field
 * would follow the user, but would put a UI preference into a schema the daemon reads and
 * would apply to every device at once.
 */
const DISMISSED_KEY = 'onboarding-dismissed'
const DONE_KEY = 'onboarding-done'

/** The order is the numbering on the card. */
export const ONBOARDING_STEP_IDS = ['library', 'source', 'check'] as const
export type OnboardingStepId = (typeof ONBOARDING_STEP_IDS)[number]

const isBrowser = typeof window !== 'undefined'

function createOnboardingDismissed() {
  const { subscribe, set } = writable<boolean>(
    isBrowser && localStorage.getItem(DISMISSED_KEY) === 'true',
  )

  return {
    subscribe,
    set: (value: boolean) => {
      if (isBrowser) localStorage.setItem(DISMISSED_KEY, String(value))
      set(value)
    },
  }
}

function readDone(): OnboardingStepId[] {
  if (!isBrowser) return []
  try {
    const parsed = JSON.parse(localStorage.getItem(DONE_KEY) ?? '[]')
    // Storage is hand-editable and survives versions: filtering by known ids keeps junk in
    // there from counting as a completed step (and making the card vanish on its own).
    return Array.isArray(parsed) ? parsed.filter((id) => ONBOARDING_STEP_IDS.includes(id)) : []
  } catch {
    return []
  }
}

/**
 * Marked by HAND, not derived from daemon state: steps 1 and 3 already started green on a
 * fresh install (the folder has a default and the pass runs on its own), which made the
 * tutorial look half-done before the user had read anything.
 */
function createOnboardingDone() {
  const { subscribe, update } = writable<OnboardingStepId[]>(readDone())

  return {
    subscribe,
    toggle: (id: OnboardingStepId) =>
      update((done) => {
        const next = done.includes(id) ? done.filter((d) => d !== id) : [...done, id]
        if (isBrowser) localStorage.setItem(DONE_KEY, JSON.stringify(next))
        return next
      }),
    reset: () => {
      if (isBrowser) localStorage.removeItem(DONE_KEY)
      update(() => [])
    },
  }
}

export const onboardingDismissed = createOnboardingDismissed()
export const onboardingDone = createOnboardingDone()
