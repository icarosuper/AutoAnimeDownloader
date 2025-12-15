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
- [ ] Criar testes unitários para handlers (`src/internal/api/*_test.go`):
  - Testar cada handler isoladamente
  - Testar validação de entrada (campos obrigatórios, tipos, formatos)
  - Testar tratamento de erros (retornar códigos HTTP corretos)
  - Testar serialização de respostas JSON
  - Testar casos de sucesso e falha
- [ ] Criar testes de integração para rotas principais (`src/internal/api/integration_test.go`):
  - Testar fluxo completo de requisições HTTP
  - Testar middlewares (logging, CORS, Content-Type)
  - Testar roteamento correto de todas as rotas
  - Testar graceful shutdown
  - **Incluir testes de integração CLI → API → Daemon:**
    - Testar todos os comandos da CLI end-to-end
    - Testar gerenciamento de processo (start/stop)
    - Validar comunicação completa entre componentes
- [ ] Testar handlers específicos:
  - `GET /api/v1/status`: retorna estado correto do daemon
  - `GET /api/v1/config`: retorna configurações
  - `PUT /api/v1/config`: valida e salva configurações
  - `POST /api/v1/check`: executa verificação assíncrona
  - `POST /api/v1/daemon/start` e `stop`: controlam daemon corretamente

---

## Etapa 3: WebSocket para Tempo Real

**Objetivo:** Implementar WebSocket para atualizações em tempo real.

### 3.1 Servidor WebSocket
- [ ] Adicionar dependência WebSocket ao `go.mod`
- [ ] Criar `src/internal/api/websocket.go`
- [ ] Implementar handler WebSocket: `/api/v1/ws` ou `/ws`
- [ ] Gerenciar conexões WebSocket (map de clientes)
- [ ] Implementar broadcast para todos os clientes conectados

### 3.2 Integração com Daemon
- [ ] WebSocket manager implementa interface `StateNotifier`
- [ ] Método `NotifyStateChange()` faz broadcast para todos os clientes conectados
- [ ] No ponto de inicialização do daemon, injetar WebSocket manager no state:
  ```go
  state.SetNotifier(wsManager)
  ```
- [ ] State notifica automaticamente quando muda (sem chamadas manuais necessárias)
- [ ] Enviar eventos via WebSocket quando estado muda:
  - Mudanças de status (stopped → running → checking)
  - Após cada verificação: enviar timestamp da última verificação e se houve erro
- [ ] Formato de mensagens JSON padronizado:
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
- [ ] Implementar ping/pong para manter conexão viva
- [ ] Tratar desconexões gracefully
- [ ] Limpar conexões mortas
- [ ] Adicionar timeout para conexões inativas

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

### 4.1 Criar `src/main.go`
- [x] Inicializar logger zerolog
- [x] Inicializar FileManager
- [x] Criar instância do daemon state
- [x] Inicializar servidor HTTP com API REST
- [x] Configurar graceful shutdown (SIGINT, SIGTERM)
- [x] Iniciar loop de verificação automaticamente ao iniciar
- [x] Gerenciamento de status: daemon gerencia seu próprio status internamente
- [x] Estado inicial: `StatusStopped` (será atualizado para `StatusRunning` quando loop iniciar)
- [ ] Inicializar servidor WebSocket (FUTURO)
- [ ] **Injetar WebSocket manager no state:** `state.SetNotifier(wsManager)` (FUTURO)

### 4.2 Servir Arquivos Estáticos (Frontend)
- [ ] Adicionar handler para servir arquivos estáticos em `/`
- [ ] Servir `index.html` para rotas não-API
- [ ] Configurar `dist/` ou similar como diretório de arquivos estáticos
- [ ] Testar que frontend será servido corretamente

### 4.3 Testes de Integração
- [ ] Testar inicialização completa do daemon
- [ ] Testar graceful shutdown
- [ ] Testar que API responde corretamente
- [ ] Testar que WebSocket funciona

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
- [ ] **Nota sobre testes:** A CLI é principalmente um wrapper que envia comandos HTTP e executa processos do sistema. Testes unitários não são prioritários aqui, pois a CLI apenas:
  - Faz parsing de argumentos (já testado indiretamente pelo uso)
  - Chama funções do cliente HTTP (que podem ser testadas isoladamente se necessário)
  - Executa comandos do sistema (difícil de testar unitariamente)
- [ ] Testes de integração serão implementados posteriormente para validar o fluxo completo:
  - CLI → API → Daemon
  - Gerenciamento de processo (start/stop)
  - Todos os comandos end-to-end

---

## Etapa 6: Setup Inicial do Frontend

**Objetivo:** Configurar projeto Svelte e estrutura básica.

### 6.1 Criar Projeto Svelte
- [ ] Executar `npm create svelte@latest` em `src/frontend/`
- [ ] Escolher template básico (SvelteKit ou SPA)
- [ ] Configurar Vite

### 6.2 Configurar Tailwind CSS
- [ ] Instalar Tailwind CSS: `npm install -D tailwindcss postcss autoprefixer`
- [ ] Inicializar Tailwind: `npx tailwindcss init -p`
- [ ] Configurar `tailwind.config.js` com paths do Svelte
- [ ] Adicionar diretivas do Tailwind ao CSS global
- [ ] Testar que Tailwind funciona

### 6.3 Estrutura de Rotas
- [ ] Criar estrutura de rotas (2 páginas inicialmente):
  - `/` - Dashboard/Lista de animes
  - `/config` - Configurações
- [ ] Configurar roteamento (svelte-spa-router ou similar)
- [ ] Criar layout base com navegação

### 6.4 Cliente API
- [ ] Criar `src/lib/api/client.js` (ou `.ts`)
- [ ] Implementar funções para chamadas HTTP com fetch
- [ ] Implementar cliente WebSocket
- [ ] Tratamento de erros básico
- [ ] Helpers para formatação de dados

---

## Etapa 7: Páginas do Frontend

**Objetivo:** Implementar as páginas principais da WebUI.

### 7.1 Página de Lista de Animes
- [ ] Criar componente `AnimeList.svelte`
- [ ] Buscar lista de animes da API
- [ ] Exibir lista com informações:
  - Nome do anime
  - Progresso (episódios assistidos)
  - Status (watching, completed, etc.)
  - Última verificação
- [ ] Adicionar loading state
- [ ] Adicionar tratamento de erros
- [ ] Estilizar com Tailwind

### 7.2 Página de Episódios
- [ ] Criar componente `EpisodeList.svelte`
- [ ] Buscar lista de episódios da API
- [ ] Exibir lista com:
  - Nome do episódio
  - Anime relacionado
  - Data de download
  - Status (baixado, em progresso)
- [ ] Adicionar filtros (por anime, data)
- [ ] Estilizar com Tailwind

### 7.3 Página de Configurações
- [ ] Criar componente `Config.svelte`
- [ ] Buscar configurações atuais da API
- [ ] Criar formulário para editar:
  - Anilist username
  - Save path
  - Completed anime path
  - Check interval
  - qBittorrent URL
  - Max episodes per anime
  - Episode retry limit
  - Delete watched episodes
  - Excluded list
- [ ] Implementar validação de formulário
- [ ] Salvar via API PUT
- [ ] Mostrar feedback de sucesso/erro
- [ ] Estilizar com Tailwind

### 7.4 Dashboard/Status
- [ ] Criar componente `Dashboard.svelte`
- [ ] Mostrar status do daemon (running/stopped)
- [ ] Mostrar estatísticas:
  - Animes monitorados
  - Episódios baixados
  - Última verificação
- [ ] Botões de controle: Start, Stop, Check
- [ ] Conectar via WebSocket para atualizações em tempo real
- [ ] Estilizar com Tailwind

- [ ] Filtrar por nível (DEBUG, INFO, WARN, ERROR)
- [ ] Estilizar com Tailwind

---

## Etapa 8: Integração Frontend-Backend

**Objetivo:** Integrar frontend com backend e testar fluxo completo.

### 8.1 Build do Frontend
- [ ] Configurar build do Svelte para gerar arquivos estáticos
- [ ] Configurar output para `dist/` ou diretório servido pelo daemon
- [ ] Testar build local

### 8.2 Integração com Daemon
- [ ] Configurar daemon para servir arquivos do frontend
- [ ] Testar que frontend carrega corretamente
- [ ] Testar que API responde corretamente do frontend
- [ ] Testar CORS se necessário

### 8.3 WebSocket no Frontend
- [ ] Conectar WebSocket na página Dashboard
- [ ] Receber atualizações de status em tempo real
- [ ] Implementar reconexão automática
- [ ] Testar desconexão e reconexão

### 8.4 Testes End-to-End
- [ ] Testar fluxo completo:
  1. Iniciar daemon
  2. Acessar WebUI
  3. Ver lista de animes
  4. Editar configurações
  5. Forçar verificação
  6. Ver episódios baixados
- [ ] Testar em diferentes navegadores
- [ ] Testar responsividade

---

## Etapa 9: Melhorias e Polimento

**Objetivo:** Adicionar features extras e melhorar qualidade.

### 9.1 Melhorias no Logging
- [ ] Configurar níveis de log por ambiente
- [ ] Adicionar mais contexto aos logs
- [ ] Melhorar formatação de logs em desenvolvimento

### 9.2 Validação e Configuração
- [ ] Adicionar validação robusta de configurações
- [ ] Suporte a variáveis de ambiente
- [ ] Migração automática de configurações antigas
- [ ] Valores padrão sensatos

### 9.3 Testes
- [ ] Aumentar cobertura de testes unitários
- [ ] Adicionar testes de integração para fluxos completos
- [ ] Criar mocks para serviços externos (Anilist, Nyaa, qBittorrent)
- [ ] Testes de performance

### 9.4 Documentação
- [ ] Documentar API (OpenAPI/Swagger ou similar)
- [ ] Criar guia de instalação
- [ ] Criar guia de uso da CLI
- [ ] Criar guia de uso da WebUI
- [ ] Atualizar README.md

### 9.5 Build e Deploy
- [ ] Configurar build para múltiplas plataformas
- [ ] Criar scripts de build
- [ ] Criar/atualizar systemd service file
- [ ] Criar/atualizar Windows service
- [ ] Testar instalação em ambiente limpo

---

## Etapa 10: Migração e Cleanup

**Objetivo:** Finalizar migração e remover código antigo.

### 10.1 Migração de Dados
- [ ] Garantir compatibilidade com configurações existentes
- [ ] Testar migração de dados de usuários existentes
- [ ] Documentar processo de migração

### 10.2 Remover Código Antigo
- [ ] Remover código da GUI Fyne (se não for mais necessário)
- [ ] Limpar dependências não utilizadas
- [ ] Remover arquivos temporários de desenvolvimento

### 10.3 Testes Finais
- [ ] Testar instalação completa do zero
- [ ] Testar todos os comandos da CLI
- [ ] Testar todas as funcionalidades da WebUI
- [ ] Testar em diferentes sistemas operacionais
- [ ] Testar com diferentes configurações

### 10.4 Release
- [ ] Atualizar versionamento
- [ ] Criar changelog
- [ ] Preparar release notes
- [ ] Tag de release

---

## Ordem de Prioridade

### Ordem de Implementação

A implementação segue esta ordem sequencial:

1. **Etapa 1: Logging e Refatoração Base do Daemon** ✅
   - Configurar zerolog
   - Refatorar daemon.go
   - Criar estrutura de estado
   - Testes

2. **Etapa 2: API REST do Daemon** ✅
   - Estrutura base da API
   - Handlers básicos (status, config)
   - Handlers de dados (animes, episodes)
   - Handlers de controle (check, daemon start/stop)
   - Documentação Swagger/OpenAPI
   - Gerenciamento de estado do daemon

3. **Etapa 4: Ponto de Entrada do Daemon** ✅
   - Criar main.go do daemon
   - Integrar tudo
   - Graceful shutdown

4. **Etapa 5: CLI Básica** ✅
   - Estrutura base da CLI
   - Cliente HTTP
   - Comandos básicos
   - Gerenciamento de processo
   - Formatação de saída

5. **Etapa 6: Setup Inicial do Frontend** (PRÓXIMO)
   - Criar projeto Svelte
   - Configurar Tailwind CSS
   - Estrutura de rotas
   - Cliente API

6. **Etapa 7: Páginas do Frontend** (PRÓXIMO)
   - Página de lista de animes
   - Página de episódios
   - Página de configurações
   - Dashboard/Status

7. **Etapa 8: Integração Frontend-Backend** (PRÓXIMO)
   - Build do frontend
   - Integração com daemon
   - Servir arquivos estáticos

8. **Etapa 3: WebSocket para Tempo Real** (FUTURO)
   - Servidor WebSocket
   - Integração com daemon
   - Reconexão e robustez
   - WebSocket no frontend

### Desejável (Polimento)
- Etapa 9: Melhorias e Polimento
- Etapa 10: Migração e Cleanup

---

## Progresso Atual

### Etapa 1: ✅ Concluída (incluindo testes)
- **1.1 Configurar Zerolog**: ✅ Implementado com suporte a desenvolvimento e produção
  - ✅ Gravação de logs em arquivo com rotação (lumberjack)
  - ✅ Multi-writer: console e arquivo simultaneamente
  - ✅ Formato JSON no arquivo, console formatado em dev
  - ✅ Rotação automática: 10MB por arquivo, 5 backups, 30 dias, compressão
- **1.2 Refatorar daemon.go**: ✅ Removidas dependências de UI, integrado zerolog, adicionado contexto
- **1.3 Estrutura de Estado**: ✅ Implementado com StateNotifier para notificações automáticas
- **1.4 Testes**: ✅ Testes completos para logger, state e refatoração do daemon
  - ✅ 12 testes para logger (inicialização, níveis, formatação, caller info, helpers, campos estruturados)
  - ✅ 13 testes para state (thread-safety, notificações, Get/Set, GetAll, sem notifier)
  - ✅ 7 testes para refatoração do daemon (erros registrados, cancelamento, logs gerados, transições de status)

**Correções realizadas:**
- Corrigido tracking de erros: `fetchDownloadedTorrents()` e `searchAnilist()` agora retornam erros e são registrados no state
- Corrigido código comentado corrompido em `src/tests/anilist_test.go`
- Corrigido `notifyChange()` para sempre retornar snapshot válido
- Corrigido limpeza de erro em caso de cancelamento
- Corrigido status "checking" para ser visível durante toda a execução de `AnimeVerification`

### Etapa 2: ✅ Concluída
- **2.1-2.7**: ✅ API REST completa implementada com todos os endpoints
- ✅ Documentação Swagger/OpenAPI
- ✅ Gerenciamento de estado do daemon (status interno)
- ✅ Correção do status "checking" para ser visível durante verificação

### Etapa 4: ✅ Concluída
- ✅ Ponto de entrada do daemon (`src/cmd/daemon/main.go`)
- ✅ Integração completa com API REST
- ✅ Graceful shutdown
- ✅ Inicialização automática do loop

### Etapa 5: ✅ Concluída
- ✅ CLI completa com todos os comandos
- ✅ Gerenciamento de processo (start/stop)
- ✅ Cliente HTTP para comunicação com daemon
- ✅ Formatação de saída (tabelas e JSON)
- ✅ Help melhorado para `config set` com lista de keys
- ✅ Correção do comando `logs` para encontrar arquivo de log

**Próximos passos:**
- Etapa 6: Setup Inicial do Frontend

---

## Notas de Implementação

### Dependências Entre Etapas
- **Ordem de implementação:**
  1. Etapa 1: Logging e Refatoração Base (base do daemon)
  2. Etapa 2: API REST (comunicação)
  3. Etapa 4: Ponto de Entrada do Daemon (integração)
  4. Etapa 5: CLI (interface de linha de comando)
  5. Etapa 6: Setup Frontend (estrutura)
  6. Etapa 7: Páginas Frontend (componentes)
  7. Etapa 8: Integração Frontend-Backend (servir arquivos estáticos)
  8. Etapa 3: WebSocket (tempo real - pode ser feito após frontend básico)
- **Dependências específicas:**
  - Etapa 1 deve ser completada antes da Etapa 2 (logging necessário)
  - Etapa 2 deve ser completada antes da Etapa 4 (API necessária)
  - Etapa 4 deve ser completada antes da Etapa 5 (daemon precisa estar rodando)
  - Etapa 6 deve ser completada antes da Etapa 7 (estrutura necessária)
  - Etapa 7 pode ser feita parcialmente antes da Etapa 8 (componentes isolados)
  - Etapa 3 (WebSocket) pode ser implementada após Etapa 8 ou em paralelo

### Testes Contínuos
- **Todas as novas features devem ter testes correspondentes**
- Testar cada etapa antes de prosseguir
- Manter testes passando durante refatoração
- Não quebrar funcionalidade existente durante migração
- **Estratégia de testes**:
  - Testes unitários para funções e métodos isolados
  - Testes de integração para componentes que interagem
  - Testes de comportamento para lógica complexa
  - Usar mocks para dependências externas (Anilist, Nyaa, qBittorrent)
  - Executar `go test -race` para detectar race conditions
  - Manter cobertura de código > 70%

### Commits
- Commits pequenos e frequentes
- Mensagens descritivas seguindo convenções
- Um commit por feature/funcionalidade

### Documentação
- Documentar decisões importantes
- Comentar código complexo
- Atualizar documentação conforme implementa

---

## Checklist de Validação Final

Antes de considerar a refatoração completa, verificar:

- [ ] Daemon inicia e para corretamente
- [ ] API REST responde a todos os endpoints
- [ ] WebSocket funciona e reconecta automaticamente
- [ ] CLI funciona para todos os comandos
- [ ] WebUI carrega e todas as páginas funcionam
- [ ] Logs são estruturados e úteis para debug
- [ ] Configurações são salvas e carregadas corretamente
- [ ] Downloads de animes ainda funcionam
- [ ] Testes passam
- [ ] Documentação está atualizada
- [ ] Código segue boas práticas definidas

