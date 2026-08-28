# Presets de codec e limpeza dos defaults de prioridade

**Data:** 2026-08-28
**Status:** implementado em 2026-08-28
**Origem:** investigação de legenda dessincronizada no Jellyfin (ver "O caminho até aqui")
**Escopo:** `frontend/routes/Priorities.svelte` (presets, rótulos), `internal/nyaa`
(canonicalização de resolução, defaults), `internal/files/filemanager.go` (defaults numéricos).
Sem mudança de API, sem `swag init`, sem campo novo em `config.json`.

---

## O caminho até aqui

O desenho nasceu de um sintoma que não parecia config: legenda 2–8 segundos **adiantada** no
Jellyfin, com deslocamento estático, em alguns episódios e não em outros — e às vezes num episódio
que já tinha sido assistido sem problema.

A cadeia real:

1. Alguns releases vêm em HEVC/x265 (Judas, Ember); outros em H.264 (SubsPlease, Erai-raws).
2. O Chrome não decodifica HEVC (licenciamento). O app do Android decodifica.
3. Sem decode no cliente, o Jellyfin **transcodifica**, o que queima a legenda ASS no vídeo.
4. No caminho de transcode, `start_time` não-zero do container desloca a legenda em relação ao
   vídeo — offset estático, de poucos segundos, variando por arquivo.

Confirmado no overlay de reprodução do Jellyfin: `Transcodificação — motivo: o codec de vídeo não é
suportado`, e reprodução direta no Android com o mesmo arquivo.

O usuário não tem como saber que "x265" significa "meu navegador vai transcodificar e a legenda vai
dessincronizar". Essa é a lacuna que os presets fecham. **O AAD nunca toca em legenda** — o
`librarian` só cria hardlink do arquivo de vídeo (`internal/files/librarian.go:75-83`) —, então
nada aqui é pós-processamento de mídia; é escolha de release.

Uma correção já foi feita fora deste spec: `codec` entrou em `episodeCriteria`
(`internal/nyaa/priorities.go:200`) com a guarda de `codecCompare` (`:143`), documentada em
`decisions.md #69`. Sem ela a lista de codecs não afetava episódio nenhum, e este spec não teria
alavanca.

---

## Decisões, e o que foi descartado

**Preset é carimbo de uma vez, não modo guardado.** Nada novo em `config.json`, nenhum estado
"custom", nenhuma regra de precedência. Depois de aplicado vira lista comum e editável.

**Preset vive no frontend, não no backend.** Só o frontend consome (o CLI não menciona priorities).
Um endpoint existiria para se servir a si mesmo: ~60 linhas em seis arquivos, mais mudança de forma
da resposta e `swag init`, contra ~30 linhas num arquivo só.

**Preset reordena, não carimba array literal.** Consequência do ponto anterior, e melhor que o
desenho original: só promove tokens que já estão na lista do usuário. Typo vira no-op em vez de
config inerte; token que o usuário adicionou à mão desce em vez de sumir; os tokens canônicos
continuam existindo num lugar só (`reCodecPatterns`).

**Só `codecs` ganha presets.** Percorridas as sete listas: `resolutions` tem default certo e duas
setas na UI; `fansubs` é gosto, não eixo técnico; `sources` e `audio` não afetam episódio;
`criteria_order` é arma carregada (`decisions.md #55`); `ignore_list` é aditiva, não ranking.

**Descartado: "Priorizar qualidade".** Era a ideia original e não se sustenta neste domínio.
SubsPlease e Erai-raws fazem **remux** do stream do Crunchyroll/HIDIVE, sem reencode: para anime em
exibição isso já é o teto da fonte, e qualquer arquivo maior do mesmo episódio é reencode (perda de
geração) ou upscale. O app já barra o primeiro caso — `re-encode` está no `ignore_list` default.
Para backlog o eixo existe, mas é **fonte** (BD vs WEB), não compressão: um encode x265 10-bit
competente de BD é visualmente indistinguível de um BDRemux 4× maior. Somado a
`DownloadStatuses: [CURRENT, REPEATING]`, um preset de qualidade não teria o que escolher no loop
principal.

**Descartado também:** preset em `criteria_order`; campo de direção de tamanho
(`size_preference`) para inverter `sizeCompare`; migração automática de `config.json` existente.

---

## 1. Presets de codec

`Priorities.svelte`, junto do `LISTS` que já existe (`:13`):

```ts
type ListPreset = { key: string; label: string; desc: string; first: string[] }

const PRESETS: Partial<Record<keyof Priorities, ListPreset[]>> = {
  codecs: [
    { key: "compat", label: "Prefiro compatibilidade",
      desc: "H.264 primeiro. Toca direto em qualquer player, sem transcode no servidor — a legenda continua soft. Arquivos maiores.",
      first: ["h.264"] },
    { key: "space", label: "Prefiro arquivos menores",
      desc: "AV1/HEVC primeiro. Até metade do tamanho na mesma qualidade, mas exige player que decodifique — no navegador vira transcode.",
      first: ["av1", "hevc"] },
  ],
}

function applyPreset(key: keyof Priorities, first: string[]) {
  if (!config) return
  const cur = config.priorities[key]
  const promote = first.filter((v) => cur.includes(v))
  config.priorities[key] = [...promote, ...cur.filter((v) => !promote.includes(v))]
  config = config
}
```

Na renderização (`:107`, dentro do `{#each LISTS}`), quando `PRESETS[key]` existir: uma linha de
botões acima dos itens, cada um com `title={p.desc}` e `on:click={() => applyPreset(key, p.first)}`.
Abaixo da lista Codec, uma linha de ajuda **fixa** com o mesmo tradeoff em uma frase — tooltip não
existe em touch, e o Jellyfin desta casa é assistido no celular.

Salvar continua sendo o botão que já existe; `applyPreset` só mexe no estado local, como
`resetList` (`:67`).

## 2. Canonicalizar resolução

`extractCodec` canonicaliza (`x265` → `HEVC`); `extractResolution` (`internal/nyaa/nyaa.go:907`)
devolve a captura crua. Essa inconsistência **é** o bug: `4k` e `2160p` são a mesma resolução e
hoje ocupam os índices 3 e 8 do default, então qual vence depende só de como o grupo escreveu o
nome do torrent. `1920x1080` é capturável pelo regex e não existe na lista, ranqueando como o pior.

`extractResolution` passa a mapear nos canônicos — `4k`/`uhd` → `2160p`, `1920x1080`/`fhd` →
`1080p`, `hd` → `720p` — usando os padrões que já estão em `nyaa_regex.go:99-110`. A lista default
encurta para cinco itens legíveis, e a duplicata deixa de ser representável.

`8k` sai da lista **de propósito**, não por esquecimento: o padrão `\b(8K)\b` continua existindo e
o token passa a ranquear como o pior, que é o tratamento certo para uma resolução que não existe em
anime.

Isto **não quebra config existente**. A lista antiga já contém `1080p`, `720p`, `480p` e `2160p`,
que são exatamente os canônicos; as entradas `4k`, `uhd`, `fhd`, `hd` e `8k` viram inalcançáveis mas
inofensivas, e um release 4K de um usuário antigo passa a casar com o `2160p` que ele já tem.

## 3. Defaults

| Campo | Hoje | Proposto | Motivo |
|---|---|---|---|
| `Codecs` (`priorities.go:39`) | `hevc, av1, x265, h.264, x264, xvid` | `h.264, hevc, av1, xvid` | Compat como default seguro. `x265`/`x264` são **inertes**: `extractCodec` nunca devolve esses tokens |
| `Resolutions` (`:37`) | 10 itens, com `4k`/`2160p` duplicados e `480p` acima de ambos | `1080p, 720p, 1440p, 2160p, 480p` | Seção 2. 4K abaixo de 720p é deliberado: anime em 4K é quase sempre upscale de master 1080p |
| `Sources` (`:38`) | `bd, bdrip, bdremux, …` | `bdremux, bd, bdrip, …` | BDRemux é o bitstream intocado, BDRip é reencode — a ordem estava invertida |
| `MaxEpisodeTorrentSizeGB` | **ausente** de `getDefaultConfig()` → `0` → ilimitado (`daemon/search.go:44`) | `5` | Passa WEB-DL (~1,4 GB) e encode de BD (~2 GB), barra BDRemux (~6–8 GB). Batch tem teto de 100 GB e episódio não tinha nenhum |
| `WatchedEpisodesToKeep` | `0` | `2` | `DeleteWatchedEpisodes` é `true` de fábrica: hoje o episódio some assim que é marcado. Perdoa "marquei errado" e "quis rever" |
| `MinSeeders` | `1` | `3` | 1 seeder trava com frequência |

## 4. Marcar o que é inerte

`sources` e `audio` aparecem editáveis na tela e **não afetam download de episódio** — só
`SortMovieResults` as lê. O usuário reordena, salva, e nada acontece. É a mesma falha que `codecs`
tinha até `decisions.md #69`.

`LISTS` (`Priorities.svelte:13`) — hoje `{ key, label }` — ganha um terceiro campo opcional
`scope?: string`, renderizado ao lado do `label` quando presente; `sources` e `audio` recebem
`scope: "só filmes"`. `criteria_order` ganha uma linha de texto embaixo dizendo que as entradas
`source` e `audio` não valem para episódio. Linha de texto em vez de badge por item: resolve a
confusão pelo mesmo preço, com menos DOM.

---

## Testes

**O teste que mais importa** é o que torna a classe do bug do `x264` impossível: todo token nas
listas default de `Priorities` precisa ser algo que `extractCodec`/`extractResolution` consigam
devolver. Config inerte deixa de passar no CI. Vive em `src/tests/unit/priorities_test.go`.

Mais:

- Canonicalização de resolução: `4K`, `UHD` e `2160p` produzem o mesmo token; `1920x1080` e `FHD`
  produzem `1080p`.
- `getDefaultConfig()` traz `MaxEpisodeTorrentSizeGB` diferente de zero — o teste existe porque a
  ausência do campo era invisível justamente por ser um zero value válido.
- Frontend (Vitest): `applyPreset` promove só o que existe na lista, preserva token adicionado à
  mão e é no-op para token inexistente.

Não há teste de endpoint porque não há endpoint novo.

## Docs

- `config.md`: linhas de `Priorities.Codecs`, `Priorities.Resolutions`, `Priorities.Sources`,
  `MaxEpisodeTorrentSizeGB`, `WatchedEpisodesToKeep`, `MinSeeders`.
- `decisions.md`: entrada nova cobrindo (a) por que a canonicalização vive em `extractResolution` e
  não na ordem da lista, e (b) por que os presets são reordenação no frontend em vez de dado
  servido pelo backend.
- Sem `swag init`: nenhuma resposta de API muda.

## Limitação conhecida

**Nenhuma mudança de default alcança instalação existente.** `LoadConfigs` desserializa por cima de
`getDefaultConfig()`, então um `config.json` que já existe mantém o que tem — comportamento
deliberado e já documentado em `config.md`. Na prática:

- As listas de Priorities se atualizam pelos botões "resetar" que já existem na tela.
- Os quatro campos numéricos (`MaxEpisodeTorrentSizeGB`, `WatchedEpisodesToKeep`, `MinSeeders`, e
  `Codecs` se o usuário não resetar) precisam de uma edição manual, uma vez.

Não vai haver migração automática. Escrever em config existente sem o usuário pedir é o que
`decisions.md #47` já rejeitou para `BatchWindowSeconds`, pelo mesmo motivo: mudar comportamento
num update sem ninguém pedir.
