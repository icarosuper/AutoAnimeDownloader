# AutoAnimeDownloader - Especificações do Projeto

## Visão Geral do Projeto

O **AutoAnimeDownloader** automatiza o download de animes com base na lista de “Watching” do usuário no Anilist. O sistema:

1. Consulta a API GraphQL do Anilist para obter os animes que o usuário está assistindo  
2. Faz scraping do Nyaa.si para encontrar torrents dos episódios  
3. Adiciona os torrents ao qBittorrent via WebUI API  
4. Gerencia episódios baixados, movendo animes completos para uma pasta separada  
5. Remove episódios assistidos quando configurado  

## Arquitetura

### Arquitetura Anterior

- GUI Fyne integrada diretamente na lógica de download  
- Tudo em um único executável  
- Difícil de usar em servidores headless  
- Forte acoplamento entre UI e lógica de negócio  

### Nova Arquitetura (3 Componentes Separados)

#### 1. **Daemon** (`src/cmd/daemon/`)

Serviço em background que executa a lógica principal de verificação e download.

**Responsabilidades:**

- Executar o loop de verificação periódica de animes  
- Gerenciar downloads via qBittorrent  
- Expor API REST para comunicação com CLI e WebUI  
- Manter estado mínimo (status, último check, erro do último check)  
- Gerenciar configurações  

**Tecnologias:**

- Go (stdlib)  
- API REST com `net/http`  
- Logging estruturado com `zerolog`  
- WebSocket com `nhooyr.io/websocket`  
- Serviço systemd (Linux) / serviço Windows  

#### 2. **CLI** (`src/cmd/cli/`)

Interface de linha de comando para gerenciar o daemon.

**Responsabilidades:**

- Iniciar/parar o processo do daemon  
- Iniciar/parar o loop de verificação  
- Ver status do daemon  
- Configurar parâmetros (Anilist username, paths, intervalos, etc.)  
- Visualizar logs  
- Forçar verificação manual  
- Listar animes e episódios baixados  

**Tecnologias:**

- Go com `urfave/cli/v2`  
- Comunicação via API REST com o daemon  
- Saída em tabela e JSON  

#### 3. **WebUI** (`src/internal/frontend/`)

Interface web moderna para gerenciar o daemon.

**Responsabilidades:**

- Dashboard com status do sistema (status, último check, erro)  
- Configuração visual das opções  
- Lista de animes monitorados  
- Lista de episódios baixados  
- Visualização de logs em tempo (quase) real  
- Controles de start/stop/check do daemon  

**Tecnologias:**

- Svelte + Vite  
- Tailwind CSS  
- Fetch API (HTTP)  
- WebSocket API nativo do browser para atualizações de status  
- Servida diretamente pelo daemon (arquivos estáticos via `net/http` + `embed`)  

## Comunicação e Fluxo de Dados

- **Daemon ↔ Anilist**: GraphQL HTTP para obter lista de animes em “Watching”  
- **Daemon ↔ Nyaa**: scraping HTML do Nyaa.si para encontrar torrents compatíveis  
- **Daemon ↔ qBittorrent**: chamadas HTTP para WebUI API do qBittorrent  
- **CLI ↔ Daemon**: HTTP (API REST)  
- **WebUI ↔ Daemon**:  
  - HTTP (API REST) para dados (status, config, animes, episódios)  
  - WebSocket para atualizações de status em tempo real  

## API (Visão Geral)

- Prefixo: `/api/v1/`  
- Exemplos de endpoints principais (já implementados):  
  - `GET /status` – estado atual do daemon  
  - `GET /config` / `PUT /config` – leitura/atualização de configuração  
  - `GET /animes` – animes monitorados com agregação de episódios  
  - `GET /episodes` – episódios baixados  
  - `GET /logs` – visualização de logs do daemon (últimas N linhas, parâmetro opcional `lines`)  
  - `POST /check` – força verificação manual  
  - `POST /daemon/start` / `POST /daemon/stop` – controle do loop do daemon  
- WebSocket:  
  - `GET /api/v1/ws` – envia eventos de atualização de status (`status_update`)  

## Boas Práticas e Convenções (Resumo)

### Código Go

- Usar nomes descritivos e em inglês  
- Pacotes em `internal/` para código interno, `cmd/` para executáveis  
- Não ignorar erros; usar `fmt.Errorf` com `%w` para wrapping  
- Usar `context.Context` para cancelamento/timeouts  
- Proteger dados compartilhados com mutex; usar `go test -race` quando possível  

### API REST

- `GET` para leitura; `POST` para ações; `PUT` para atualização completa  
- Resposta padronizada JSON: `{ "success", "data", "error" }`  
- Versionamento via prefixo `/api/v1/`  
- Inputs sempre validados, com mensagens de erro claras  

### Frontend

- Componentes Svelte pequenos e focados  
- Tailwind CSS para estilização consistente  
- Uso de stores Svelte para estado compartilhado quando necessário  
- Tratar erros de API e WebSocket com mensagens amigáveis ao usuário  

### Git e Fluxo de Trabalho

- Mensagens de commit descritivas (`feat:`, `fix:`, `docs:`, etc.)  
- Branches `feature/*` e `fix/*` para trabalho isolado  
- PRs com descrição clara e testes passando  
