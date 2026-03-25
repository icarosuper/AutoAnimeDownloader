# Plano de Melhorias de UI/UX — Web UI

## Decisões de Design e Tecnologia

- **Estilo:** tecnológico minimalista, limpo e legível
- **Biblioteca de componentes:** DaisyUI (plugin Tailwind, zero JS — só CSS)
- **Fonte:** Inter via `@fontsource/inter` (npm, sem dependência de CDN externo)
- **Tema base:** dark tecnológico com acento em cyan/indigo — temas e cores definidos iterativamente durante a implementação
- **Light mode:** mantido e refinado junto com o dark
- **Package manager:** migrar de npm para **Bun** (drop-in replacement, mais rápido; Svelte + Vite são totalmente compatíveis)
- **i18n:** **paraglide-js** (Inlang) — zero runtime, geração estática por locale, integração nativa com Svelte 5
  - Idiomas: Português (pt-BR) e Inglês (en)
  - Todo texto visível no frontend deve ter tradução nos dois idiomas
  - Código-fonte, mensagens da CLI e logs do backend permanecem em inglês
- As sessões abaixo devem ser implementadas já usando DaisyUI + Inter

## Ordem de Implementação

0. Setup base: instalar DaisyUI, @fontsource/inter, paraglide-js; migrar para Bun; configurar temas
1. Sistema de Toasts
2+8. Redesign do Dashboard + Busca de Animes
3. Barras de Progresso nos Animes
4. Indicador de Status do WebSocket
5. Agrupamento das Configurações em Seções
6. Confirmação em Ações Destrutivas
7. Logs — Altura Dinâmica e Auto-scroll
9. Atualização Dinâmica de Datas Relativas
10. Empty States

---

## 0. Setup Base
- [ ] Migrar de npm para Bun (`bun install`, atualizar scripts no package.json e Makefile/scripts de build)
- [ ] Instalar DaisyUI: `bun add -d daisyui`
- [ ] Instalar Inter: `bun add @fontsource/inter`
- [ ] Instalar paraglide-js e configurar locales pt-BR e en
- [ ] Configurar DaisyUI no `tailwind.config.js` com os temas dark e light
- [ ] Importar fonte Inter no `app.css`
- [ ] Adaptar `theme.ts` para setar `data-theme` no `<html>` junto com a classe `dark` do Tailwind

## 1. Sistema de Toasts
- [ ] Criar store `src/lib/stores/toast.ts` com funções `addToast` e `removeToast`
- [ ] Criar componente `Toast.svelte` com suporte a tipos: success, error, warning, info
- [ ] Toast aparece no canto inferior direito e some após 4 segundos
- [ ] Substituir todos os banners/alerts inline de Config.svelte, Status.svelte e AnimeDetail.svelte por toasts
- [ ] Suporte a múltiplos toasts empilhados
- [ ] Animação de entrada/saída (slide + fade)
- [ ] Textos dos toasts traduzidos em pt-BR e en

## 2+8. Redesign do Dashboard + Busca de Animes (Status.svelte)
- [ ] Remover o limite hardcoded de Top 10 animes — exibir todos
- [ ] Separar os stat blocks em cards visuais distintos:
  - Status do daemon (com badge colorido)
  - Última verificação (com timeAgo atualizado dinamicamente)
  - Próxima verificação (countdown regressivo baseado no check_interval da config)
  - Total de animes monitorados
  - Total de episódios baixados
- [ ] Adicionar barra de busca por nome no topo da lista de animes
- [ ] Filtro reativo com botão "×" para limpar e contador "Exibindo X de Y animes"
- [ ] Empty state: card com ícone + mensagem orientativa
- [ ] Todos os textos traduzidos em pt-BR e en

## 3. Barras de Progresso nos Animes
- [ ] Exibir barra de progresso `episodios_baixados / total_episodios` em cada linha/card de anime
- [ ] Mostrar o texto "X/Y eps" ao lado da barra
- [ ] Colorir a barra: verde se completo, azul se em progresso, cinza se total desconhecido

## 4. Indicador de Status do WebSocket
- [ ] Adicionar dot colorido no header (Layout.svelte):
  - Verde: conectado
  - Amarelo: reconectando
  - Vermelho: desconectado
- [ ] Tooltip com mensagem traduzida: "Atualização em tempo real ativa" / "Reconectando..."
- [ ] Expor estado da conexão pelo `WebSocketClient` para consumo pelo Layout
- [ ] Atualizar o dot automaticamente conforme o estado da conexão muda

## 5. Agrupamento das Configurações em Seções (Config.svelte)
- [ ] Dividir o formulário em seções com títulos e separadores visuais:
  - **Anilist** — anilist_username
  - **Downloads** — save_path, completed_anime_path, delete_watched_episodes, watched_episodes_to_keep
  - **Automação** — check_interval, max_episodes_per_anime, episode_retry_limit
  - **qBittorrent** — qbittorrent_url
  - **Filtros** — excluded_list
- [ ] Mostrar valor padrão como placeholder em cada campo
- [ ] Manter a validação e feedback de erro por campo
- [ ] Todos os labels e mensagens traduzidos em pt-BR e en

## 6. Confirmação em Ações Destrutivas (AnimeDetail.svelte)
- [ ] Criar componente `ConfirmDialog.svelte` usando modal do DaisyUI
- [ ] Exibir o dialog antes de executar DELETE de episódio, com o nome do episódio na mensagem
- [ ] Botão de confirmação em vermelho (danger)
- [ ] Textos do dialog traduzidos em pt-BR e en

## 7. Logs — Altura Dinâmica e Auto-scroll (Logs.svelte)
- [ ] Substituir `height: 600px` fixo por `height: calc(100vh - Xpx)` para ocupar o restante da viewport
- [ ] Adicionar toggle "Auto-scroll" (ligado por padrão) — quando ativo, scrolla para o fim ao carregar novos logs
- [ ] Adicionar botão "Ir para o fim" fixo no canto inferior do container
- [ ] Adicionar botão "Copiar" em cada linha de log (aparece no hover)
- [ ] Textos e labels traduzidos em pt-BR e en

## 9. Atualização Dinâmica de Datas Relativas
- [ ] Criar store com `setInterval` de 60 segundos que atualiza variável `now` via `$state`
- [ ] Re-renderizar os campos `formatTimeAgo()` em Status.svelte e AnimeDetail.svelte sem re-fetchar dados
- [ ] Usar reatividade do Svelte 5 para atualizar apenas os componentes de data

## 10. Empty States
- [ ] Status.svelte — sem animes: card com ícone + texto orientativo
- [ ] AnimeDetail.svelte — sem episódios: mensagem traduzida
- [ ] Logs.svelte — sem logs após filtro: mensagem traduzida
- [ ] Todos os textos traduzidos em pt-BR e en
