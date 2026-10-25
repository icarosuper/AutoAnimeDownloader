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

  // scope: source applies to movies and FINISHED series; audio stays movies-only.
  const LISTS: { key: keyof Priorities; label: () => string; scope?: () => string }[] = [
    { key: "criteria_order", label: m.priorities_list_criteria_order },
    { key: "fansubs", label: m.priorities_list_fansubs },
    { key: "resolutions", label: m.priorities_list_resolutions },
    { key: "sources", label: m.priorities_list_sources, scope: m.priorities_scope_movies_and_finished },
    { key: "codecs", label: m.priorities_list_codecs },
    { key: "audio", label: m.priorities_list_audio, scope: m.priorities_scope_movies_only },
    { key: "ignore_list", label: m.priorities_list_ignore_list },
  ];

  // A fixed line instead of a tooltip: tooltips don't exist on touch, and this screen is used on
  // the phone.
  const NOTES: Partial<Record<keyof Priorities, () => string>> = {
    criteria_order: m.priorities_note_criteria_order,
    codecs: m.priorities_note_codecs,
  };

  $: T = $locale && {
    title: m.priorities_title(),
    subtitle: m.priorities_subtitle(),
    loading: m.priorities_loading(),
    resetList: m.priorities_reset_list(),
    addPlaceholder: m.priorities_add_placeholder(),
    resetAll: m.priorities_reset_all(),
    save: m.priorities_save(),
    saving: m.priorities_saving(),
    lists: LISTS.map((l) => ({ ...l, label: l.label(), scope: l.scope && l.scope() })),
    notes: Object.fromEntries(Object.entries(NOTES).map(([k, f]) => [k, f()])) as Partial<Record<keyof Priorities, string>>,
    presets: Object.fromEntries(
      Object.entries(PRESETS).map(([k, list]) => [k, (list ?? []).map((p) => ({ ...p, label: p.label(), desc: p.desc() }))]),
    ) as Partial<Record<keyof Priorities, { key: string; label: string; desc: string; first: string[] }[]>>,
  };

  let config: Config | null = null;
  let defaults: Priorities | null = null;
  let loading = true;
  let saving = false;
  let newItem: Record<string, string> = {};

  async function load() {
    try {
      loading = true;
      const [c, d] = await Promise.all([getConfig(), getPriorityDefaults()]);
      config = c;
      defaults = d;
    } catch (err) {
      toast.error(err instanceof Error ? err.message : m.priorities_toast_load_error());
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
   * The list's rows: what is saved (checked, in order) followed by the canonical tokens left out
   * (unchecked, at the end). A token missing from the list is already treated as the worst by the
   * backend (`priorityIndex` returns `len(list)`), so "unchecked at the end" is exactly what
   * unchecking means — and nothing disappears from the screen, which was the problem with the X.
   *
   * `custom` = not in the default: what the user typed, plus the inert leftovers of an old
   * config.json (`x265`, `4k`). Only those can be removed for good.
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
      toast.success(m.priorities_toast_saved());
    } catch (err) {
      toast.error(err instanceof Error ? err.message : m.priorities_toast_save_error());
    } finally {
      saving = false;
    }
  }

  onMount(load);
</script>

<div class="space-y-6">
  <div>
    <h1 class="text-2xl font-semibold text-heading">{T && T.title}</h1>
    <p class="text-sm text-subtle mt-0.5">{T && T.subtitle}</p>
  </div>

  {#if loading}
    <Loading message={(T && T.loading) || ""} />
  {:else if config && T}
    <div class="space-y-4">
      {#each T.lists as { key, label, scope } (key)}
        {@const items = rows(key, config.priorities[key], defaults)}
        <div class="flex flex-col rounded-card border border-default bg-sunken">
          <div class="flex flex-col gap-3 p-5">
            <div class="flex items-center justify-between">
              <h2 class="text-sm font-semibold text-subtle uppercase tracking-wider">
                {label}
                {#if scope}<span class="normal-case tracking-normal font-normal text-subtle">({scope})</span>{/if}
              </h2>
              <button
                type="button"
                on:click={() => resetList(key)}
                class="text-xs font-medium text-subtle hover:text-heading transition-colors"
              >
                {T.resetList}
              </button>
            </div>

            {#if T.presets[key]}
              <div class="flex flex-wrap gap-2">
                {#each T.presets[key] ?? [] as p (p.key)}
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
                      label={$locale && (row.custom ? m.priorities_item_custom({ item: row.item }) : m.priorities_item_use({ item: row.item }))}
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
                        aria-label={$locale && m.priorities_move_up({ item: row.item })}
                        class="text-subtle hover:text-heading disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
                      >
                        ↑
                      </button>
                      <button
                        type="button"
                        on:click={() => move(key, i, 1)}
                        disabled={i === config.priorities[key].length - 1}
                        aria-label={$locale && m.priorities_move_down({ item: row.item })}
                        class="text-subtle hover:text-heading disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
                      >
                        ↓
                      </button>
                    {/if}
                    {#if row.custom}
                      <button
                        type="button"
                        on:click={() => remove(key, i)}
                        aria-label={$locale && m.priorities_remove({ item: row.item })}
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

            {#if T.notes[key]}
              <p class="text-xs text-subtle">{T.notes[key]}</p>
            {/if}

            <!-- criteria_order is a closed set (sortByCriteria silently skips a criterion it doesn't
                 know) and every criterion already shows above, checked or not: there's nothing to
                 add, and free text there would only produce inert config. -->
            {#if key !== "criteria_order"}
            <div class="flex gap-2">
              <input
                type="text"
                bind:value={newItem[key]}
                placeholder={T.addPlaceholder}
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
        {T.resetAll}
      </button>
      <button
        type="button"
        on:click={save}
        disabled={saving}
        class="inline-flex items-center gap-2 px-4 py-2 rounded-lg bg-accent text-on-accent hover:opacity-90 font-medium text-sm transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
      >
        {saving ? T.saving : T.save}
      </button>
    </div>
  {/if}
</div>
