<script lang="ts">
  // "On lists" panel of the anime detail screen: every configured account that has the anime on
  // its list, in any status, with its own status and score — the AniList "Following" box, for the
  // accounts this install tracks. The status comes from each account's own entry; the response's
  // top-level `status` belongs to one arbitrary account (decisions.md #43) and is not used here.
  import type { AnimeListAccount } from "../lib/api/client.js";
  import { formatScore, type FormatLocale } from "../lib/domain/format.js";
  import * as m from "../lib/i18n/messages.js";
  import { locale } from "../lib/stores/locale.js";

  export let accounts: AnimeListAccount[] = [];
  /** Translates a list status (CURRENT, PLANNING, ...); the screen already owns that table. */
  export let statusLabel: (status: string) => string;

  $: fmtLocale = ($locale ?? "en") as FormatLocale;

  function profileUrl(acc: AnimeListAccount): string {
    const user = encodeURIComponent(acc.username);
    return acc.source === "mal"
      ? `https://myanimelist.net/animelist/${user}`
      : `https://anilist.co/user/${user}/animelist`;
  }
</script>

{#if accounts.length > 0}
  <section class="rounded-card border border-default bg-card p-4" aria-labelledby="anime-lists-title">
    <h2 id="anime-lists-title" class="text-card-title text-heading">{$locale && m.detail_lists_title()}</h2>
    <ul class="mt-3 grid grid-cols-1 gap-2 sm:grid-cols-2 xl:grid-cols-3">
      {#each accounts as acc (acc.source + ":" + acc.username)}
        <li class="flex min-w-0 items-center gap-3 rounded-field bg-control px-3 py-2">
          {#if acc.avatar}
            <img src={acc.avatar} alt="" class="h-8 w-8 shrink-0 rounded-field object-cover" />
          {:else}
            <span
              class="inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-field bg-sunken font-mono text-caption uppercase text-subtle"
              aria-hidden="true"
            >
              {acc.username.slice(0, 1)}
            </span>
          {/if}
          <a
            href={profileUrl(acc)}
            target="_blank"
            rel="noopener noreferrer"
            class="min-w-0 flex-1 truncate text-copy font-semibold text-heading hover:underline"
            aria-label={$locale && m.detail_lists_profile({ username: acc.username })}
          >
            {acc.username}
          </a>
          {#if acc.source === "mal"}
            <span class="shrink-0 rounded-pill border border-default px-1.5 font-mono text-[11px] text-subtle">MAL</span>
          {/if}
          <span class="shrink-0 text-copy text-body">{$locale && statusLabel(acc.status)}</span>
          <!-- Always rendered, empty when unscored: a fixed score column keeps every status
               aligned, like AniList's own box. -->
          <span class="w-12 shrink-0 text-right font-mono text-caption text-subtle">
            {formatScore(acc.score, fmtLocale)}
          </span>
        </li>
      {/each}
    </ul>
  </section>
{/if}
