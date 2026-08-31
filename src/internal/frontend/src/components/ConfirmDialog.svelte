<script lang="ts">
  // Diálogo de confirmação. Construído sobre o `ui/Modal` do projeto — que já resolve overlay,
  // foco preso, Esc e devolução do foco — em vez do `<dialog>` + `.modal-*` do daisyUI que
  // morava aqui. A API pública (bind:open, title, message, confirmLabel, cancelLabel, slot,
  // on:confirm/on:cancel) é a mesma de antes, então os consumidores não mudam.
  import { createEventDispatcher } from "svelte";
  import Modal from "./ui/Modal.svelte";
  import Button from "./ui/Button.svelte";

  export let open = false;
  export let title = "Are you sure?";
  export let message = "";
  export let confirmLabel = "Confirm";
  export let cancelLabel = "Cancel";

  const dispatch = createEventDispatcher<{ confirm: void; cancel: void }>();

  // id único por instância: a tela monta mais de um ConfirmDialog ao mesmo tempo
  // (AnimeDetail tem dois), e um id repetido faria o aria-labelledby apontar para o errado.
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
  <!-- Slot opcional para composição: quem precisar de conteúdo extra entre a mensagem e os
       botões (ex.: TorrentDeleteDialog com seus checkboxes) usa isto em vez de duplicar o
       diálogo. O wrapper só existe quando há slot — antes ele era incondicional e dependia de
       colapso de margem para não abrir um vão nos diálogos que passam só `message`. -->
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
