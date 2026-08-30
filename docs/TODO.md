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

## Etapa 1 — `v2.3.0` (`+0.1.0`) — features que faltam

Fecha o escopo funcional **antes** do rebranding, pra UI nova nascer desenhada em cima do conjunto
final de features em vez de ser redesenhada duas vezes.

- Adicionar integração com MyAnimeList
- Mecanismo de bug report — precisa existir bem antes de divulgar, senão o feedback chega sem
  contexto
- Parear features do webApp na cli — por último dentro da etapa, pra parear já com o MyAnimeList
  dentro

## Etapa 2 — `v3.0.0` (`+1.0.0`) — Serval

Vem **antes** de tudo que carimba o nome e a cara do projeto (instalação, README, social preview,
landing page), senão é retrabalho garantido.

- Renomear o projeto para **Serval** — nome do módulo Go, binários (`daemon`/`cli`), repo no GitHub,
  `config.json` e diretório de dados do usuário, imagem do ghcr, títulos do webApp e do Swagger.
  Quebra o caminho de dados de quem já usa → precisa de migração ou nota de release explícita, e é
  o que faz o major da etapa
- Mexida na UI/UX do webApp
- Melhorar experiência de baixar animes grandes (provavelmente tirar campo de progresso do
  frontend) — mesma superfície do frontend, entra junto com a mexida de UI/UX
- Identidade visual — logo, paleta, favicon; alimenta o social preview e a landing page da Etapa 3

## Etapa 3 — `v3.1.0` (`+0.1.0`) — chegar no usuário de fora

Tudo aqui depende do nome e da identidade da Etapa 2.

- Melhorar a experiência de instalação — opções medidas e ordenadas em
  [distribuicao.md](distribuicao.md): publicar imagem no ghcr (maior ganho, o Dockerfile já existe),
  depois um `install.sh` via curl. `go install`, Homebrew, Scoop, .deb/.rpm e Flatpak avaliados e
  descartados lá. É o que dá o `+0.1.0` da etapa
- Atualizar README — depois da instalação, porque é ela que muda as instruções
- Subir social preview no GitHub — hoje o card de link compartilhado é o genérico do GitHub.
  Settings > General > Social preview > Upload, 1280x640, usar a identidade visual nova. Só existe
  na web, `gh repo edit` não cobre
- Criar landing page

## Etapa 4 — `v4.0.0` (`+1.0.0`) — proper release no Windows

- Autenticar app com conta Microsoft
- Instalar ao invés de rodar de arquivo executável
- Dar detach e rodar como serviço no background
- Fazer mudanças de ui pra funcionar melhor no Windows
- Testar bem

## Etapa 5 — sem release — divulgar

Depois da Etapa 4: divulgar sem Windows corta boa parte do público.
