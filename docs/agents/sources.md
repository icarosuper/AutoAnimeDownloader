# Fontes de torrent — levantamento

Leia antes de acrescentar uma fonte nova ao daemon. Registra o que foi **medido** em agosto/2026,
não o que se costuma dizer sobre cada site. Hoje o daemon tem uma fonte só: Nyaa, via
`internal/nyaa` (scraping de HTML com goquery).

O seam para acrescentar fonte já existe: a interface `nyaaSearcher`, injetada em
`daemon.processAnimeEpisodes` (ver `decisions.md` #14). Uma fonte nova é uma implementação nova,
não uma refatoração.

---

## Nyaa (fonte atual)

`https://nyaa.si/?f=0&c=1_2&q=<titulo>&s=seeders&o=desc`, paginado por `&p=N`.

**Limites que importam para o algoritmo:**

- **Não tem filtro de tamanho na query.** Nem `minsize`/`maxsize`. Quem filtra tamanho é o
  daemon, depois de baixar a página. (O `minage`/`maxage`/`minsize` que aparece em issues de
  scraper é do NyaaPantsu, um fork diferente, não do `nyaa.si`.)
- **`s=seeders&o=desc` é péssimo para pack de série longa.** A ordenação enterra pack de 40-100
  seeders sob centenas de episódios semanais com 200-1000 seeders. Medido em One Piece: o pack
  parcial mais útil (61 GiB) só aparece na **página 3**. Foi a causa do bug de "One Piece nunca
  baixa" — ver `decisions.md` #59.
- **Tem RSS** (`?page=rss`, mesmos parâmetros de query, mais tags próprias de seeders/leechers/
  infohash/size). Mais barato de parsear que HTML e imune a mudança de layout, mas devolve menos
  itens por requisição e **não** resolve o problema de ordenação acima. Não é upgrade automático.
- `f=1` filtra só "trusted", `f=2` filtra "no remakes". Hoje o daemon usa `f=0`.

---

## AnimeTosho — a melhor fonte nova candidata

**Atenção ao domínio.** `animetosho.org` congelou em maio/2026 e não indexa mais nada. A
continuação é `animetosho.xyz`, e a API/feeds migraram para `feed.animetosho.xyz` (os endpoints do
domínio antigo saíram em 1º/agosto/2026). Ambos verificados respondendo em agosto/2026.

Ele **indexa o próprio Nyaa** (campos `source: "nyaa"`, `nyaa_id`, `article_url`). Não é um acervo
concorrente — é um front-end melhor para o acervo que o daemon já usa.

### API

`GET https://feed.animetosho.xyz/json?q=<query>` → array JSON. Campos por item:

```
title, torrent_name, magnet_uri, torrent_url, info_hash,
seeders, leechers, total_size, num_files, timestamp,
nyaa_id, article_url, anidb_aid, anidb_eid, nzb_url
```

Mapeia quase 1:1 em `nyaa.TorrentResult` — some o goquery e some a fragilidade de contar `<td>`.

### Lista de arquivos do pack ANTES de baixar

`GET https://feed.animetosho.xyz/json?show=torrent&id=<id>` acrescenta `is_batch`, `files[]` e
`primary_file_id`. Cada item de `files[]` traz `filename`, `size` e `crc32`.

Isso responde "quais episódios este pack cobre e quanto pesa cada um" **sem** adicionar o torrent
e esperar o metadata chegar. É o dado que falta hoje para medir teto por episódio coberto em vez
de por tamanho total do pack.

**Ressalva medida:** entradas antigas vêm com `files: []`. O pack `[Judas] One Piece 575-782`
(id 369911) devolve lista vazia; um batch recente do SubsPlease devolve os 26 `.mkv` com nome e
tamanho. A lista de arquivos só é confiável para conteúdo recente — trate `files: []` como
"desconhecido", não como "pack vazio".

### O que NÃO resolve

Não permite baixar só alguns arquivos de um pack. Isso é limitação do **cliente**, não da fonte:
o `rain` v2.3.1 não tem seleção de arquivo (`Torrent.Files()`/`FileStats()` são read-only, e não
existe nada de prioridade/skip por arquivo no módulo). Seleção por episódio dentro de pack exigiria
fork do `rain` ou troca do cliente.

---

## Outras fontes avaliadas

| Fonte | Veredito |
|---|---|
| **AniDex, TokyoTosho** | Mesmos uploaders, mesmo conteúdo do Nyaa. Ganho marginal quase nulo. Não vale o código. |
| **RSS de fansub direto** (SubsPlease, Erai-raws) | Sinal limpíssimo e sem ruído para a temporada corrente; **inútil para backlog** (não hospedam catálogo antigo). Serve como "fonte preferida" para anime em exibição, não como fonte geral. |
| **Usenet / NZB** | O AnimeTosho carrega `nzb_url`. Velocidade de HTTP, sem problema de seeder, retenção longa. Exige provider pago + indexer + um caminho de download inteiro em paralelo ao `rain`. Mudança grande. |
| **Debrid** (Real-Debrid, AllDebrid) | Manda o magnet, o serviço baixa do lado dele, e você escolhe **quais arquivos** puxar por HTTP. É a única saída que dá seleção por episódio dentro de pack sem tocar no `rain`. Custa mensalidade e vira dependência externa. |

---

## Ordem recomendada

1. **Corrigir a busca de pack no Nyaa** — feito, ver `decisions.md` #59. Era bug, não falta de
   fonte.
2. **AnimeTosho como segunda fonte** — JSON em vez de HTML, e a lista de arquivos do pack. Maior
   ganho por linha escrita.
3. **Debrid/Usenet** — só se seleção por arquivo virar requisito de verdade.
