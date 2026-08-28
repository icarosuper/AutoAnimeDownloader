import type { Priorities } from "../api/client.js"

/**
 * Preset é carimbo de uma vez, não modo guardado: aplica, vira lista comum e editável,
 * e o botão "Salvar" que já existe persiste. Nada novo em config.json.
 *
 * Reordena em vez de carimbar um array literal — só promove tokens que o usuário já tem.
 * Assim um token adicionado à mão desce em vez de sumir, e os tokens canônicos continuam
 * existindo num lugar só (reCodecPatterns, no backend).
 */
export type ListPreset = { key: string; label: string; desc: string; first: string[] }

export const PRESETS: Partial<Record<keyof Priorities, ListPreset[]>> = {
  codecs: [
    {
      key: "compat",
      label: "Prefiro compatibilidade",
      desc: "H.264 primeiro. Toca direto em qualquer player, sem transcode no servidor — a legenda continua soft. Arquivos maiores.",
      first: ["h.264"],
    },
    {
      key: "space",
      label: "Prefiro arquivos menores",
      desc: "AV1/HEVC primeiro. Até metade do tamanho na mesma qualidade, mas exige player que decodifique — no navegador vira transcode.",
      first: ["av1", "hevc"],
    },
  ],
}

/** Devolve `list` com os itens de `first` que ela contém promovidos ao topo, na ordem de `first`. */
export function applyPreset(list: string[], first: string[]): string[] {
  const promote = first.filter((v) => list.includes(v))
  return [...promote, ...list.filter((v) => !promote.includes(v))]
}
