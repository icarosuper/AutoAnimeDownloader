<script lang="ts">
  import { createEventDispatcher } from "svelte";
  import ConfirmDialog from "./ConfirmDialog.svelte";
  import * as m from "../lib/i18n/messages.js";
  import { locale } from "../lib/stores/locale.js";

  // Serve tanto para exclusão de 1 torrent (usa `name`) quanto em lote (usa `count`).
  export let open = false;
  export let count = 1;
  export let name = "";

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
    consequenceBlocked: m.downloads_delete_consequence_blocked(),
    consequenceRedownload: m.downloads_delete_consequence_will_redownload(),
  };

  $: title = count > 1 ? T && T.titleBulk : T && T.titleSingle;
  $: consequence = blockRedownload ? T && T.consequenceBlocked : T && T.consequenceRedownload;
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
      <span class="text-copy leading-snug text-base-content">{T && T.checkboxBlock}</span>
    </label>
    <p class="!mt-4 text-caption leading-snug {blockRedownload ? 'text-base-content/50' : 'text-warning'}">
      {consequence}
    </p>
  </div>
</ConfirmDialog>
