# AutoAnimeDownloader - Plano de Refatoração

## Visão Geral do Projeto

O **AutoAnimeDownloader** é uma aplicação que automatiza o download de animes baseado na lista de "Watching" do usuário no Anilist. O sistema:

1. Consulta a API GraphQL do Anilist para obter os animes que o usuário está assistindo
2. Faz scraping do Nyaa.si para encontrar torrents dos episódios
3. Adiciona os torrents ao qBittorrent via WebUI API
4. Gerencia episódios baixados, movendo animes completos para pasta separada
5. Remove episódios assistidos quando configurado

## Arquitetura Atual vs. Nova Arquitetura

### Arquitetura Anterior
- GUI Fyne integrada diretamente na lógica de download
- Tudo em um único executável
- Difícil de usar em servidores headless
- Acoplamento entre UI e lógica de negócio

### Nova Arquitetura (3 Componentes Separados)

#### 1. **Daemon** (`src/cmd/daemon/`)
Serviço em background que executa a lógica principal de verificação e download.

**Responsabilidades:**
- Executar o loop de verificação periódica de animes
- Gerenciar downloads através do qBittorrent
- Expor API REST para comunicação com CLI e WebUI
- Manter estado dos episódios baixados
- Gerenciar configurações

**Tecnologias:**
- Go padrão para o serviço
- API REST com `net/http` padrão (stdlib)
- Logging estruturado com `zerolog` (`github.com/rs/zerolog`)
- WebSocket com `nhooyr.io/websocket` ou `github.com/gorilla/websocket`
- Sistema de serviço (systemd no Linux, serviço Windows)

#### 2. **CLI** (`src/cmd/cli/`)
Interface de linha de comando para gerenciar o daemon.

**Responsabilidades:**
- Iniciar/parar/reiniciar o daemon
- Ver status do daemon
- Configurar parâmetros (Anilist username, paths, intervalos, etc.)
- Visualizar logs
- Forçar verificação manual
- Listar episódios baixados

**Tecnologias:**
- Go com `urfave/cli` (`github.com/urfave/cli/v2`)
- Comunicação via API REST com o daemon

#### 3. **WebUI** (`src/frontend/`)
Interface web moderna para gerenciar o daemon.

**Responsabilidades:**
- Dashboard com status do sistema
- Configuração visual das opções
- Lista de animes sendo monitorados
- Lista de episódios baixados
- Histórico de downloads
- Controles de start/stop/restart

**Tecnologias:**
- Frontend: Svelte (framework compilado, sintaxe minimalista)
- Build: Vite (já vem configurado com Svelte)
- UI Library: Tailwind CSS (utility-first CSS framework)
- HTTP Client: Fetch API nativo do browser
- WebSocket: WebSocket API nativo do browser para atualizações em tempo real
- Servido pelo próprio daemon (arquivos estáticos via `net/http`)

## Estrutura de Diretórios Proposta

```
AutoAnimeDownloader/
├── src/
│   ├── cmd/
│   │   ├── daemon/          # Executável do daemon
│   │   │   └── main.go
│   │   └── cli/             # Executável da CLI
│   │       └── main.go
│   ├── daemon/              # Lógica do daemon (já existe, refatorar)
│   │   └── daemon.go
│   ├── internal/
│   │   ├── api/             # API REST do daemon (NOVO)
│   │   │   ├── handlers.go
│   │   │   ├── routes.go
│   │   │   └── middleware.go
│   │   ├── anilist/         # Cliente Anilist (já existe)
│   │   ├── nyaa/            # Scraper Nyaa (já existe)
│   │   ├── torrents/        # Cliente qBittorrent (já existe)
│   │   └── files/           # Gerenciamento de arquivos (já existe)
│   └── frontend/            # WebUI (NOVO)
│       ├── public/
│       ├── src/
│       └── package.json
├── go.mod
└── README.md
```

## Tarefas de Implementação

### Fase 1: Refatoração do Daemon

1. **Criar `src/cmd/daemon/main.go`**
   - Ponto de entrada do daemon
   - Inicializar servidor HTTP para API REST
   - Inicializar loop de verificação
   - Gerenciar graceful shutdown

2. **Criar API REST (`src/internal/api/`)**
   - `GET /api/v1/status` - Status do daemon
   - `GET /api/v1/config` - Obter configurações
   - `PUT /api/v1/config` - Atualizar configurações
   - `POST /api/v1/check` - Forçar verificação manual
   - `GET /api/v1/animes` - Listar animes monitorados
   - `GET /api/v1/episodes` - Listar episódios baixados
   - `POST /api/v1/daemon/start` - Iniciar daemon
   - `POST /api/v1/daemon/stop` - Parar daemon
   - `GET /api/v1/logs` - Obter logs
   - `WS /api/v1/ws` ou `/ws` - WebSocket para atualizações em tempo real

3. **Refatorar `src/daemon/daemon.go`**
   - Remover dependências de UI (funções `ShowError`, `UpdateEpisodesListView`, `SetLoading`)
   - Substituir por logging estruturado com `zerolog`
   - Configurar caller information e stack traces para facilitar debug
   - Tornar a lógica mais modular e testável
   - Adicionar contexto para cancelamento

4. **Gerenciamento de Estado**
   - Criar estrutura para manter estado do daemon
   - Gerenciar mutex para acesso thread-safe
   - Implementar fila de tarefas se necessário

### Fase 2: Implementação da CLI

1. **Criar `src/cmd/cli/main.go`**
   - Definir comandos principais:
     - `start` - Iniciar daemon
     - `stop` - Parar daemon
     - `restart` - Reiniciar daemon
     - `status` - Ver status
     - `config` - Gerenciar configurações
     - `check` - Forçar verificação
     - `list` - Listar animes/episódios
     - `logs` - Ver logs

2. **Cliente HTTP para API**
   - Criar cliente para comunicação com daemon
   - Tratar erros de conexão
   - Formatação de saída (tabelas, JSON, etc.)

3. **Gerenciamento de Processo**
   - Detectar se daemon está rodando
   - Iniciar daemon como processo separado
   - Gerenciar PID file

### Fase 3: Implementação da WebUI

1. **Setup do Frontend**
   - Criar projeto Svelte com Vite: `npm create svelte@latest`
   - Configurar Tailwind CSS
   - Setup de roteamento simples (2 páginas: lista de animes e configurações)

2. **Páginas Principais**
   - Dashboard (status, estatísticas)
   - Configurações (formulário de config)
   - Animes (lista de animes monitorados)
   - Episodes (lista de episódios baixados)
   - Logs (visualização de logs em tempo real)

3. **Integração com API**
   - Fetch API nativo do browser para requisições HTTP
   - WebSocket nativo do browser para atualizações em tempo real
   - Tratamento de erros e loading states

4. **Servir Frontend**
   - Arquivos estáticos servidos pelo próprio daemon via `net/http`
   - Build do Svelte gerado em diretório `dist/` ou similar
   - Daemon serve arquivos estáticos na rota raiz `/`
   - API REST disponível em `/api/v1/*`

### Fase 4: Melhorias e Polimento

1. **Logging**
   - Implementar logging estruturado com `zerolog` (`github.com/rs/zerolog`)
   - Configurar caller information (arquivo:linha) para facilitar debug
   - Usar `.Stack()` para stack traces em erros
   - Níveis de log (DEBUG, INFO, WARN, ERROR)
   - Logs em JSON para fácil processamento e busca
   - Rotação de logs

2. **WebSocket**
   - Implementar servidor WebSocket no daemon usando `nhooyr.io/websocket` ou `github.com/gorilla/websocket`
   - Endpoint WebSocket: `/ws` ou `/api/v1/ws`
   - Usar para atualizações em tempo real:
     - Logs em tempo real
     - Status do daemon
     - Notificações de novos downloads
     - Atualizações de progresso
   - Cliente WebSocket nativo do browser no frontend
   - Reconexão automática em caso de desconexão

3. **Configuração**
   - Validação de configurações
   - Configuração via variáveis de ambiente
   - Migração de configurações antigas

4. **Testes**
   - Testes unitários para lógica de negócio
   - Testes de integração para API
   - Mocks para serviços externos (Anilist, Nyaa, qBittorrent)

5. **Documentação**
   - Documentação da API (OpenAPI/Swagger)
   - Guia de instalação
   - Guia de uso da CLI
   - Guia de uso da WebUI

## Boas Práticas e Convenções

### Código Go

1. **Nomenclatura**
   - Use nomes descritivos e em inglês
   - Interfaces: `-er` suffix (ex: `HTTPClient`, `FileManager`)
   - Pacotes: lowercase, uma palavra quando possível
   - Constantes: `UPPER_SNAKE_CASE` ou `CamelCase` para exported

2. **Estrutura de Pacotes**
   - `internal/` para código privado ao módulo
   - `cmd/` para executáveis
   - Um pacote por diretório
   - Evite `util` ou `common` - seja específico

3. **Error Handling**
   - Sempre retorne erros, nunca ignore
   - Use `fmt.Errorf` com `%w` para wrapping: `fmt.Errorf("failed to load: %w", err)`
   - Crie tipos de erro customizados quando necessário
   - Documente erros retornados

4. **Interfaces**
   - Mantenha interfaces pequenas (1-3 métodos)
   - Defina interfaces onde são usadas, não onde são implementadas
   - Use interfaces para desacoplamento e testabilidade

5. **Context**
   - Use `context.Context` para cancelamento e timeouts
   - Passe context como primeiro parâmetro
   - Respeite context cancellation

6. **Concorrência**
   - Use channels para comunicação entre goroutines
   - Use mutex para proteção de dados compartilhados
   - Documente quando estruturas não são thread-safe
   - Evite race conditions - use `go run -race`

7. **Testing**
   - Testes no mesmo pacote com sufixo `_test.go`
   - Use table-driven tests quando apropriado
   - Mock dependências externas
   - Cobertura de código > 70%

8. **Documentação**
   - Documente exported functions, types, constants
   - Use comentários para explicar "por quê", não "o quê"
   - Exemplos em `Example*` functions

### API REST

1. **Convenções HTTP**
   - `GET` para leitura
   - `POST` para criação/ações
   - `PUT` para atualização completa
   - `PATCH` para atualização parcial
   - `DELETE` para remoção
   - Use códigos HTTP apropriados (200, 201, 400, 404, 500, etc.)

2. **Estrutura de Resposta**
   ```json
   {
     "success": true,
     "data": { ... },
     "error": null
   }
   ```
   Ou para erros:
   ```json
   {
     "success": false,
     "data": null,
     "error": {
       "code": "CONFIG_INVALID",
       "message": "Anilist username is required"
     }
   }
   ```

3. **Versionamento**
   - Use `/api/v1/` prefix
   - Permite evolução futura da API

4. **Validação**
   - Valide todos os inputs
   - Retorne erros descritivos
   - Use struct tags para validação

### Frontend

1. **Estrutura de Componentes**
   - Componentes Svelte (.svelte files)
   - Separe lógica de apresentação
   - Reutilize componentes
   - Use stores do Svelte para estado compartilhado quando necessário

2. **State Management**
   - Svelte stores para estado global (writable, readable, derived)
   - Estado local em componentes quando possível
   - Stores reativas para dados da API

3. **Estilização**
   - Tailwind CSS com classes utility-first
   - Mantenha consistência visual
   - Design responsivo
   - Estilos scoped por componente (padrão do Svelte)

4. **Error Handling**
   - Trate erros de API (fetch e WebSocket)
   - Mostre mensagens amigáveis ao usuário
   - Loading states durante requisições
   - Use `console.log` para debug (sem biblioteca de logging inicialmente)

### Git e Versionamento

1. **Commits**
   - Mensagens descritivas em inglês
   - Formato: `type(scope): description`
   - Types: `feat`, `fix`, `docs`, `refactor`, `test`, `chore`

2. **Branches**
   - `main` - produção
   - `develop` - desenvolvimento
   - `feature/*` - novas features
   - `fix/*` - correções

3. **Pull Requests**
   - Descrição clara do que foi feito
   - Referencie issues
   - Peça review antes de merge

### Configuração e Deploy

1. **Variáveis de Ambiente**
   - Use para configurações sensíveis
   - Documente todas as variáveis
   - Valores padrão quando apropriado

2. **Build**
   - Builds reproduzíveis
   - Versionamento de binários
   - Cross-compilation para múltiplas plataformas

3. **Serviços**
   - Systemd service file para Linux
   - Windows service para Windows
   - Graceful shutdown

## Dependências Recomendadas

### Backend (Go)
- **API Framework**: `net/http` (stdlib) - sem dependências externas
- **Logging**: `zerolog` (`github.com/rs/zerolog`) - logging estruturado com caller e stack traces para debug
- **CLI**: `urfave/cli` (`github.com/urfave/cli/v2`) - biblioteca CLI organizada
- **WebSocket**: `nhooyr.io/websocket` ou `github.com/gorilla/websocket` - servidor WebSocket
- **Config**: Viper (`github.com/spf13/viper`) para configuração flexível (opcional)
- **Testing**: Testify (`github.com/stretchr/testify`)

### Frontend
- **Framework**: Svelte - framework compilado, sintaxe minimalista
- **Build**: Vite - já vem configurado com Svelte
- **UI Library**: Tailwind CSS - utility-first CSS framework
- **HTTP Client**: Fetch API nativo do browser
- **WebSocket**: WebSocket API nativo do browser
- **Logging**: `console.log` nativo (sem biblioteca externa inicialmente)

## Próximos Passos Imediatos

1. ✅ Criar este documento de planejamento
2. ⬜ Criar estrutura de diretórios para API
3. ⬜ Implementar API REST básica no daemon
4. ⬜ Refatorar `daemon.go` para remover dependências de UI
5. ⬜ Criar `cmd/daemon/main.go`
6. ⬜ Implementar CLI básica
7. ⬜ Setup inicial do frontend
8. ⬜ Integrar frontend com API

## Notas Importantes

- Manter compatibilidade com configurações existentes durante migração
- Testar cada componente isoladamente antes de integrar
- Priorizar funcionalidade core antes de features extras
- Manter código limpo e bem documentado
- Fazer commits frequentes e pequenos

