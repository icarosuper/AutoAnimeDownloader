<script lang="ts">
  import { createEventDispatcher } from "svelte";
  import ConfirmDialog from "./ConfirmDialog.svelte";
  import * as m from "../lib/i18n/messages.js";
  import { locale } from "../lib/stores/locale.js";

  // Serve tanto para exclusão de 1 torrent (usa `name`) quanto em lote (usa `count`).
  export let open = false;
  export let count = 1;
  export let name = "";
  /** "anime" quando a exclusão cobre o grupo inteiro — troca o texto do 2º check. */
  export let scope: "episode" | "anime" = "episode";
  /** Anime avulso: em vez de bloquear episódios, o check deixa de acompanhá-lo. */
  export let standalone = false;

  // Ambos marcados por default: apaga os arquivos e bloqueia o redownload do episódio.
  let deleteFiles = true;
  let blockRedownload = true;

  const dispatch = createEventDispatcher<{
    confirm: { keepData: boolean; block: boolean };
    cancel: void;
  }>();

  // Reseta os checkboxes toda vez que o diálogo abre, para não herdar o estado de uma exclusão anterior.
  $: if (open) {
    deleteFiles = true;
    blockRedownload = true;
  }

  function handleConfirm() {
    dispatch("confirm", { keepData: !deleteFiles, block: blockRedownload });
  }

  function handleCancel() {
    dispatch("cancel");
  }

  $: T = $locale && {
    titleSingle: m.downloads_delete_title_single({ name }),
    titleBulk: m.downloads_delete_title_bulk({ count }),
    confirmBtn: m.downloads_delete_confirm_btn(),
    cancelBtn: m.common_cancel(),
    checkboxFiles: m.downloads_delete_checkbox_files(),
    checkboxBlock: m.downloads_delete_checkbox_block(),
    checkboxBlockAnime: m.downloads_delete_checkbox_block_anime(),
    checkboxUntrack: m.downloads_delete_checkbox_untrack(),
    consequenceBlocked: m.downloads_delete_consequence_blocked(),
    consequenceBlockedAnime: m.downloads_delete_consequence_blocked_anime(),
    consequenceUntracked: m.downloads_delete_consequence_untracked(),
    consequenceRedownload: m.downloads_delete_consequence_will_redownload(),
  };

  $: title = count > 1 ? T && T.titleBulk : T && T.titleSingle;
  $: blockLabel =
    T && (scope !== "anime" ? T.checkboxBlock : standalone ? T.checkboxUntrack : T.checkboxBlockAnime);
  $: consequence = !blockRedownload
    ? T && T.consequenceRedownload
    : T && (scope !== "anime" ? T.consequenceBlocked : standalone ? T.consequenceUntracked : T.consequenceBlockedAnime);
</script>

<ConfirmDialog
  bind:open
  title={title || ""}
  confirmLabel={(T && T.confirmBtn) || ""}
  cancelLabel={(T && T.cancelBtn) || ""}
  on:confirm={handleConfirm}
  on:cancel={handleCancel}
>
  <div class="space-y-3.5">
    <label class="flex items-start gap-2.5 cursor-pointer">
      <input type="checkbox" class="checkbox checkbox-sm mt-px" bind:checked={deleteFiles} />
      <span class="text-copy leading-snug text-base-content">{T && T.checkboxFiles}</span>
    </label>
    <label class="flex items-start gap-2.5 cursor-pointer">
      <input type="checkbox" class="checkbox checkbox-sm mt-px" bind:checked={blockRedownload} />
      <span class="text-copy leading-snug text-base-content">{blockLabel}</span>
    </label>
    <p class="!mt-4 text-caption leading-snug {blockRedownload ? 'text-base-content/50' : 'text-warning'}">
      {consequence}
    </p>
  </div>
</ConfirmDialog>
