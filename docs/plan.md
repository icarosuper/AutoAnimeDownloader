# AutoAnimeDownloader - Plano de Implementação

Este documento descreve o plano ordenado de implementação da refatoração do AutoAnimeDownloader, baseado nas especificações em `specifications.md`.

## Visão Geral da Estratégia

A implementação será feita em etapas sequenciais, priorizando a base (daemon) antes de construir as interfaces (CLI e WebUI). Cada etapa deve ser testada antes de prosseguir.

---

## Etapa 1: Logging e Refatoração Base do Daemon

**Objetivo:** Substituir logging básico por zerolog e remover dependências de UI do código existente.

### 1.1 Configurar Zerolog
- [x] Criar `src/internal/logger/logger.go` com configuração do zerolog
- [x] Configurar caller information (arquivo:linha)
- [x] Configurar níveis de log (DEBUG, INFO, WARN, ERROR)
- [x] Configurar output (JSON em produção, console formatado em dev)
- [x] Adicionar função helper para stack traces em erros
- [x] Implementar gravação de logs em arquivo com rotação (lumberjack)
- [x] Multi-writer: escrever simultaneamente no console e arquivo
- [x] Formato JSON no arquivo, console formatado em dev

### 1.2 Refatorar `src/daemon/daemon.go`
- [x] Remover parâmetros de UI: `ShowError`, `UpdateEpisodesListView`, `SetLoading`
- [x] Substituir `fmt.Printf` e `log.*` por chamadas ao zerolog
- [x] Adicionar contexto (`context.Context`) para cancelamento
- [x] Adicionar campos estruturados aos logs (anime, episode, etc.)
- [x] Usar `.Stack()` em logs de erro
- [x] Garantir que todos os erros sejam registrados no state (`SetLastCheckError()`)
- [x] Testar que a lógica de download ainda funciona

### 1.3 Criar Estrutura de Estado do Daemon
- [x] Criar `src/daemon/state.go` com estrutura de estado mínima
- [x] Definir interface `StateNotifier` para notificações de mudanças
- [x] Definir struct `State` com:
  - Status (running, stopped, checking)
  - Timestamp da última verificação
  - Erro da última verificação (se houver)
  - Mutex para thread-safety
  - Campo opcional `notifier StateNotifier` (injeção de dependência)
- [x] Criar funções para atualizar estado de forma thread-safe
- [x] Implementar `SetNotifier()` para injetar notificador
- [x] Implementar `notifyChange()` que chama o notificador quando estado muda
- [x] Chamar `notifyChange()` automaticamente em `SetStatus()`, `SetLastCheck()`, `SetLastCheckError()`
- [x] Estado será usado para:
  - Endpoint `GET /api/v1/status` (retorna estado completo)
  - WebSocket (notifica mudanças de estado automaticamente via notifier)
- [x] Não incluir estatísticas ou operação atual (podem ser obtidas dos dados ou logs)

### 1.4 Testes da Etapa 1
- [x] Criar testes para logger (`src/internal/logger/logger_test.go`):
  - [x] Testar inicialização em modo desenvolvimento e produção
  - [x] Testar diferentes níveis de log
  - [x] Testar formatação de output (console vs JSON)
  - [x] Testar caller information
  - [x] Testar funções helper (Debug, Info, Warn, Error)
  - [x] Testar logging estruturado com campos
  - [x] Testar filtragem por nível
- [x] Criar testes para state (`src/daemon/state_test.go`):
  - [x] Testar thread-safety (múltiplas goroutines acessando simultaneamente)
  - [x] Testar notificações quando estado muda
  - [x] Testar métodos Get/Set de todos os campos
  - [x] Testar `GetAll()` retorna snapshot consistente
  - [x] Testar que notificações não são chamadas quando não há mudança real
  - [x] Testar comportamento sem notifier
  - [x] Testar conteúdo das notificações
- [x] Criar testes para refatoração do daemon (`src/daemon/daemon_test.go`):
  - [x] Testar que erros são registrados corretamente no state (config load error)
  - [x] Testar que erros são registrados corretamente no state (episodes load error)
  - [x] Testar que contexto de cancelamento funciona
  - [x] Testar que logs são gerados corretamente
  - [x] Testar que erros são tratados corretamente
  - [x] Testar transições de status no StartLoop (`TestStartLoop_StatusTransitions`)
  - [x] Testar que status muda para "checking" durante verificação (`TestStartLoop_StatusCheckingDuringVerification`)

---

## Etapa 2: API REST do Daemon

**Objetivo:** Criar API REST completa para comunicação com CLI e WebUI.

**Status:** ✅ **CONCLUÍDA**

### 2.1 Estrutura Base da API
- [x] Criar `src/internal/api/server.go` com servidor HTTP básico
- [x] Configurar roteamento com `net/http`
- [x] Criar middleware para:
  - Logging de requisições (`middleware.go`)
  - CORS (`corsMiddleware`)
  - Content-Type JSON (`jsonMiddleware`)
- [x] Implementar graceful shutdown
- [x] Estrutura vertical slice: cada endpoint em arquivo separado com prefixo `endpoint_`
- [x] Todos os arquivos no mesmo pacote `api` (sem subpastas)

### 2.2 Handlers Básicos
- [x] Criar handlers em arquivos separados (`endpoint_*.go`)
- [x] Implementar handler de status: `GET /api/v1/status` (`endpoint_status.go`)
  - Retornar estado completo do daemon:
    - Status atual (stopped/running/checking)
    - Timestamp da última verificação
    - Se houve erro na última checagem (boolean)
  - Formato JSON padronizado
  - Usado pela WebUI para exibir informações do daemon
- [x] Implementar handler de configuração: `GET /api/v1/config` (`endpoint_config.go`)
  - Retornar configurações atuais
- [x] Implementar handler de atualização: `PUT /api/v1/config` (`endpoint_config.go`)
  - Validar entrada (campos obrigatórios, valores numéricos)
  - Salvar configurações
  - Retornar erro se inválido

### 2.3 Handlers de Dados
- [x] Implementar `GET /api/v1/animes` (`endpoint_animes.go`)
  - Listar animes monitorados com agregação de episódios
  - Incluir informações de progresso (episódios count, latest episode ID)
  - Extração automática de nome do anime a partir do nome do episódio
- [x] Implementar `GET /api/v1/episodes` (`endpoint_episodes.go`)
  - Listar episódios baixados
  - Incluir hash e nome do episódio

### 2.4 Handlers de Controle
- [x] Implementar `POST /api/v1/check` (`endpoint_check.go`)
  - Forçar verificação manual
  - Executar em goroutine separada
  - Retornar imediatamente (async)
  - Retornar apenas confirmação
- [x] Implementar `POST /api/v1/daemon/start` (`endpoint_daemon_start.go`)
  - Iniciar loop de verificação
  - Estado é atualizado internamente pelo daemon
  - Retornar apenas confirmação
  - Verificar se já está rodando antes de iniciar
- [x] Implementar `POST /api/v1/daemon/stop` (`endpoint_daemon_stop.go`)
  - Parar loop de verificação
  - Estado é atualizado internamente pelo daemon (com atualização imediata no `StopDaemonLoop()`)
  - Retornar apenas confirmação
  - Verificar se já está parado antes de parar

### 2.5 Estrutura de Resposta Padrão
- [x] Criar tipos de resposta padronizados (`responses.go`):
  - `SuccessResponse` com `success`, `data`, `error`
  - `ErrorInfo` com código e mensagem
- [x] Criar helpers para serializar respostas:
  - `JSONSuccess()` - resposta de sucesso
  - `JSONError()` - resposta de erro
  - `JSONInternalError()` - erro interno (500)
- [x] Implementar tratamento de erros consistente

### 2.6 Documentação Swagger/OpenAPI
- [x] Instalar e configurar Swaggo/swag
- [x] Adicionar comentários Swagger em todos os endpoints
- [x] Gerar documentação OpenAPI (`docs/swagger.json`, `docs/swagger.yaml`)
- [x] Adicionar rota `/swagger/` para servir UI do Swagger
- [x] Documentar todos os endpoints com descrições, parâmetros e respostas

### 2.7 Gerenciamento de Estado do Daemon
- [x] Daemon gerencia seu próprio status internamente
- [x] Status é atualizado automaticamente quando:
  - Loop inicia: `StatusRunning`
  - Verificação começa: `StatusChecking` (definido diretamente antes de `AnimeVerification`)
  - Verificação termina: `StatusRunning` (ou `StatusStopped` se contexto foi cancelado)
  - Loop para: `StatusStopped`
- [x] `StopDaemonLoop()` atualiza status imediatamente para resposta rápida
- [x] API apenas lê o status, não modifica diretamente
- [x] Corrigido: Status "checking" agora é visível durante toda a execução de `AnimeVerification` (removida função anônima com defer que causava problemas de timing)

### 2.8 Testes da API
- [x] Criar testes unitários para handlers (`src/internal/api/*_test.go`):
  - [x] Testar cada handler isoladamente (status, config, check, daemon start/stop, episodes, animes)
  - [x] Testar validação de entrada (campos obrigatórios, tipos, formatos)
  - [x] Testar tratamento de erros (retornar códigos HTTP corretos)
  - [x] Testar serialização de respostas JSON
  - [x] Testar casos de sucesso e falha
- [x] Criar testes de integração para rotas principais (`src/tests/integration/integration_test.go`):
  - [x] Testar fluxo completo de requisições HTTP
  - [x] Testar todos os endpoints da API (status, config, animes, episodes, check, daemon start/stop)
  - [x] Testar lifecycle do daemon (start/stop)
  - [x] Testar fluxo completo de download (com mocks)
  - [x] Script de execução de testes de integração (`scripts/run-integration-tests.sh`)
  - [ ] Testar middlewares (logging, CORS, Content-Type) - pode ser adicionado se necessário
  - [ ] Testar graceful shutdown - pode ser adicionado se necessário
  - [ ] **Testes de integração CLI → API → Daemon:**
    - [ ] Testar todos os comandos da CLI end-to-end
    - [ ] Testar gerenciamento de processo (start/stop via CLI)
    - [ ] Validar comunicação completa entre componentes
- [x] Testar handlers específicos:
  - [x] `GET /api/v1/status`: retorna estado correto do daemon
  - [x] `GET /api/v1/config`: retorna configurações
  - [x] `PUT /api/v1/config`: valida e salva configurações
  - [x] `POST /api/v1/check`: executa verificação assíncrona
  - [x] `POST /api/v1/daemon/start` e `stop`: controlam daemon corretamente

---

## Etapa 3: WebSocket para Tempo Real

**Objetivo:** Implementar WebSocket para atualizações em tempo real.

**Status:** ✅ **CONCLUÍDA**

### 3.1 Servidor WebSocket
- [x] Adicionar dependência WebSocket ao `go.mod`
- [x] Criar `src/internal/api/websocket.go`
- [x] Implementar handler WebSocket: `/api/v1/ws`
- [x] Gerenciar conexões WebSocket (map de clientes)
- [x] Implementar broadcast para todos os clientes conectados

### 3.2 Integração com Daemon
- [x] WebSocket manager implementa interface `StateNotifier`
- [x] Método `NotifyStateChange()` faz broadcast para todos os clientes conectados
- [x] No ponto de inicialização do daemon, injetar WebSocket manager no state:
  ```go
  state.SetNotifier(wsManager)
  ```
- [x] State notifica automaticamente quando muda (sem chamadas manuais necessárias)
- [x] Enviar eventos via WebSocket quando estado muda:
  - Mudanças de status (stopped → running → checking)
  - Após cada verificação: enviar timestamp da última verificação e se houve erro
- [x] Formato de mensagens JSON padronizado:
  ```json
  {
    "type": "status_update",
    "data": {
      "status": "checking",
      "last_check": "2024-01-15T10:30:45Z",
      "has_error": false
    }
  }
  ```

### 3.3 Reconexão e Robustez
- [x] Implementar ping/pong para manter conexão viva
- [x] Tratar desconexões gracefully
- [x] Limpar conexões mortas
- [x] Adicionar timeout para conexões inativas

### 3.4 Testes do WebSocket
- [ ] Criar testes para WebSocket manager (`src/internal/api/websocket_test.go`):
  - Testar conexão de clientes
  - Testar desconexão de clientes
  - Testar broadcast de mensagens para todos os clientes
  - Testar que `NotifyStateChange()` envia mensagens corretas
  - Testar formato JSON das mensagens
- [ ] Testes de integração:
  - Testar conexão WebSocket end-to-end
  - Testar que mudanças de estado são propagadas via WebSocket
  - Testar ping/pong
  - Testar reconexão automática
  - Testar múltiplos clientes conectados simultaneamente

---

## Etapa 4: Ponto de Entrada do Daemon

**Objetivo:** Criar `main.go` do daemon que integra tudo.

### 4.1 Criar `src/cmd/daemon/main.go`
- [x] Inicializar logger zerolog
- [x] Inicializar FileManager
- [x] Criar instância do daemon state
- [x] Inicializar servidor HTTP com API REST
- [x] Configurar graceful shutdown (SIGINT, SIGTERM)
- [x] Iniciar loop de verificação automaticamente ao iniciar
- [x] Gerenciamento de status: daemon gerencia seu próprio status internamente
- [x] Estado inicial: `StatusStopped` (será atualizado para `StatusRunning` quando loop iniciar)
- [x] Inicializar servidor WebSocket
- [x] **Injetar WebSocket manager no state:** `state.SetNotifier(wsManager)`

### 4.2 Servir Arquivos Estáticos (Frontend)
- [x] Adicionar handler para servir arquivos estáticos em `/`
- [x] Servir `index.html` para rotas não-API
- [x] Configurar `dist/` ou similar como diretório de arquivos estáticos
- [x] Testar que frontend será servido corretamente

### 4.3 Testes de Integração
- [x] Testar inicialização completa do daemon
- [x] Testar graceful shutdown
- [x] Testar que API responde corretamente
- [x] Testar que WebSocket funciona

---

## Etapa 5: CLI Básica

**Objetivo:** Criar CLI para gerenciar o daemon.

**Status:** ✅ **CONCLUÍDA**

### 5.1 Estrutura Base da CLI
- [x] Criar `src/cmd/cli/main.go`
- [x] Configurar `urfave/cli` com app básico
- [x] Definir flags globais (endpoint do daemon, formato de saída, verbose)

### 5.2 Cliente HTTP
- [x] Criar `src/internal/api/client.go`
- [x] Implementar cliente HTTP para comunicação com daemon
- [x] Tratar erros de conexão
- [x] Implementar timeout
- [x] Criar funções helper para cada endpoint

### 5.3 Comandos Básicos
- [x] Implementar `start` - iniciar processo do daemon
- [x] Implementar `stop` - parar processo do daemon
- [x] Implementar `loop start` - iniciar loop de verificação
- [x] Implementar `loop stop` - parar loop de verificação
- [x] Implementar `status` - mostrar status do daemon
- [x] Implementar `config get` - mostrar configurações
- [x] Implementar `config set <key> <value>` - atualizar configuração (com lista de keys no help)
- [x] Implementar `check` - forçar verificação manual
- [x] Implementar `animes` - listar animes monitorados
- [x] Implementar `episodes` - listar episódios baixados
- [x] Implementar `logs` - mostrar logs recentes

### 5.4 Gerenciamento de Processo
- [x] Criar `src/internal/cli/process.go`
- [x] Implementar detecção se daemon está rodando (verificar PID file)
- [x] Implementar `start` - iniciar daemon como processo separado em background
- [x] Implementar `stop` - parar daemon (enviar SIGTERM)
- [x] Gerenciar PID file (`~/.autoAnimeDownloader/daemon.pid`)

### 5.5 Formatação de Saída
- [x] Implementar formatação em tabela (padrão) usando `go-pretty`
- [x] Implementar formatação JSON (`--json` flag)
- [x] Suporte a cores para terminal

### 5.6 Melhorias na CLI
- [x] Help do comando `config set` lista todas as keys disponíveis com tipos
- [x] Mensagem de erro quando `config set` é chamado sem argumentos também lista keys disponíveis

### 5.7 Testes da CLI
- [x] **Nota sobre testes:** A CLI é principalmente um wrapper que envia comandos HTTP e executa processos do sistema. Testes unitários não são prioritários aqui, pois a CLI apenas:
  - Faz parsing de argumentos (já testado indiretamente pelo uso)
  - Chama funções do cliente HTTP (que podem ser testadas isoladamente se necessário)
  - Executa comandos do sistema (difícil de testar unitariamente)
- [ ] Testes de integração CLI → API → Daemon (pendente):
  - [ ] Testar todos os comandos da CLI end-to-end
  - [ ] Testar gerenciamento de processo (start/stop via CLI)
  - [ ] Validar comunicação completa entre componentes

---

## Etapa 6: Setup Inicial do Frontend

**Objetivo:** Configurar projeto Svelte e estrutura básica.

**Status:** ✅ **CONCLUÍDA**

### 6.1 Criar Projeto Svelte
- [x] Executar `npm create svelte@latest` em `src/internal/frontend/`
- [x] Escolher template básico (SPA)
- [x] Configurar Vite

### 6.2 Configurar Tailwind CSS
- [x] Instalar Tailwind CSS: `npm install -D tailwindcss postcss autoprefixer`
- [x] Inicializar Tailwind: `npx tailwindcss init -p`
- [x] Configurar `tailwind.config.js` com paths do Svelte
- [x] Adicionar diretivas do Tailwind ao CSS global
- [x] Testar que Tailwind funciona

### 6.3 Estrutura de Rotas
- [x] Criar estrutura de rotas (3 páginas):
  - `/` ou `/status` - Dashboard/Status
  - `/episodes` - Lista de episódios
  - `/config` - Configurações
- [x] Configurar roteamento (svelte-spa-router)
- [x] Criar layout base com navegação

### 6.4 Cliente API
- [x] Criar `src/lib/api/client.ts`
- [x] Implementar funções para chamadas HTTP com fetch
- [x] Implementar cliente WebSocket (`src/lib/websocket/client.ts`)
- [x] Tratamento de erros básico
- [x] Helpers para formatação de dados

---

## Etapa 7: Páginas do Frontend

**Objetivo:** Implementar as páginas principais da WebUI.

**Status:** ✅ **CONCLUÍDA**

### 7.1 Página de Status/Dashboard
- [x] Criar componente `Status.svelte`
- [x] Buscar status do daemon da API
- [x] Exibir informações:
  - Status do daemon (running/stopped/checking)
  - Última verificação
  - Erro da última verificação (se houver)
  - Lista de animes monitorados (top 10)
- [x] Adicionar loading state
- [x] Adicionar tratamento de erros
- [x] Botões de controle: Start, Stop, Check
- [x] Conectar via WebSocket para atualizações em tempo real
- [x] Estilizar com Tailwind

### 7.2 Página de Episódios
- [x] Criar componente `Episodes.svelte`
- [x] Buscar lista de episódios da API
- [x] Exibir lista com:
  - Nome do episódio
  - ID do episódio
  - Hash do episódio
  - Data de download
- [x] Adicionar loading state
- [x] Adicionar tratamento de erros
- [x] Estilizar com Tailwind

### 7.3 Página de Configurações
- [x] Criar componente `Config.svelte`
- [x] Buscar configurações atuais da API
- [x] Criar formulário para editar:
  - Anilist username
  - Save path
  - Completed anime path
  - Check interval
  - qBittorrent URL
  - Max episodes per anime
  - Episode retry limit
  - Delete watched episodes
  - Excluded list
- [x] Implementar validação de formulário
- [x] Salvar via API PUT
- [x] Mostrar feedback de sucesso/erro
- [x] Estilizar com Tailwind

### 7.4 Componentes Reutilizáveis
- [x] Criar componente `Layout.svelte` com navegação
- [x] Criar componente `StatusBadge.svelte` para exibir status
- [x] Criar componente `Loading.svelte` para estados de carregamento
- [x] Criar componente `ErrorMessage.svelte` para exibir erros
- [x] Criar componente `Input.svelte` para campos de formulário

---

## Etapa 8: Integração Frontend-Backend

**Objetivo:** Integrar frontend com backend e testar fluxo completo.

**Status:** ✅ **CONCLUÍDA**

### 8.1 Build do Frontend
- [x] Configurar build do Svelte para gerar arquivos estáticos
- [x] Configurar output para `dist/` ou diretório servido pelo daemon
- [x] Testar build local

### 8.2 Integração com Daemon
- [x] Configurar daemon para servir arquivos do frontend
- [x] Testar que frontend carrega corretamente
- [x] Testar que API responde corretamente do frontend
- [x] Configurar CORS para permitir requisições do frontend
- [x] Implementar fallback para servir `index.html` em rotas SPA

### 8.3 WebSocket no Frontend
- [x] Conectar WebSocket na página Status/Dashboard
- [x] Receber atualizações de status em tempo real
- [x] Implementar reconexão automática
- [x] Testar desconexão e reconexão

### 8.4 Testes End-to-End
- [x] Testar fluxo completo:
  1. Iniciar daemon
  2. Acessar WebUI
  3. Ver status e lista de animes
  4. Editar configurações
  5. Forçar verificação
  6. Ver episódios baixados
- [ ] Testar em diferentes navegadores
- [ ] Testar responsividade

---

## Etapa 9: Testes Finais

**Objetivo:** Garantir qualidade e confiabilidade através de testes abrangentes usando containers.

### 9.1 Containerizar o Projeto Completo
- [x] Criar `Dockerfile` para o daemon (multi-stage build com frontend embedado)
- [x] Criar `docker-compose.test.yml` para orquestração de testes:
  - Serviço do daemon
  - Serviços mockados para testes (Anilist, Nyaa, qBittorrent)
- [x] Configurar variáveis de ambiente nos containers
- [x] Criar `.dockerignore` para otimizar builds
- [ ] Documentar como executar o projeto via Docker (pode ser adicionado se necessário)

### 9.2 Testes de Integração com Containers
- [x] Criar mocks para serviços externos:
  - [x] Mock do Anilist API (`src/tests/mocks/anilist/`)
  - [x] Mock do Nyaa (`src/tests/mocks/nyaa/`)
  - [x] Mock do qBittorrent API (`src/tests/mocks/qbittorrent/`)
- [x] Criar testes de integração usando containers:
  - [x] Testar todos os endpoints da API (`src/tests/integration/integration_test.go`)
  - [x] Testar lifecycle do daemon (start/stop)
  - [x] Testar fluxo completo de download (com mocks)
  - [ ] Testar fluxo completo: CLI → API → Daemon (pendente)
  - [ ] Testar WebSocket end-to-end (pendente)
- [x] Configurar ambiente de testes isolado (`docker-compose.test.yml`)
- [x] Criar scripts para executar testes de integração (`scripts/run-integration-tests.sh`)
- [x] Integrar testes de integração no CI/CD (`.github/workflows/build.yml`)

### 9.3 Implementar Testes Unitários Faltantes
- [x] Testes unitários para handlers da API (`src/internal/api/*_test.go`):
  - [x] Testar cada handler isoladamente (status, config, check, daemon start/stop, episodes, animes)
  - [x] Testar validação de entrada
  - [x] Testar tratamento de erros
  - [x] Testar serialização de respostas JSON
- [ ] Testes unitários para WebSocket (`src/internal/api/websocket_test.go`):
  - [ ] Testar conexão de clientes
  - [ ] Testar desconexão de clientes
  - [ ] Testar broadcast de mensagens
  - [ ] Testar formato JSON das mensagens
- [ ] Testes unitários para cliente HTTP da CLI (`src/internal/api/client_test.go`) - opcional
- [ ] Testes unitários para gerenciamento de processo (`src/internal/cli/process_test.go`) - opcional
- [ ] Aumentar cobertura de testes para > 80% (verificar cobertura atual)
- [ ] Executar testes com race detector (`go test -race`) - verificar se já está sendo feito no CI

---

## Etapa 10: Deploy

**Objetivo:** Preparar o projeto para distribuição e deploy automatizado.

### 10.1 Configurar Build Multiplataforma
- [x] Criar scripts de build para diferentes plataformas:
  - [x] Linux x64 (amd64) - `scripts/build.sh`
  - [x] Linux ARM64 (arm64) - `scripts/build.sh`
  - [x] Windows x64 (amd64) - `scripts/build.ps1`
- [x] Configurar build do frontend para produção:
  - [x] Otimizar assets (minificação, compressão) - via Vite
  - [x] Configurar variáveis de ambiente para produção
- [x] Criar arquivos de serviço:
  - [x] `autoanimedownloader.service` (systemd para Linux) - `infra/linux/autoanimedownloader.service`
  - [x] `autoanimedownloader.xml` (NSSM para Windows Service) - `infra/windows/autoanimedownloader.xml`
  - [x] Scripts de instalação/desinstalação para Linux - `infra/linux/Makefile`
  - [x] Scripts de instalação/desinstalação para Windows - `infra/windows/install.ps1`, `uninstall.ps1`
- [x] Criar scripts auxiliares:
  - [x] `build.sh` / `build.ps1` para build local
  - [x] `package.sh` / `package.ps1` para criar pacotes de distribuição
- [ ] Testar builds em cada plataforma alvo (testado via CI/CD)
- [x] Documentar processo de build e instalação (`docs/build.md`, `docs/installation.md`)

### 10.2 Pipeline de CI/CD no GitHub
- [x] Criar `.github/workflows/build.yml`:
  - [x] Build para Linux x64, Linux ARM64 e Windows x64
  - [x] Build do frontend
  - [x] Executar testes unitários
  - [x] Executar testes de integração (usando containers)
  - [x] Criar artifacts (binários e frontend)
  - [x] Gerar checksums (SHA256) para verificação
- [x] Criar `.github/workflows/release.yml`:
  - [x] Trigger em tags de release
  - [x] Build para todas as plataformas
  - [x] Criar release no GitHub com binários anexados
  - [x] Gerar checksums (SHA256) para verificação
- [x] Configurar secrets necessários (se houver) - não necessário (usa GITHUB_TOKEN)
- [x] Testar pipeline completo - em uso
- [ ] Documentar processo de release (pode ser adicionado se necessário)

---

## Etapa 11: Fim

**Objetivo:** Finalizar o projeto e limpar arquivos temporários.

### 11.1 Excluir Arquivo plan.md
- [ ] Verificar que todas as etapas foram concluídas
- [ ] Remover `docs/plan.md` do repositório
- [ ] Atualizar `.gitignore` se necessário

### 11.2 Atualizar Documentação em /docs
- [ ] Revisar e atualizar `specifications.md` se necessário
- [x] Criar/atualizar guia de instalação (`docs/installation.md`)
- [ ] Criar/atualizar guia de uso da CLI (`docs/cli-guide.md`)
- [ ] Criar/atualizar guia de uso da WebUI (`docs/webui-guide.md`)
- [ ] Criar/atualizar guia de desenvolvimento (`docs/development.md`)
- [ ] Criar/atualizar guia de contribuição (`docs/contributing.md`)
- [x] Garantir que toda documentação está atualizada e consistente (parcialmente)

### 11.3 Atualizar README.md
- [x] Adicionar badges (build status, version, etc.)
- [x] Atualizar descrição do projeto
- [ ] Adicionar screenshots da WebUI
- [x] Documentar instalação rápida
- [x] Documentar uso básico (CLI e WebUI)
- [x] Adicionar links para documentação completa
- [x] Adicionar seção de contribuição (link existe, mas arquivo `contributing.md` falta)
- [x] Adicionar licença e créditos (créditos existem, licença falta)

---

## Checklist de Validação Final

Antes de considerar a refatoração completa, verificar:

- [x] Daemon inicia e para corretamente
- [x] API REST responde a todos os endpoints
- [x] WebSocket funciona e reconecta automaticamente
- [x] CLI funciona para todos os comandos
- [x] WebUI carrega e todas as páginas funcionam
- [x] Logs são estruturados e úteis para debug
- [x] Configurações são salvas e carregadas corretamente
- [ ] Downloads de animes ainda funcionam (testar end-to-end)
- [x] Testes passam (testes unitários implementados)
- [x] Testes de integração completos (implementados em `src/tests/integration/`, integrados no CI/CD)
- [x] Documentação está atualizada (parcialmente - faltam alguns guias)
- [x] Código segue boas práticas definidas

