<script lang="ts">
  import { onMount } from "svelte";
  import Checkbox from "../components/ui/Checkbox.svelte";
  import {
    getConfig,
    updateConfig,
    testWebhook,
    type Config,
    type WebhookPreset,
  } from "../lib/api/client.js";
  import Loading from "../components/Loading.svelte";
  import Input from "../components/Input.svelte";
  import { toast } from "../lib/stores/toast.js";
  import * as m from "../lib/i18n/messages.js";
  import { locale } from "../lib/stores/locale.js";

  $: T = $locale && {
    title: m.notifications_title(),
    subtitle: m.notifications_subtitle(),
    loading: m.notifications_loading(),
    sectionWebhooks: m.notifications_section_webhooks(),
    btnAdd: m.notifications_btn_add(),
    btnTest: m.notifications_btn_test(),
    btnRemove: m.notifications_btn_remove(),
    btnConfirm: m.notifications_btn_confirm(),
    btnCancel: m.common_cancel(),
    btnSave: m.config_btn_save(),
    btnSaving: m.config_btn_saving(),
    labelName: m.notifications_label_name(),
    labelMethod: m.notifications_label_method(),
    labelUrl: m.notifications_label_url(),
    labelHeaders: m.notifications_label_headers(),
    labelBody: m.notifications_label_body(),
    presetLabel: m.notifications_preset_label(),
    btnEdit: m.notifications_btn_edit(),
    sectionBatch: m.notifications_section_batch(),
    labelBatchWindow: m.notifications_label_batch_window(),
    hintBatchWindow: m.notifications_hint_batch_window(),
    labelEvents: m.notifications_label_events(),
    eventNewEpisode: m.notifications_event_new_episode(),
    eventDownloadFailed: m.notifications_event_download_failed(),
    eventDownloadCompleted: m.notifications_event_download_completed(),
  };

  const ALL_EVENTS = ['new_episode', 'download_failed', 'download_completed'] as const;

  const WEBHOOK_PRESETS: Record<string, WebhookPreset> = {
    ntfy:     { name: 'ntfy',     url: 'https://ntfy.sh/CHANGE_ME',                                    method: 'POST', headers: { Title: '{{title}}', Priority: 'default' },         body: '{{message}}',                                                                                                                                            events: [...ALL_EVENTS] },
    gotify:   { name: 'Gotify',   url: 'http://YOUR_GOTIFY_URL/message?token=CHANGE_ME',               method: 'POST', headers: { 'Content-Type': 'application/json' },               body: '{"title":"{{title}}","message":"{{message}}","priority":5}',                                                                                            events: [...ALL_EVENTS] },
    discord:  { name: 'Discord',  url: 'https://discord.com/api/webhooks/CHANGE_ME',                   method: 'POST', headers: { 'Content-Type': 'application/json' },               body: '{"content":"**{{title}}**\\n{{message}}"}',                                                                                                              events: [...ALL_EVENTS] },
    telegram: { name: 'Telegram', url: 'https://api.telegram.org/botCHANGE_TOKEN/sendMessage',         method: 'POST', headers: { 'Content-Type': 'application/json' },               body: '{"chat_id":"CHANGE_CHAT_ID","text":"*{{title}}*\\n{{message}}","parse_mode":"Markdown"}',                                                                events: [...ALL_EVENTS] },
    pushover: { name: 'Pushover', url: 'https://api.pushover.net/1/messages.json',                     method: 'POST', headers: { 'Content-Type': 'application/json' },               body: '{"token":"CHANGE_APP_TOKEN","user":"CHANGE_USER_KEY","title":"{{title}}","message":"{{message}}"}',                                                      events: [...ALL_EVENTS] },
    slack:    { name: 'Slack',    url: 'https://hooks.slack.com/services/CHANGE_ME',                   method: 'POST', headers: { 'Content-Type': 'application/json' },               body: '{"text":"*{{title}}*\\n{{message}}"}',                                                                                                                   events: [...ALL_EVENTS] },
    apprise:  { name: 'Apprise',  url: 'http://YOUR_APPRISE_URL/notify/CHANGE_TAG',                    method: 'POST', headers: { 'Content-Type': 'application/json' },               body: '{"title":"{{title}}","body":"{{message}}"}',                                                                                                             events: [...ALL_EVENTS] },
    // Nao sao notificacoes: disparam um scan da biblioteca. So em download_completed — nao ha o que
    // escanear quando o episodio foi apenas detectado ou o download falhou.
    jellyfin: { name: 'Jellyfin', url: 'http://YOUR_JELLYFIN_URL/Library/Refresh',                     method: 'POST', headers: { 'X-Emby-Token': 'CHANGE_API_KEY' },                  body: '',                                                                                                                                                      events: ['download_completed'] },
    plex:     { name: 'Plex',     url: 'http://YOUR_PLEX_URL/library/sections/CHANGE_ID/refresh?X-Plex-Token=CHANGE_TOKEN', method: 'GET', headers: {},                                 body: '',                                                                                                                                                      events: ['download_completed'] },
  };

  const bodyPlaceholder = '{"message":"{{message}}"}';
  const varsHint = 'Variables: {{title}}, {{message}}, {{anime_name}}, {{episode}}, {{reason}}, {{timestamp}}';

  let fullConfig: Config | null = null;
  let notifications: { webhooks: WebhookPreset[]; batch_window_seconds: number } = { webhooks: [], batch_window_seconds: 0 };
  let savedWebhookNames = new Set<string>();
  let loading = true;
  let saving = false;

  let showWebhookForm = false;
  let editingIndex: number | null = null;
  let newWebhook: WebhookPreset = { name: '', url: '', method: 'POST', headers: {}, body: '', events: [...ALL_EVENTS] };

  // Os headers viram lista de pares ENQUANTO o formulario esta aberto, e so voltam a ser
  // Record<string,string> no Confirmar. O objeto nao serve para editar: renomear uma chave em
  // Record e apagar e recriar a entrada, e a linha pula de lugar a cada tecla. Ver
  // decisions.md #87.
  let headerRows: { key: string; value: string }[] = [{ key: '', value: '' }];

  const toRows = (headers: Record<string, string>) =>
    Object.entries(headers).map(([key, value]) => ({ key, value }));

  // Linha sem nome e descartada: o + adiciona uma linha vazia, e sair sem preencher nao pode
  // gravar um header "".
  const fromRows = (rows: { key: string; value: string }[]) =>
    Object.fromEntries(rows.filter(r => r.key.trim()).map(r => [r.key.trim(), r.value]));

  function resetForm() {
    newWebhook = { name: '', url: '', method: 'POST', headers: {}, body: '', events: [...ALL_EVENTS] };
    headerRows = [{ key: '', value: '' }];
    editingIndex = null;
    showWebhookForm = false;
  }

  function applyPreset(key: string) {
    newWebhook = { ...WEBHOOK_PRESETS[key] };
    headerRows = toRows(WEBHOOK_PRESETS[key].headers);
  }

  function editWebhook(index: number) {
    newWebhook = { ...notifications.webhooks[index], headers: { ...notifications.webhooks[index].headers }, events: [...(notifications.webhooks[index].events ?? [])] };
    headerRows = toRows(notifications.webhooks[index].headers);
    editingIndex = index;
    showWebhookForm = true;
  }

  function confirmWebhook() {
    if (!newWebhook.name || !newWebhook.url) return;
    const webhook = { ...newWebhook, headers: fromRows(headerRows) };
    if (editingIndex !== null) {
      notifications.webhooks = notifications.webhooks.map((h, i) => i === editingIndex ? webhook : h);
    } else {
      notifications.webhooks = [...notifications.webhooks, webhook];
    }
    resetForm();
  }

  function removeWebhook(index: number) {
    notifications.webhooks = notifications.webhooks.filter((_, i) => i !== index);
  }

  async function testWebhookHandler(name: string) {
    try {
      await testWebhook(name);
      toast.success(m.notifications_toast_test_ok({ name }));
    } catch (e) {
      toast.error(e instanceof Error ? e.message : m.notifications_toast_test_err());
    }
  }

  function addHeaderRow() {
    headerRows = [...headerRows, { key: '', value: '' }];
  }

  function removeHeaderRow(index: number) {
    headerRows = headerRows.filter((_, i) => i !== index);
  }

  async function loadNotifications() {
    loading = true;
    try {
      fullConfig = await getConfig();
      notifications = fullConfig.notifications ?? { webhooks: [], batch_window_seconds: 0 };
      savedWebhookNames = new Set(notifications.webhooks.map(h => h.name));
    } finally {
      loading = false;
    }
  }

  async function saveNotifications() {
    if (!fullConfig) return;
    saving = true;
    try {
      await updateConfig({ ...fullConfig, notifications });
      fullConfig = await getConfig();
      notifications = fullConfig.notifications ?? { webhooks: [], batch_window_seconds: 0 };
      savedWebhookNames = new Set(notifications.webhooks.map(h => h.name));
      toast.success(m.notifications_toast_saved());
    } catch (e) {
      toast.error(e instanceof Error ? e.message : m.notifications_toast_save_err());
    } finally {
      saving = false;
    }
  }

  onMount(loadNotifications);
</script>

<div class="max-w-2xl mx-auto">
  <div class="mb-6">
    <h1 class="text-2xl font-bold text-heading">{T && T.title}</h1>
    <p class="text-subtle text-sm mt-1">{T && T.subtitle}</p>
  </div>

  {#if loading}
    <Loading />
  {:else}
    <div class="flex flex-col gap-6">
      <div class="flex flex-col rounded-card border border-default bg-sunken">
        <div class="flex flex-col gap-4 p-5">
          <h2 class="text-sm font-semibold text-subtle uppercase tracking-wider">{T && T.sectionBatch}</h2>

          <Input
            id="batch_window_seconds"
            label={T && T.labelBatchWindow || ""}
            subtitle={T && T.hintBatchWindow || ""}
            type="number"
            bind:value={notifications.batch_window_seconds}
            min="0"
          />
        </div>
      </div>

      <div class="flex flex-col rounded-card border border-default bg-sunken">
        <div class="flex flex-col gap-4 p-5">
          <h2 class="text-sm font-semibold text-subtle uppercase tracking-wider">{T && T.sectionWebhooks}</h2>

          {#if notifications.webhooks.length > 0}
            <div class="flex flex-col gap-2">
              {#each notifications.webhooks as hook, i}
                <div class="flex items-center justify-between gap-2 px-3 py-2 rounded-md bg-card border border-default">
                  <div class="flex-1 min-w-0">
                    <span class="text-sm font-medium text-heading">{hook.name}</span>
                    <span class="text-xs text-subtle ml-2 truncate">{hook.url.length > 50 ? hook.url.slice(0, 50) + '…' : hook.url}</span>
                  </div>
                  <div class="flex gap-2 shrink-0">
                    {#if savedWebhookNames.has(hook.name)}
                      <button
                        type="button"
                        on:click={() => testWebhookHandler(hook.name)}
                        class="inline-flex items-center px-2 py-1 rounded text-xs border border-default text-body hover:bg-control transition-colors"
                      >
                        {T && T.btnTest}
                      </button>
                    {/if}
                    <button
                      type="button"
                      on:click={() => editWebhook(i)}
                      class="inline-flex items-center px-2 py-1 rounded text-xs border border-default text-body hover:bg-control transition-colors"
                    >
                      {T && T.btnEdit}
                    </button>
                    <button
                      type="button"
                      on:click={() => removeWebhook(i)}
                      class="inline-flex items-center px-2 py-1 rounded text-xs border border-danger-tint/28 text-danger hover:bg-danger-tint/12 transition-colors"
                    >
                      {T && T.btnRemove}
                    </button>
                  </div>
                </div>
              {/each}
            </div>
          {/if}

          {#if !showWebhookForm}
            <button
              type="button"
              on:click={() => { showWebhookForm = true; }}
              class="inline-flex items-center gap-1 px-3 py-2 rounded-md border border-dashed border-default text-body hover:bg-control text-sm transition-colors w-fit"
            >
              + {T && T.btnAdd}
            </button>
          {:else}
            <div class="flex flex-col gap-3 p-4 rounded-md border border-default bg-card">
              <div>
                <p class="text-xs text-subtle mb-2">{T && T.presetLabel}</p>
                <div class="flex flex-wrap gap-2">
                  {#each Object.keys(WEBHOOK_PRESETS) as key}
                    <button
                      type="button"
                      on:click={() => applyPreset(key)}
                      class="px-3 py-1 rounded-full text-xs border border-default text-body hover:bg-control transition-colors"
                    >
                      {WEBHOOK_PRESETS[key].name}
                    </button>
                  {/each}
                </div>
              </div>

              <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
                <div class="flex flex-col gap-1">
                  <label for="webhook-name" class="text-xs font-medium text-heading">{T && T.labelName}</label>
                  <input
                    id="webhook-name"
                    type="text"
                    bind:value={newWebhook.name}
                    placeholder="ex: ntfy"
                    class="block rounded-md border-default bg-control text-heading shadow-sm focus:border-accent text-sm px-3 py-2"
                  />
                </div>
                <div class="flex flex-col gap-1">
                  <label for="webhook-method" class="text-xs font-medium text-heading">{T && T.labelMethod}</label>
                  <select
                    id="webhook-method"
                    bind:value={newWebhook.method}
                    class="block rounded-md border-default bg-control text-heading shadow-sm focus:border-accent text-sm px-3 py-2"
                  >
                    <option>POST</option>
                    <option>GET</option>
                    <option>PUT</option>
                  </select>
                </div>
              </div>

              <div class="flex flex-col gap-1">
                <label for="webhook-url" class="text-xs font-medium text-heading">{T && T.labelUrl}</label>
                <input
                  id="webhook-url"
                  type="text"
                  bind:value={newWebhook.url}
                  placeholder="https://ntfy.sh/meu-topico"
                  class="block w-full rounded-md border-default bg-control text-heading shadow-sm focus:border-accent text-sm px-3 py-2"
                />
              </div>

              <div class="flex flex-col gap-2" role="group" aria-labelledby="webhook-headers-label">
                <span id="webhook-headers-label" class="text-xs font-medium text-heading">{T && T.labelHeaders}</span>
                {#each headerRows as row, i}
                  <div class="flex gap-2">
                    <input
                      type="text"
                      bind:value={row.key}
                      placeholder="Header"
                      aria-label="Header {i + 1}"
                      class="flex-1 rounded-md border-default bg-control text-heading text-xs px-2 py-1.5"
                    />
                    <input
                      type="text"
                      bind:value={row.value}
                      placeholder="Value"
                      aria-label="Value {i + 1}"
                      class="flex-1 rounded-md border-default bg-control text-heading text-xs px-2 py-1.5"
                    />
                    <button
                      type="button"
                      on:click={() => removeHeaderRow(i)}
                      class="px-2 py-1 text-xs text-subtle hover:text-danger"
                      aria-label="{T && T.labelHeaders}: {row.key}"
                    >✕</button>
                  </div>
                {/each}
                <button
                  type="button"
                  on:click={addHeaderRow}
                  class="self-start px-2 py-1 rounded border border-default text-xs text-body hover:bg-control"
                >+</button>
              </div>

              <div class="flex flex-col gap-1">
                <label for="webhook-body" class="text-xs font-medium text-heading">{T && T.labelBody}</label>
                <textarea
                  id="webhook-body"
                  bind:value={newWebhook.body}
                  rows="3"
                  placeholder={bodyPlaceholder}
                  class="block w-full rounded-md border-default bg-control text-heading shadow-sm focus:border-accent text-sm px-3 py-2 font-mono"
                ></textarea>
                <p class="text-xs text-subtle">{varsHint}</p>
              </div>

              <div class="flex flex-col gap-2">
                <span id="webhook-events-label" class="text-xs font-medium text-heading">{T && T.labelEvents}</span>
                <div class="flex flex-col gap-1" role="group" aria-labelledby="webhook-events-label">
                  {#each [
                    { value: 'new_episode',        label: T && T.eventNewEpisode },
                    { value: 'download_failed',    label: T && T.eventDownloadFailed },
                    { value: 'download_completed', label: T && T.eventDownloadCompleted },
                  ] as ev}
                    <Checkbox
                      label={ev.label || ""}
                      checked={newWebhook.events.includes(ev.value)}
                      on:change={(e) => {
                        if ((e.target as HTMLInputElement).checked) {
                          newWebhook.events = [...newWebhook.events, ev.value];
                        } else {
                          newWebhook.events = newWebhook.events.filter(v => v !== ev.value);
                        }
                      }}
                    />
                  {/each}
                </div>
              </div>

              <div class="flex gap-2">
                <button
                  type="button"
                  on:click={confirmWebhook}
                  disabled={!newWebhook.name || !newWebhook.url}
                  class="inline-flex items-center px-3 py-2 rounded-md bg-accent text-on-accent hover:opacity-90 text-sm font-medium transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  {T && T.btnConfirm}
                </button>
                <button
                  type="button"
                  on:click={resetForm}
                  class="inline-flex items-center px-3 py-2 rounded-md border border-default text-body hover:bg-control text-sm transition-colors"
                >
                  {T && T.btnCancel}
                </button>
              </div>
            </div>
          {/if}
        </div>
      </div>

      <div class="flex justify-end pt-2">
        <button
          type="button"
          on:click={saveNotifications}
          disabled={saving}
          class="inline-flex items-center gap-2 px-4 py-2 rounded-lg bg-accent text-on-accent hover:opacity-90 font-medium text-sm transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
        >
          {saving ? (T && T.btnSaving) : (T && T.btnSave)}
        </button>
      </div>
    </div>
  {/if}
</div>
