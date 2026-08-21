import { writable } from 'svelte/store'

/**
 * O card de primeiros passos: a dispensa e os passos que o usuário marcou como feitos. Mesmo
 * molde de `theme.ts` e `locale.ts`: `localStorage` com a guarda `isBrowser`.
 *
 * É por NAVEGADOR de propósito, e não campos no `config.json` — ver decisions.md. Um campo de
 * config seguiria o usuário, mas poria preferência de UI num schema que o daemon lê e valeria
 * para todos os dispositivos de uma vez.
 */
const DISMISSED_KEY = 'onboarding-dismissed'
const DONE_KEY = 'onboarding-done'

/** A ordem é a numeração do card. */
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
    // Storage é editável à mão e sobrevive a versões: filtrar pelos ids conhecidos evita que
    // lixo ali dentro conte como passo feito (e faça o card sumir sozinho).
    return Array.isArray(parsed) ? parsed.filter((id) => ONBOARDING_STEP_IDS.includes(id)) : []
  } catch {
    return []
  }
}

/**
 * Marcado à MÃO, não derivado do estado do daemon: os passos 1 e 3 já nasciam verdes numa
 * instalação nova (a pasta tem default e o passe roda sozinho), o que fazia o tutorial parecer
 * meio pronto antes de o usuário ler qualquer coisa.
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
