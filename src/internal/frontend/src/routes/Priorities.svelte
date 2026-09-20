<script lang="ts">
  import { onMount } from "svelte";
  import {
    getConfig,
    updateConfig,
    getPriorityDefaults,
    type Config,
    type Priorities,
  } from "../lib/api/client.js";
  import Loading from "../components/Loading.svelte";
  import Checkbox from "../components/ui/Checkbox.svelte";
  import { toast } from "../lib/stores/toast.js";
  import { PRESETS, applyPreset } from "../lib/domain/priorityPresets.js";
  import * as m from "../lib/i18n/messages.js";
  import { locale } from "../lib/stores/locale.js";

  // scope: a lista existe mas não vale para episódio — só SortMovieResults lê source e áudio.
  const LISTS: { key: keyof Priorities; label: string; scope?: string }[] = [
    { key: "criteria_order", label: "priorities_criteria_order" },
    { key: "fansubs", label: "priorities_fansubs" },
    { key: "resolutions", label: "priorities_resolutions" },
    { key: "sources", label: "priorities_sources", scope: "priorities_scope_movies_only" },
    { key: "codecs", label: "priorities_codecs" },
    { key: "audio", label: "priorities_audio", scope: "priorities_scope_movies_only" },
    { key: "ignore_list", label: "priorities_ignore_list" },
  ];

  // Mapa tipado de cada rótulo/nota para a função de mensagem correspondente. Paraglide gera um
  // módulo sem index signature, então `m[key]` não tipa — e montar esses maps à mão é o que
  // permite manter o typecheck sem `any`.
  const LIST_LABELS: Record<keyof Priorities, () => string> = {
    criteria_order: m.priorities_criteria_order,
    fansubs: m.priorities_fansubs,
    resolutions: m.priorities_resolutions,
    sources: m.priorities_sources,
    codecs: m.priorities_codecs,
    audio: m.priorities_audio,
    ignore_list: m.priorities_ignore_list,
  };
  const SCOPE_LABELS: Partial<Record<keyof Priorities, () => string>> = {
    sources: m.priorities_scope_movies_only,
    audio: m.priorities_scope_movies_only,
  };
  const NOTE_TEXTS: Record<string, () => string> = {
    criteria_order: m.priorities_note_criteria_order,
    codecs: m.priorities_note_codecs,
  };

  let config: Config | null = null;
  let defaults: Priorities | null = null;
  let loading = true;
  let saving = false;
  let newItem: Record<string, string> = {};

$: T = $locale && {
    title: m.priorities_title(),
    subtitle: m.priorities_subtitle(),
    loading: m.priorities_loading(),
    save: m.priorities_save(),
    saving: m.priorities_saving(),
    saved: m.priorities_saved(),
    errorSave: m.priorities_error_save(),
    errorLoad: m.priorities_error_load(),
    resetAll: m.priorities_reset_all(),
    addItem: m.priorities_add_item(),
    resetList: m.priorities_reset_list(),
    moveUp: m.priorities_move_up,
    moveDown: m.priorities_move_down,
    remove: m.priorities_remove,
    customLabel: m.priorities_custom_label,
    listLabels: Object.fromEntries(LISTS.map((l) => [l.key, LIST_LABELS[l.key]()])) as Record<keyof Priorities, string>,
    scopeLabels: Object.fromEntries(LISTS.filter((l) => SCOPE_LABELS[l.key]).map((l) => [l.key, SCOPE_LABELS[l.key]!()])) as Record<keyof Priorities, string>,
    noteText: Object.fromEntries(Object.keys(NOTE_TEXTS).map((k) => [k, NOTE_TEXTS[k]()])) as Record<string, string>,
  };

  async function load() {
    try {
      loading = true;
      const [c, d] = await Promise.all([getConfig(), getPriorityDefaults()]);
      config = c;
      defaults = d;
    } catch (err) {
      toast.error(err instanceof Error ? err.message : (T && T.errorLoad));
    } finally {
      loading = false;
    }
  }

  function move(key: keyof Priorities, i: number, dir: -1 | 1) {
    if (!config) return;
    const list = [...config.priorities[key]];
    const j = i + dir;
    if (j < 0 || j >= list.length) return;
    [list[i], list[j]] = [list[j], list[i]];
    config.priorities[key] = list;
    config = config;
  }

  /**
   * As linhas da lista: o que está salvo (marcado, na ordem) seguido dos tokens canônicos
   * que ficaram de fora (desmarcados, no fim). Token ausente da lista já é tratado como o
   * pior pelo backend (`priorityIndex` devolve `len(list)`), então "desmarcado no fim" é
   * exatamente o que desmarcar significa — e nada some da tela, que era o problema do X.
   *
   * `custom` = não está no default: o que o usuário digitou, mais o legado inerte de um
   * config.json antigo (`x265`, `4k`). Só esses podem ser removidos de vez.
   */
  function rows(key: keyof Priorities, list: string[], def: Priorities | null) {
    const canon = def?.[key] ?? [];
    return [
      ...list.map((item) => ({ item, on: true, custom: !canon.includes(item) })),
      ...canon.filter((v) => !list.includes(v)).map((item) => ({ item, on: false, custom: false })),
    ];
  }

  function toggle(key: keyof Priorities, item: string, on: boolean) {
    if (!config) return;
    config.priorities[key] = on
      ? [...config.priorities[key], item]
      : config.priorities[key].filter((v) => v !== item);
    config = config;
  }

  function remove(key: keyof Priorities, i: number) {
    if (!config) return;
    config.priorities[key] = config.priorities[key].filter((_, idx) => idx !== i);
    config = config;
  }

  function add(key: keyof Priorities) {
    if (!config) return;
    const v = (newItem[key] ?? "").trim().toLowerCase();
    if (!v || config.priorities[key].includes(v)) return;
    config.priorities[key] = [...config.priorities[key], v];
    newItem[key] = "";
    config = config;
  }

  function preset(key: keyof Priorities, first: string[]) {
    if (!config) return;
    config.priorities[key] = applyPreset(config.priorities[key], first);
    config = config;
  }

  function resetList(key: keyof Priorities) {
    if (!config || !defaults) return;
    config.priorities[key] = [...defaults[key]];
    config = config;
  }

  function resetAll() {
    if (!config || !defaults) return;
    config.priorities = JSON.parse(JSON.stringify(defaults));
    config = config;
  }

  async function save() {
    if (!config) return;
    try {
      saving = true;
      await updateConfig(config);
      toast.success(T && T.saved);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : (T && T.errorLoad));
    } finally {
      saving = false;
    }
  }

  onMount(load);
</script>

<div class="space-y-6">
  <div>
    <h1 class="text-2xl font-semibold text-heading">{T && T.title}</h1>
    <p class="text-sm text-subtle mt-0.5">
      {T && T.subtitle}
    </p>
  </div>

  {#if loading}
    <Loading message={T && T.loading || ""} />
  {:else if config}
    <div class="space-y-4">
      {#each LISTS as { key, scope } (key)}
        {@const items = rows(key, config.priorities[key], defaults)}
        <div class="flex flex-col rounded-card border border-default bg-sunken">
          <div class="flex flex-col gap-3 p-5">
            <div class="flex items-center justify-between">
              <h2 class="text-sm font-semibold text-subtle uppercase tracking-wider">
                {T && T.listLabels[key]}
                {#if scope}<span class="normal-case tracking-normal font-normal text-subtle">({T && T.scopeLabels[key]})</span>{/if}
              </h2>
              <button
                type="button"
                on:click={() => resetList(key)}
                class="text-xs font-medium text-subtle hover:text-heading transition-colors"
              >
                {T && T.resetList}
              </button>
            </div>

            {#if PRESETS[key]}
              <div class="flex flex-wrap gap-2">
                {#each PRESETS[key] ?? [] as p (p.key)}
                  <button
                    type="button"
                    on:click={() => preset(key, p.first)}
                    title={p.desc}
                    class="px-3 py-1.5 rounded-md border border-default text-body hover:bg-control text-xs font-medium transition-colors"
                  >
                    {p.label}
                  </button>
                {/each}
              </div>
            {/if}

            {#if items.length > 0}
              <ol class="flex flex-col gap-1.5">
                {#each items as row, i (row.item)}
                  <li class="flex items-center gap-2 bg-card rounded-md px-3 py-1.5 {row.on ? '' : 'opacity-50'}">
                    <Checkbox
                      checked={row.on}
                      disabled={row.custom}
                      label={row.custom ? (T && T.customLabel({ item: row.item }) || `${row.item}`) : `Usar ${row.item}`}
                      labelHidden
                      on:change={() => toggle(key, row.item, !row.on)}
                    />
                    <span class="text-xs text-subtle w-5 text-right">{row.on ? i + 1 : ""}</span>
                    <span class="flex-1 text-sm text-heading">{row.item}</span>
                    {#if row.on}
                      <button
                        type="button"
                        on:click={() => move(key, i, -1)}
                        disabled={i === 0}
                        aria-label={T && T.moveUp({ item: row.item })}
                        class="text-subtle hover:text-heading disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
                      >
                        ↑
                      </button>
                      <button
                        type="button"
                        on:click={() => move(key, i, 1)}
                        disabled={i === config.priorities[key].length - 1}
                        aria-label={T && T.moveDown({ item: row.item })}
                        class="text-subtle hover:text-heading disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
                      >
                        ↓
                      </button>
                    {/if}
                    {#if row.custom}
                      <button
                        type="button"
                        on:click={() => remove(key, i)}
                        aria-label={T && T.remove({ item: row.item })}
                        class="text-subtle hover:text-danger transition-colors"
                      >
                        <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"/>
                        </svg>
                      </button>
                    {/if}
                  </li>
                {/each}
              </ol>
            {/if}

            {#if T && T.noteText[key]}
              <p class="text-xs text-subtle">{T.noteText[key]}</p>
            {/if}

            <!-- criteria_order é conjunto fechado (sortByCriteria pula em silêncio o critério que
                 não conhece) e todo critério já aparece acima, marcado ou desmarcado: não há o que
                 adicionar, e texto livre ali só produziria config inerte. -->
            {#if key !== "criteria_order"}
            <div class="flex gap-2">
              <input
                type="text"
                bind:value={newItem[key]}
                placeholder={T && T.addItem}
                class="flex-1 block rounded-md border-default bg-control text-heading shadow-sm focus:border-accent sm:text-sm px-3 py-2"
                on:keydown={(e) => { if (e.key === 'Enter') { e.preventDefault(); add(key); } }}
              />
              <button
                type="button"
                on:click={() => add(key)}
                class="inline-flex items-center px-3 py-2 rounded-md border border-default text-body hover:bg-control text-sm font-medium transition-colors"
              >
                +
              </button>
            </div>
            {/if}
          </div>
        </div>
      {/each}
    </div>

    <div class="flex justify-end gap-3 pt-2">
      <button
        type="button"
        on:click={resetAll}
        disabled={saving}
        class="inline-flex items-center gap-2 px-4 py-2 rounded-lg border border-default text-body hover:bg-control font-medium text-sm transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
      >
        {T && T.resetAll}
      </button>
      <button
        type="button"
        on:click={save}
        disabled={saving}
        class="inline-flex items-center gap-2 px-4 py-2 rounded-lg bg-accent text-on-accent hover:opacity-90 font-medium text-sm transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
      >
        {saving ? (T && T.saving) : (T && T.save)}
      </button>
    </div>
  {/if}
</div>
