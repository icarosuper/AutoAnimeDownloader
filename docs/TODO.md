- Fontes alternativas de torrent **+0.1.0**
	- ~~Fallback pra AnimeTosho~~ — descartado: é espelho do Nyaa (mesmo infohash/swarm), não ordena
	  por seeders e o `seeders` é cache de ~22 dias. Ver `agents/sources.md`
	- Antes de abstrair `TorrentSource`: responder a Etapa 0 — existe fonte com infohash **diferente**
	  e peers vivos onde o Nyaa está morto? Só o TokyoTosho tem sinal (mas o RSS não expõe seeders)
- Player de vídeo no frontend **+0.1.0**
- Modal com lista para substituir torrent **+0.1.0**
	- Hoje vc tem que trazer um torrent de fora e inserir no campo
	- Quero um botão que abre a listagem do nyaa dentro do app
- Proper release no Windows — **+1.0.0**
	- Autenticar app com conta Microsoft
	- Instalar ao invés de rodar de arquivo executável
	- Dar detach e rodar como serviço no background
	- Fazer mudanças de ui pra funcionar melhor no Windows
	- Testar bem
