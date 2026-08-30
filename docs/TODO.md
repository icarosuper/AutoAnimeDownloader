- Scrappar página de detalhes do torrent no nyaa — escopo medido em `docs/agents/sources.md`
  ("Página de detalhe"): só compensa para **pack**, e só quando o nome não dá a faixa ou o pack
  estoura o teto
	- Usar esses detalhes pra filtrar melhor os torrents — a lista de arquivos dá a cobertura
	  real do pack antes de baixar; hoje pack sem faixa no nome vira "cobre tudo" e grava
	  episódio fantasma como baixado
- Reavaliar o frontend buscar direto na AniList — `id_in` em lote nos avulsos e o gate por
  prioridade **já entraram**; falta medir se bastaram. Veredito e medições em
  `docs/agents/decisions.md` #73
- Parear features do webApp na cli
- Adicionar integração com MyAnimeList
- Melhorar experiencia de baixar animes grandes (provavelmente tirar campo de progresso do frontend)
- **Melhorar a experiência de instalação** — opções medidas e ordenadas em
  `docs/distribuicao.md`: publicar imagem no ghcr (maior ganho, o Dockerfile já existe),
  depois um `install.sh` via curl. `go install`, Homebrew, Scoop, .deb/.rpm e Flatpak
  avaliados e descartados lá
- Mecanismo de bug report
- Atualizar README
- Proper release no Windows — **+1.0.0**
	- Autenticar app com conta Microsoft
	- Instalar ao invés de rodar de arquivo executável
	- Dar detach e rodar como serviço no background
	- Fazer mudanças de ui pra funcionar melhor no Windows
	- Testar bem
- Criar landing page
- Divulgar
