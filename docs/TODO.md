- **Correções de posse/busca de pack** — plano ordenado e checklist em `docs/problemas/index.md`
  (arquivo temporário: apagar a pasta quando tudo estiver feito, migrando o que sobrar para cá)
- Scrappar página de detalhes do torrent no nyaa — escopo medido em `docs/agents/sources.md`
  ("Página de detalhe"): só compensa para **pack**, e só quando o nome não dá a faixa ou o pack
  estoura o teto
	- Usar esses detalhes pra filtrar melhor os torrents — a lista de arquivos dá a cobertura
	  real do pack antes de baixar; hoje pack sem faixa no nome vira "cobre tudo" e grava
	  episódio fantasma como baixado
	- Segundo uso: a contagem de arquivos desempata a convenção de numeração do pack — ver
	  `docs/agents/sources.md`, "Granularidade e numeração dos packs"
- `extractPart` entender `Part 1 + Part 2` como cobertura das duas parts — hoje devolve o primeiro
  número e o pack é descartado. Só vale enquanto o filtro de part for por marcador: morre sozinho se
  a busca passar a decidir por cobertura de range
- Reavaliar o frontend buscar direto na AniList — **só** se `id_in` em lote nos avulsos e o gate por
  prioridade não bastarem. Veredito e medições em `docs/agents/decisions.md` #73
- Parear features do webApp na cli
- Melhorar experiencia de baixar animes grandes
- Publicar imagem no docker
- Mecanismo de bug report
- Atualizar README
- Criar landing page
- Proper release no Windows — **+1.0.0**
	- Autenticar app com conta Microsoft
	- Instalar ao invés de rodar de arquivo executável
	- Dar detach e rodar como serviço no background
	- Fazer mudanças de ui pra funcionar melhor no Windows
	- Testar bem
