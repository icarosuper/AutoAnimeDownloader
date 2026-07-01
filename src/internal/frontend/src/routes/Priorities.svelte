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
  import { toast } from "../lib/stores/toast.js";

  const LISTS: { key: keyof Priorities; label: string }[] = [
    { key: "criteria_order", label: "Ordem dos critérios" },
    { key: "fansubs", label: "Fansubs" },
    { key: "resolutions", label: "Resoluções" },
    { key: "sources", label: "Source" },
    { key: "codecs", label: "Codec" },
    { key: "audio", label: "Áudio" },
    { key: "ignore_list", label: "Lista de bloqueio" },
  ];

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
      toast.error(err instanceof Error ? err.message : "Falha ao carregar prioridades");
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
      toast.success("Prioridades salvas");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Falha ao salvar prioridades");
    } finally {
      saving = false;
    }
  }

  onMount(load);
</script>

<div class="space-y-6">
  <div>
    <h1 class="text-2xl font-semibold text-base-content">Prioridades dos torrents</h1>
    <p class="text-sm text-base-content/50 mt-0.5">
      Controla a ordem de preferência usada para ranquear e filtrar releases do Nyaa.
    </p>
  </div>

  {#if loading}
    <Loading message="Carregando..." />
  {:else if config}
    <div class="space-y-4">
      {#each LISTS as { key, label } (key)}
        <div class="card bg-base-200 border border-base-300">
          <div class="card-body p-5 gap-3">
            <div class="flex items-center justify-between">
              <h2 class="text-sm font-semibold text-base-content/60 uppercase tracking-wider">{label}</h2>
              <button
                type="button"
                on:click={() => resetList(key)}
                class="text-xs font-medium text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-200 transition-colors"
              >
                Resetar esta lista
              </button>
            </div>

            {#if config.priorities[key].length > 0}
              <ol class="flex flex-col gap-1.5">
                {#each config.priorities[key] as item, i (item)}
                  <li class="flex items-center gap-2 bg-base-100 rounded-md px-3 py-1.5">
                    <span class="text-xs text-base-content/40 w-5 text-right">{i + 1}</span>
                    <span class="flex-1 text-sm text-base-content">{item}</span>
                    <button
                      type="button"
                      on:click={() => move(key, i, -1)}
                      disabled={i === 0}
                      aria-label="Mover {item} para cima"
                      class="text-gray-400 hover:text-gray-700 dark:hover:text-gray-200 disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
                    >
                      ↑
                    </button>
                    <button
                      type="button"
                      on:click={() => move(key, i, 1)}
                      disabled={i === config.priorities[key].length - 1}
                      aria-label="Mover {item} para baixo"
                      class="text-gray-400 hover:text-gray-700 dark:hover:text-gray-200 disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
                    >
                      ↓
                    </button>
                    <button
                      type="button"
                      on:click={() => remove(key, i)}
                      aria-label="Remover {item}"
                      class="text-gray-400 hover:text-red-500 transition-colors"
                    >
                      <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"/>
                      </svg>
                    </button>
                  </li>
                {/each}
              </ol>
            {/if}

            <div class="flex gap-2">
              <input
                type="text"
                bind:value={newItem[key]}
                placeholder="Adicionar item"
                class="flex-1 block rounded-md border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 text-gray-900 dark:text-white shadow-sm focus:border-blue-500 focus:ring-blue-500 sm:text-sm px-3 py-2"
                on:keydown={(e) => { if (e.key === 'Enter') { e.preventDefault(); add(key); } }}
              />
              <button
                type="button"
                on:click={() => add(key)}
                class="inline-flex items-center px-3 py-2 rounded-md border border-gray-300 dark:border-gray-600 text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700 text-sm font-medium transition-colors"
              >
                +
              </button>
            </div>
          </div>
        </div>
      {/each}
    </div>

    <div class="flex justify-end gap-3 pt-2">
      <button
        type="button"
        on:click={resetAll}
        disabled={saving}
        class="inline-flex items-center gap-2 px-4 py-2 rounded-lg border border-gray-300 dark:border-gray-600 text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700 font-medium text-sm transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
      >
        Resetar tudo
      </button>
      <button
        type="button"
        on:click={save}
        disabled={saving}
        class="inline-flex items-center gap-2 px-4 py-2 rounded-lg bg-blue-600 hover:bg-blue-700 text-white font-medium text-sm transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
      >
        {saving ? "Salvando..." : "Salvar"}
      </button>
    </div>
  {/if}
</div>
