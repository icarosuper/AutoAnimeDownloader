<script lang="ts">
  import { createEventDispatcher } from "svelte";

  export let open = false;
  export let title = "Are you sure?";
  export let message = "";
  export let confirmLabel = "Confirm";
  export let cancelLabel = "Cancel";

  const dispatch = createEventDispatcher<{ confirm: void; cancel: void }>();

  function confirm() {
    open = false;
    dispatch("confirm");
  }

  function cancel() {
    open = false;
    dispatch("cancel");
  }
</script>

<!-- O atributo nativo `open` não é usado pelo CSS (daisyUI mostra/esconde via .modal-open), mas
     sem ele o <dialog> fica fora da árvore de acessibilidade (spec HTML: dialog fechado não
     expõe conteúdo a leitores de tela/testes de role) mesmo com a classe aplicando o visual. -->
<dialog class="modal" class:modal-open={open} {open}>
  <div class="modal-box">
    <h3 class="text-lg font-semibold text-base-content">{title}</h3>
    {#if message}
      <p class="py-4 text-sm text-base-content/70">{message}</p>
    {/if}
    <!-- Slot opcional para composição: quem precisar de conteúdo extra entre a mensagem e os
         botões (ex.: TorrentDeleteDialog com seus checkboxes) usa isto em vez de duplicar o modal. -->
    <slot />
    <div class="modal-action">
      <button class="btn btn-sm btn-ghost" on:click={cancel}>{cancelLabel}</button>
      <button class="btn btn-sm btn-error" on:click={confirm}>{confirmLabel}</button>
    </div>
  </div>
  <div class="modal-backdrop" on:click={cancel}></div>
</dialog>
