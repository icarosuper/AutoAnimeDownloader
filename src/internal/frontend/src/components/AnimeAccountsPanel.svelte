<script lang="ts">
  // "On lists" panel of the anime detail screen: every configured account that has the anime on
  // its list, in any status, with its own status, progress and score — the AniList "Following"
  // box, for the accounts this install tracks. The status comes from each account's own entry; the
  // response's top-level `status` belongs to one arbitrary account (decisions.md #43) and is not
  // used here.
  import { FaceNeutral, FaceSlightlyFrowning, FaceSlightlySmiling } from "@lucide/svelte";
  import type { AnimeListAccount } from "../lib/api/client.js";
  import { formatScore, type FormatLocale } from "../lib/domain/format.js";
  import * as m from "../lib/i18n/messages.js";
  import { locale } from "../lib/stores/locale.js";

  export let accounts: AnimeListAccount[] = [];
  /** Translates a list status (CURRENT, PLANNING, ...); the screen already owns that table. */
  export let statusLabel: (status: string) => string;
  /** The anime's episode count, 0 when unknown (shown as "?"). */
  export let totalEpisodes = 0;

  $: fmtLocale = ($locale ?? "en") as FormatLocale;

  const faces = { good: FaceSlightlySmiling, ok: FaceNeutral, bad: FaceSlightlyFrowning };

  function faceLabel(mood: "good" | "ok" | "bad"): string {
    if (mood === "good") return m.detail_lists_score_good();
    if (mood === "ok") return m.detail_lists_score_ok();
    return m.detail_lists_score_bad();
  }

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
        {@const score = formatScore(acc.score, fmtLocale, acc.score_raw, acc.score_format)}
        <li class="flex min-w-0 items-center gap-3 rounded-field bg-control px-3 py-2">
          {#if acc.avatar}
            <img src={acc.avatar} alt="" class="h-9 w-9 shrink-0 rounded-field object-cover" />
          {:else}
            <span
              class="inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-field bg-sunken font-mono text-caption uppercase text-subtle"
              aria-hidden="true"
            >
              {acc.username.slice(0, 1)}
            </span>
          {/if}
          <!-- Two lines: who on top, where they are below. One line held five things and squeezed
               the username to nothing in the 2-column grid at `sm`. -->
          <div class="min-w-0 flex-1">
            <div class="flex min-w-0 items-center gap-1.5">
              <a
                href={profileUrl(acc)}
                target="_blank"
                rel="noopener noreferrer"
                class="min-w-0 truncate text-copy font-semibold text-heading hover:underline"
                aria-label={$locale && m.detail_lists_profile({ username: acc.username })}
              >
                {acc.username}
              </a>
              {#if acc.source === "mal"}
                <span class="shrink-0 rounded-pill border border-default px-1.5 font-mono text-[11px] text-subtle">MAL</span>
              {/if}
            </div>
            <p class="truncate text-caption text-body">
              {$locale && statusLabel(acc.status)}
              {#if acc.progress > 0}
                <span class="text-subtle">
                  · {$locale && m.detail_lists_progress({ progress: acc.progress, total: totalEpisodes || "?" })}
                </span>
              {/if}
            </p>
          </div>
          <!-- Always rendered, empty when unscored: a fixed score column keeps the rows aligned,
               like AniList's own box. The score is in the account's own format (85/100, ★ 4/5, a
               face), falling back to x/10 when the format is unknown. -->
          <span class="inline-flex w-14 shrink-0 justify-end font-mono text-caption text-subtle">
            {#if score?.kind === "text"}
              {score.text}
            {:else if score?.kind === "smiley"}
              <span role="img" aria-label={$locale && faceLabel(score.mood)} title={$locale && faceLabel(score.mood)}>
                <svelte:component this={faces[score.mood]} size={18} strokeWidth={2} />
              </span>
            {/if}
          </span>
        </li>
      {/each}
    </ul>
  </section>
{/if}
