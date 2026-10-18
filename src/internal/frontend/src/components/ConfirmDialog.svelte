<script lang="ts">
  // Confirmation dialog. Built on the project's `ui/Modal` — which already handles overlay,
  // focus trap, Esc and focus return — instead of daisyUI's `<dialog>` + `.modal-*` that
  // used to live here. The public API (bind:open, title, message, confirmLabel, cancelLabel,
  // slot, on:confirm/on:cancel) is the same as before, so consumers don't change.
  import { createEventDispatcher } from "svelte";
  import Modal from "./ui/Modal.svelte";
  import Button from "./ui/Button.svelte";

  export let open = false;
  export let title = "Are you sure?";
  export let message = "";
  export let confirmLabel = "Confirm";
  export let cancelLabel = "Cancel";

  const dispatch = createEventDispatcher<{ confirm: void; cancel: void }>();

  // unique id per instance: the screen mounts more than one ConfirmDialog at once
  // (AnimeDetail has two), and a repeated id would make aria-labelledby point at the wrong one.
  const titleId = `confirm-title-${Math.random().toString(36).slice(2, 9)}`;

  function confirm() {
    open = false;
    dispatch("confirm");
  }

  function cancel() {
    open = false;
    dispatch("cancel");
  }
</script>

<Modal {open} labelledBy={titleId} on:close={cancel}>
  <h3 id={titleId} class="text-card-title leading-snug text-heading">{title}</h3>
  {#if message}
    <p class="mt-3 text-copy font-normal text-body">{message}</p>
  {/if}
  <!-- Optional slot for composition: anyone who needs extra content between the message and
       the buttons (e.g. TorrentDeleteDialog with its checkboxes) uses this instead of
       duplicating the dialog. The wrapper only exists when there is a slot — before it was
       unconditional and relied on margin collapse to avoid opening a gap in dialogs that
       only pass `message`. -->
  {#if $$slots.default}
    <div class="mt-5">
      <slot />
    </div>
  {/if}
  <div class="mt-5 flex justify-end gap-2">
    <Button variant="ghost" on:click={cancel}>{cancelLabel}</Button>
    <Button variant="warn" on:click={confirm}>{confirmLabel}</Button>
  </div>
</Modal>
