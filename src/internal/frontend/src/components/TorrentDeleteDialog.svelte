<script lang="ts">
  import { createEventDispatcher } from "svelte";
  import ConfirmDialog from "./ConfirmDialog.svelte";
  import Checkbox from "./ui/Checkbox.svelte";
  import * as m from "../lib/i18n/messages.js";
  import { locale } from "../lib/stores/locale.js";

  // Serves both single-torrent deletion (uses `name`) and bulk (uses `count`).
  export let open = false;
  export let count = 1;
  export let name = "";
  /** "anime" when the deletion covers the whole group — swaps the 2nd check's text. */
  export let scope: "episode" | "anime" = "episode";
  /** Standalone anime: instead of blocking episodes, the check stops tracking it. */
  export let standalone = false;

  // Both checked by default: deletes the files and blocks redownload of the episode.
  let deleteFiles = true;
  let blockRedownload = true;

  const dispatch = createEventDispatcher<{
    confirm: { keepData: boolean; block: boolean };
    cancel: void;
  }>();

  // Reset the checkboxes every time the dialog opens, so it doesn't inherit state from a previous deletion.
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
    <Checkbox bind:checked={deleteFiles} label={(T && T.checkboxFiles) || ""} />
    <Checkbox bind:checked={blockRedownload} label={blockLabel} />
    <p class="!mt-4 text-caption leading-snug {blockRedownload ? 'text-subtle' : 'text-warn'}">
      {consequence}
    </p>
  </div>
</ConfirmDialog>
