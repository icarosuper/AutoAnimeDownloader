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
- [ ] Criar testes para logger (`src/internal/logger/logger_test.go`):
  - Testar inicialização em modo desenvolvimento e produção
  - Testar diferentes níveis de log
  - Testar formatação de output (console vs JSON)
  - Testar caller information
- [ ] Criar testes para state (`src/daemon/state_test.go`):
  - Testar thread-safety (múltiplas goroutines acessando simultaneamente)
  - Testar notificações quando estado muda
  - Testar métodos Get/Set de todos os campos
  - Testar `GetAll()` retorna snapshot consistente
  - Testar que notificações não são chamadas quando não há mudança real
- [ ] Criar testes para refatoração do daemon:
  - Testar que erros são registrados corretamente no state
  - Testar que contexto de cancelamento funciona
  - Testar que logs são gerados corretamente

---

## Etapa 2: API REST do Daemon

**Objetivo:** Criar API REST completa para comunicação com CLI e WebUI.

### 2.1 Estrutura Base da API
- [ ] Criar `src/internal/api/server.go` com servidor HTTP básico
- [ ] Configurar roteamento com `net/http`
- [ ] Criar middleware para:
  - Logging de requisições
  - CORS (se necessário)
  - Content-Type JSON
- [ ] Implementar graceful shutdown

### 2.2 Handlers Básicos
- [ ] Criar `src/internal/api/handlers.go`
- [ ] Implementar handler de status: `GET /api/v1/status`
  - Retornar estado completo do daemon:
    - Status atual (stopped/running/checking)
    - Timestamp da última verificação
    - Se houve erro na última checagem (boolean)
  - Formato JSON padronizado
  - Usado pela WebUI para exibir informações do daemon
- [ ] Implementar handler de configuração: `GET /api/v1/config`
  - Retornar configurações atuais
- [ ] Implementar handler de atualização: `PUT /api/v1/config`
  - Validar entrada
  - Salvar configurações
  - Retornar erro se inválido

### 2.3 Handlers de Dados
- [ ] Implementar `GET /api/v1/animes`
  - Listar animes monitorados
  - Incluir informações de progresso
- [ ] Implementar `GET /api/v1/episodes`
  - Listar episódios baixados
  - Incluir hash e nome do episódio
- [ ] Implementar `GET /api/v1/logs`
  - Retornar logs recentes (últimas N linhas)
  - Suportar filtro por nível

### 2.4 Handlers de Controle
- [ ] Implementar `POST /api/v1/check`
  - Forçar verificação manual
  - Executar em goroutine separada
  - Retornar imediatamente (async)
  - Retornar apenas confirmação (não retornar estado)
- [ ] Implementar `POST /api/v1/daemon/start`
  - Iniciar loop de verificação
  - Atualizar estado internamente
  - Retornar apenas confirmação (não retornar estado)
  - Estado será atualizado via WebSocket
- [ ] Implementar `POST /api/v1/daemon/stop`
  - Parar loop de verificação
  - Atualizar estado internamente
  - Retornar apenas confirmação (não retornar estado)
  - Estado será atualizado via WebSocket
  - Graceful shutdown

### 2.5 Estrutura de Resposta Padrão
- [ ] Criar tipos de resposta padronizados:
  - `SuccessResponse` com `success`, `data`, `error`
  - `ErrorResponse` com código e mensagem
- [ ] Criar helpers para serializar respostas
- [ ] Implementar tratamento de erros consistente

### 2.6 Testes da API
- [ ] Criar testes unitários para handlers (`src/internal/api/handlers_test.go`):
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
- [ ] Nota: Logs em tempo real podem ser implementados futuramente se necessário

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

### 4.1 Criar `src/cmd/daemon/main.go`
- [ ] Inicializar logger zerolog
- [ ] Carregar configurações
- [ ] Inicializar FileManager
- [ ] Criar instância do daemon state
- [ ] Inicializar servidor HTTP com API REST
- [ ] Inicializar servidor WebSocket
- [ ] **Injetar WebSocket manager no state:** `state.SetNotifier(wsManager)`
- [ ] Configurar graceful shutdown (SIGINT, SIGTERM)
- [ ] Iniciar loop de verificação (se configurado para auto-start)

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

### 5.1 Estrutura Base da CLI
- [ ] Criar `src/cmd/cli/main.go`
- [ ] Configurar `urfave/cli` com app básico
- [ ] Definir flags globais (endpoint do daemon, formato de saída)

### 5.2 Cliente HTTP
- [ ] Criar `src/internal/api/client.go` (ou similar)
- [ ] Implementar cliente HTTP para comunicação com daemon
- [ ] Tratar erros de conexão
- [ ] Implementar timeout
- [ ] Criar funções helper para cada endpoint

### 5.3 Comandos Básicos
- [ ] Implementar `status` - mostrar status do daemon
- [ ] Implementar `config get` - mostrar configurações
- [ ] Implementar `config set <key> <value>` - atualizar configuração
- [ ] Implementar `check` - forçar verificação manual
- [ ] Implementar `list animes` - listar animes monitorados
- [ ] Implementar `list episodes` - listar episódios baixados
- [ ] Implementar `logs` - mostrar logs recentes

### 5.4 Gerenciamento de Processo
- [ ] Criar `src/internal/daemon/process.go`
- [ ] Implementar detecção se daemon está rodando (verificar PID file ou conexão)
- [ ] Implementar `start` - iniciar daemon como processo separado
- [ ] Implementar `stop` - parar daemon (enviar sinal ou chamar API)
- [ ] Implementar `restart` - parar e iniciar
- [ ] Gerenciar PID file

### 5.5 Formatação de Saída
- [ ] Implementar formatação em tabela (padrão)
- [ ] Implementar formatação JSON (`--json` flag)
- [ ] Implementar cores para terminal (opcional)

### 5.6 Testes da CLI
- [ ] Testar cada comando isoladamente
- [ ] Testar tratamento de erros
- [ ] Testar formatação de saída

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

### 7.5 Página de Logs (Opcional)
- [ ] Criar componente `Logs.svelte`
- [ ] Conectar via WebSocket para logs em tempo real
- [ ] Exibir logs com scroll automático
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
- [ ] Receber logs em tempo real (se implementado)
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
- [ ] Implementar rotação de logs
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

### Crítico (MVP)
1. Etapa 0: Preparação
2. Etapa 1: Logging e Refatoração Base
3. Etapa 2: API REST Básica (status, config, check)
4. Etapa 4: Ponto de Entrada do Daemon
5. Etapa 5: CLI Básica (status, config, check)
6. Etapa 6: Setup Frontend
7. Etapa 7: Páginas Básicas (lista animes, configurações)
8. Etapa 8: Integração Básica

### Importante (Funcionalidade Completa)
9. Etapa 2: Handlers de Dados (animes, episodes)
10. Etapa 3: WebSocket
11. Etapa 7: Dashboard e Logs
12. Etapa 8: WebSocket no Frontend

### Desejável (Polimento)
13. Etapa 9: Melhorias e Polimento
14. Etapa 10: Migração e Cleanup

---

## Progresso Atual

### Etapa 1: ✅ Concluída
- **1.1 Configurar Zerolog**: ✅ Implementado com suporte a desenvolvimento e produção
- **1.2 Refatorar daemon.go**: ✅ Removidas dependências de UI, integrado zerolog, adicionado contexto
- **1.3 Estrutura de Estado**: ✅ Implementado com StateNotifier para notificações automáticas

**Correções realizadas:**
- Corrigido tracking de erros: `fetchDownloadedTorrents()` e `searchAnilist()` agora retornam erros e são registrados no state
- Corrigido código comentado corrompido em `src/tests/anilist_test.go`

**Próximos passos:**
- Etapa 2: API REST do Daemon

---

## Notas de Implementação

### Dependências Entre Etapas
- Etapa 1 deve ser completada antes da Etapa 2 (logging necessário)
- Etapa 2 deve ser completada antes da Etapa 4 (API necessária)
- Etapa 4 deve ser completada antes da Etapa 5 (daemon precisa estar rodando)
- Etapa 6 deve ser completada antes da Etapa 7 (estrutura necessária)
- Etapa 7 pode ser feita parcialmente antes da Etapa 8 (componentes isolados)

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

