import { writable, derived } from 'svelte/store'

/**
 * Saúde do NOSSO backend, derivada só das respostas que o app já faz. Não existe endpoint de
 * health check: um daemon que responde 200 em `/status` está são por definição, e um que não
 * responde não responderia a um endpoint novo também.
 *
 * `unreachable` = a requisição nem chegou (daemon caiu, rede fora). `server_error` = chegou e
 * voltou 5xx, que é bug nosso. 4xx NÃO conta: 404, 405 e o 409 do POST de avulso são respostas
 * legítimas do backend funcionando, e tratá-las como degradação poria banner na tela toda vez
 * que o usuário tentasse adicionar um anime repetido.
 */
export type BackendState = 'ok' | 'unreachable' | 'server_error'

/**
 * Falhas CONSECUTIVAS exigidas antes de acusar. Uma falha isolada num poll de 5s é ruído de
 * rede; o banner existe para estado que persiste (ver decisions.md #66). É também o que permite
 * conviver com `silent: true` sem reintroduzir o spam de toast que essa opção existe para evitar.
 */
const FAILURES_BEFORE_ALARM = 2

/**
 * Contadores por endpoint, e não um contador global: um `/last-check` quebrado enquanto
 * `/status` responde é bug nosso e tem que aparecer. Com contador único, o sucesso de um
 * endpoint apagaria a falha do outro.
 */
const failures = new Map<string, { count: number; state: Exclude<BackendState, 'ok'> }>()

const store = writable<BackendState>('ok')

function recompute() {
  let next: BackendState = 'ok'
  for (const f of failures.values()) {
    if (f.count < FAILURES_BEFORE_ALARM) continue
    // Inalcançável ganha de 5xx: com o daemon fora do ar, o 5xx que sobrou no contador é
    // informação velha da última vez em que ele ainda respondia.
    if (f.state === 'unreachable') {
      next = 'unreachable'
      break
    }
    next = 'server_error'
  }
  store.set(next)
}

export const backendHealth = {
  subscribe: store.subscribe,

  /** Uma resposta útil zera o endpoint. O banner some no primeiro sucesso, nunca por timer. */
  recordSuccess(endpoint: string) {
    if (failures.delete(endpoint)) recompute()
  },

  /** `status` 0 significa que o fetch nem chegou a receber resposta. */
  recordFailure(endpoint: string, status: number) {
    if (status !== 0 && status < 500) {
      // Resposta legítima de um backend são: não é degradação, e não deve manter aceso um
      // alarme anterior deste endpoint.
      this.recordSuccess(endpoint)
      return
    }
    const state = status === 0 ? 'unreachable' : 'server_error'
    const prev = failures.get(endpoint)
    failures.set(endpoint, { count: prev && prev.state === state ? prev.count + 1 : 1, state })
    recompute()
  },

  /** Só para testes: zera o estado do módulo entre casos. */
  reset() {
    failures.clear()
    store.set('ok')
  },
}

/** Conveniência para condições de render. */
export const backendDegraded = derived(store, ($s) => $s !== 'ok')
