- Teto de pack por episódio em vez de por tamanho total — escopo medido em
  `docs/agents/sources.md` ("Página de detalhe", item 2). **Não economiza download** (o `rain` não
  seleciona arquivo), muda só o critério de aceitar; só vale se o teto atual estiver reprovando
  pack que o usuário quer
- Desempatar a numeração do pack pela contagem de arquivos por pasta — teto conhecido da
  `decisions.md` #84: pack cujos arquivos reiniciam a numeração por season fica com a faixa da
  maior season em vez do total
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
