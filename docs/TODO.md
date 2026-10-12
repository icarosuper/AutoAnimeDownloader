# TODO

Agrupado por release: cada seção é **uma** release, e o bump é o maior bump das tasks dentro dela —
task que não mexe no binário (doc, coisa fora do repo) pega carona sem mudar o número.
Versão atual: `v2.3.0`.

A ordem segue um critério só: **nada que carimba o nome do projeto sai antes do rename**
(instalador do Windows, imagem no ghcr, README, social preview, landing page), senão é migração
dupla. Se a apresentação do repositório para empregadores apertar, o `v3.0.0` pode subir para logo
depois do `v2.4.0` — bug report e paridade da CLI não aparecem para quem só olha o repositório.

> A antiga Etapa 1 (fechar tetos do que já roda) fechou em 30/ago/2026 **sem código**: os três
> itens foram medidos e os três disseram para não mexer. Numeração de pack por pasta e teto de
> pack por episódio em [decisions.md #84](agents/decisions.md#84-a-cobertura-de-um-pack-sem-faixa-no-nome-vem-da-lista-de-arquivos-e-não-da-suposição-de-que-ele-cobre-tudo)
> e [sources.md](agents/sources.md); custo do frontend na AniList em
> [decisions.md #85](agents/decisions.md#85-o-custo-do-frontend-na-anilist-é-por-conta-e-por-órfão-não-por-aba),
> que fecha a reavaliação da [#73](agents/decisions.md#73-o-frontend-não-busca-direto-na-anilist-mesmo-podendo).
> O instrumento das duas primeiras ficou em `nyaa/live_pack_measure_test.go` (`AAD_LIVE_NYAA=1`).

> O **cache local dos animes** fechou em 07/set/2026: a AniList é servida de um snapshot em disco
> quando cai, com o snapshot voltando com TTL zero para nenhuma tela ficar mais velha do que era
> ([#92](agents/decisions.md#92-o-cache-em-disco-da-anilist-volta-com-ttl-zero-e-o-valor-servido-dele-vem-acompanhado-de-errfromcache)).
> As capas vieram de graça (a URL é campo de `Media`, já no snapshot) e o passe em modo cache
> deleta, por decisão explícita — as duas coisas, e o que mudou em relação ao levantamento, estão
> no fim da [spec](superpowers/specs/2026-09-05-cache-local-anilist-design.md).

## `v2.4.0` — integração com MyAnimeList

- Adicionar integração com MyAnimeList

## `v2.5.0` — bug report

- Mecanismo de bug report — precisa existir bem antes de divulgar, senão o feedback chega sem
  contexto

## `v2.6.0` — paridade da CLI

- Parear features do webApp na cli — depois do MyAnimeList, pra parear já com ele dentro

## `v3.0.0` — Serval

Tudo que carimba o nome e a cara do projeto, numa release só. O major vem do rename, e o descriptor
do Nyaa vem junto pra que quem atualiza passe por **uma** migração, não duas.

- Renomear o projeto para **Serval** — nome do módulo Go, binários (`daemon`/`cli`), repo no GitHub,
  `config.json` e diretório de dados do usuário, imagem do ghcr, títulos do webApp e do Swagger.
  Quebra o caminho de dados de quem já usa → precisa de migração ou nota de release explícita
- **Tirar o Nyaa do repositório (fonte como descriptor externo)** — spec desenhada em 13/set e nunca
  executada: [fonte-como-descriptor](superpowers/specs/2026-09-13-fonte-como-descriptor-design.md).
  Objetivo é **publicabilidade**, não múltiplas fontes: o endereço, a query e o layout do Nyaa saem
  do binário para um JSON que o usuário põe ao lado do `config.json`. Plugin compilado em Go está
  fora (o pacote `plugin` é Linux-only e exige mesmo compilador e mesmas deps). O tratamento **não**
  vai no descriptor: das 2217 linhas de `internal/nyaa`, ~150 são do Nyaa e ~2050 são parsing de
  nome de release, igual em qualquer fonte. O grosso do diff é a quebra em `internal/release` +
  `internal/source` (21 arquivos importam o pacote, 277 ocorrências de `nyaa.`), e as etapas 1-2 da
  spec valem sozinhas — dá pra adiantá-las numa minor, já que não mudam comportamento. Quem atualiza
  sem o descriptor fica sem fonte: a migração do rename cobre isso
- Mexida na UI/UX do webApp
- Identidade visual — logo, paleta, favicon; alimenta o social preview e a landing page
- Publicar imagem no ghcr — maior ganho das opções medidas em [distribuicao.md](distribuicao.md),
  o Dockerfile já existe. O `install.sh` via curl é o passo seguinte de lá, se fizer falta.
  `go install`, Homebrew, Scoop, .deb/.rpm e Flatpak avaliados e descartados
- Atualizar README — depois da imagem, porque é ela que muda as instruções de instalação
- Subir social preview no GitHub — hoje o card de link compartilhado é o genérico do GitHub.
  Settings > General > Social preview > Upload, 1280x640, usar a identidade visual nova. Só existe
  na web, `gh repo edit` não cobre
- Criar landing page

## `v3.1.0` — proper release no Windows

Depois do rename: instalador, nome do serviço e caminho de dados carimbam o nome, e os ajustes de
UI pra Windows seriam refeitos pela mexida de UI/UX do `v3.0.0`.

- Autenticar app com conta Microsoft
- Instalar ao invés de rodar de arquivo executável
- Dar detach e rodar como serviço no background
- Fazer mudanças de ui pra funcionar melhor no Windows
- Testar bem

## Sem release — divulgar

Depois do `v3.1.0`: divulgar sem Windows corta boa parte do público.

## Fixes sem release fixa — baixar direito anime velho e longo

Sai como patch (`v2.x.y`/`v3.x.y`) quando for atacado. O sintoma é um só: **anime velho ou longo
não baixa**. Anime da temporada corrente baixa sem problema. A causa não é achar candidato — é que
o candidato ou está morto (0-2 seeders, barrado por `min_seeders` ou travando em 0%) ou é um pack
gigante que o teto de tamanho reprova. Ordem não é preferência: o primeiro item é um **gate**, e o
resultado dele decide qual das saídas do segundo existe.

> Os três itens de **higiene do download** fecharam em 07/set/2026, sem depender do gate: a guarda
> de disco passou a receber o tamanho do torrent
> ([#93](agents/decisions.md#93-a-guarda-de-disco-tem-dois-gatilhos-e-o-de-tamanho-vale-mesmo-com-min_free_disk_percent--0)),
> a falha de torrent passou a preservar os bytes já baixados (emenda na
> [#24](agents/decisions.md#24-a-failed-torrent-is-dropped-from-the-session-and-re-added-by-the-next-pass--no-blacklist))
> e torrent a zero peers por mais de uma hora passou a ser derrubado pelo mesmo caminho de falha
> ([#94](agents/decisions.md#94-torrent-travado-é-derrubado-pelo-caminho-de-falha-e-só-quem-está-tentando-conta-como-travado)).
> Nenhum dos três foi medido ao vivo — os testes são unitários, e o passe do terceiro item da lista
> abaixo é o que os exercita de verdade.

- **Rodar a Etapa 0 de fontes múltiplas** — spec desenhada em 16/ago e nunca executada:
  [fontes-multiplas-etapa0](superpowers/specs/2026-08-16-fontes-multiplas-etapa0-design.md).
  Pergunta única: onde o Nyaa está morto, existe infohash **diferente** com peers **vivos**?
  Trocar de indexador não ressuscita swarm — o AnimeTosho é espelho do Nyaa (75/75 `nyaa_id`), e
  só o TokyoTosho agrega tracker de fora. Critério de decisão já fixado antes de medir (≥ 1/3
  confirma; abaixo disso o caminho vira Debrid/Usenet, que é outro *protocolo*, não outro
  indexador). **Pré-requisito que só o usuário tem:** a lista de 4-6 animes que ele viu falhar de
  verdade — sem ela a medição roda na população errada, que foi o erro dos vereditos originais do
  [sources.md](agents/sources.md)
- **Baixar parte de um pack** — para a janela 1-12 de One Piece, o único pack vivo que cobre custa
  171,8 GiB e o teto de 100 GB o barra; os episódios soltos existem e estão mortos. Nenhuma fonte
  resolve isso, porque 574 episódios pesam o que pesam. As três saídas, todas em spec própria:
  teto de tamanho por anime em `AnimeSettings` (menor diff, custa disco), **seleção por arquivo**
  (o `rain` v2.3.1 não tem — `AddTorrentOptions` só expõe `Stopped`/`StopAfterDownload`/
  `StopAfterMetadata`, e `Files()`/`FileStats()` são read-only; `anacrolix/torrent` tem prioridade
  por arquivo) e Debrid. **Escolher depois do item anterior**, que é o que diz se sobra alternativa
- **Passe ao vivo numa franquia em partes (Attack on Titan)** — o eixo absoluto por série, o
  `packAxis` e a posse por cobertura ([#77](agents/decisions.md#77-o-eixo-absoluto-por-série-é-um-bfs-de-duas-em-duas-gerações-e-o-nível-cortado-volta-para-a-fila)/[#78](agents/decisions.md#78-a-unidade-de-posse-de-um-torrent-é-a-cobertura-no-eixo-absoluto-não-a-chave-anime_id-episódio)/[#79](agents/decisions.md#79-a-escolha-de-pack-pergunta-cobre-a-janela-não-é-da-part-n--e-a-numeração-do-pack-é-palpite-entre-três-hipóteses)/[#84](agents/decisions.md#84-a-cobertura-de-um-pack-sem-faixa-no-nome-vem-da-lista-de-arquivos-e-não-da-suposição-de-que-ele-cobre-tudo))
  foram escritos em agosto e só têm teste unitário mais medição de **listagem**. Nenhum download
  real de ponta a ponta rodou na cadeia S1 → Final Season Part 3, que é justo onde as quatro
  convenções de numeração da seção "Granularidade" do [sources.md](agents/sources.md) colidem.
  Instrumento já existe: `scripts/robustness-animes.txt` + `make debug-batch`
