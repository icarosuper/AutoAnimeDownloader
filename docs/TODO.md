# TODO

Agrupado por release: cada etapa é **uma** release, e o bump da etapa é o maior bump das tasks
dentro dela — task que não mexe no binário (doc, coisa fora do repo) pega carona sem mudar o número.
Versão atual: `v2.2.1`.

> A antiga Etapa 1 (fechar tetos do que já roda) fechou em 30/ago/2026 **sem código**: os três
> itens foram medidos e os três disseram para não mexer. Numeração de pack por pasta e teto de
> pack por episódio em [decisions.md #84](agents/decisions.md#84-a-cobertura-de-um-pack-sem-faixa-no-nome-vem-da-lista-de-arquivos-e-não-da-suposição-de-que-ele-cobre-tudo)
> e [sources.md](agents/sources.md); custo do frontend na AniList em
> [decisions.md #85](agents/decisions.md#85-o-custo-do-frontend-na-anilist-é-por-conta-e-por-órfão-não-por-aba),
> que fecha a reavaliação da [#73](agents/decisions.md#73-o-frontend-não-busca-direto-na-anilist-mesmo-podendo).
> O instrumento das duas primeiras ficou em `nyaa/live_pack_measure_test.go` (`AAD_LIVE_NYAA=1`).

## Etapa 1 — `v2.3.0` (`+0.1.0`) — baixar direito anime velho e longo

O sintoma é um só: **anime velho ou longo não baixa**. Anime da temporada corrente baixa sem
problema. A causa não é achar candidato — é que o candidato ou está morto (0-2 seeders, barrado por
`min_seeders` ou travando em 0%) ou é um pack gigante que o teto de tamanho reprova. Ordem não é
preferência: o item 1 é um **gate**, e o resultado dele decide qual das saídas do item 2 existe.

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
  por arquivo) e Debrid. **Escolher depois do item 1**, que é o que diz se sobra alternativa
- **Passe ao vivo numa franquia em partes (Attack on Titan)** — o eixo absoluto por série, o
  `packAxis` e a posse por cobertura ([#77](agents/decisions.md#77-o-eixo-absoluto-por-série-é-um-bfs-de-duas-em-duas-gerações-e-o-nível-cortado-volta-para-a-fila)/[#78](agents/decisions.md#78-a-unidade-de-posse-de-um-torrent-é-a-cobertura-no-eixo-absoluto-não-a-chave-anime_id-episódio)/[#79](agents/decisions.md#79-a-escolha-de-pack-pergunta-cobre-a-janela-não-é-da-part-n--e-a-numeração-do-pack-é-palpite-entre-três-hipóteses)/[#84](agents/decisions.md#84-a-cobertura-de-um-pack-sem-faixa-no-nome-vem-da-lista-de-arquivos-e-não-da-suposição-de-que-ele-cobre-tudo))
  foram escritos em agosto e só têm teste unitário mais medição de **listagem**. Nenhum download
  real de ponta a ponta rodou na cadeia S1 → Final Season Part 3, que é justo onde as quatro
  convenções de numeração da seção "Granularidade" do [sources.md](agents/sources.md) colidem.
  Instrumento já existe: `scripts/robustness-animes.txt` + `make debug-batch`
- **Falha de torrent joga fora os bytes já baixados** — `HandleTorrentFailure` chama
  `backend.Remove(hash, false)` (`daemon/helpers.go:109`) e o passe seguinte re-adiciona o magnet
  ([#24](agents/decisions.md#24-a-failed-torrent-is-dropped-from-the-session-and-re-added-by-the-next-pass--no-blacklist)).
  Em pack de 90 GiB a 90%, um erro transitório custa o download inteiro de novo. Medir se
  `keepData=true` + re-add com o mesmo id + `Verify()` recupera o bitfield
- **A guarda de disco é cega ao tamanho do torrent** — `checkDiskSpace` (`daemon/helpers.go:49`) só
  compara `min_free_disk_percent` no instante do Add. Um pack de 61 GiB entra num volume com 40 GiB
  livres e enche o disco no meio do download. O tamanho já vem do resultado do Nyaa — é o mesmo
  dado dos tetos `max_batch_torrent_size_gb`/`max_episode_torrent_size_gb`
- **Torrent travado não tem dono no backend** — `stallTracker` mora na tela e some no reload; o
  daemon não faz nada com 0 peers por horas, e o torrent segura um slot de
  `max_concurrent_downloads` indefinidamente. Com anime velho isso é o caso comum, não a exceção

## Etapa 2 — `v2.4.0` (`+0.1.0`) — features que faltam

Fecha o escopo funcional **antes** do rebranding, pra UI nova nascer desenhada em cima do conjunto
final de features em vez de ser redesenhada duas vezes.

- Adicionar integração com MyAnimeList
- Mecanismo de bug report — precisa existir bem antes de divulgar, senão o feedback chega sem
  contexto
- Parear features do webApp na cli — por último dentro da etapa, pra parear já com o MyAnimeList
  dentro

## Etapa 3 — `v3.0.0` (`+1.0.0`) — Serval

Vem **antes** de tudo que carimba o nome e a cara do projeto (instalação, README, social preview,
landing page), senão é retrabalho garantido.

- Renomear o projeto para **Serval** — nome do módulo Go, binários (`daemon`/`cli`), repo no GitHub,
  `config.json` e diretório de dados do usuário, imagem do ghcr, títulos do webApp e do Swagger.
  Quebra o caminho de dados de quem já usa → precisa de migração ou nota de release explícita, e é
  o que faz o major da etapa
- Mexida na UI/UX do webApp
- Identidade visual — logo, paleta, favicon; alimenta o social preview e a landing page da Etapa 4

## Etapa 4 — `v3.1.0` (`+0.1.0`) — chegar no usuário de fora

Tudo aqui depende do nome e da identidade da Etapa 3.

- Melhorar a experiência de instalação — opções medidas e ordenadas em
  [distribuicao.md](distribuicao.md): publicar imagem no ghcr (maior ganho, o Dockerfile já existe),
  depois um `install.sh` via curl. `go install`, Homebrew, Scoop, .deb/.rpm e Flatpak avaliados e
  descartados lá. É o que dá o `+0.1.0` da etapa
- Atualizar README — depois da instalação, porque é ela que muda as instruções
- Subir social preview no GitHub — hoje o card de link compartilhado é o genérico do GitHub.
  Settings > General > Social preview > Upload, 1280x640, usar a identidade visual nova. Só existe
  na web, `gh repo edit` não cobre
- Criar landing page

## Etapa 5 — `v4.0.0` (`+1.0.0`) — proper release no Windows

- Autenticar app com conta Microsoft
- Instalar ao invés de rodar de arquivo executável
- Dar detach e rodar como serviço no background
- Fazer mudanças de ui pra funcionar melhor no Windows
- Testar bem

## Etapa 6 — sem release — divulgar

Depois da Etapa 5: divulgar sem Windows corta boa parte do público.
