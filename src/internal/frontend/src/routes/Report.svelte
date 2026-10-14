<script lang="ts">
  // Report — the bug report screen. The daemon builds a redacted bundle locally; this screen shows
  // exactly what's in it and hands it over as a file. Nothing leaves the machine unless the user
  // attaches it (decisions.md #102). Opened from the MoreMenu, the tray, the Report link of an
  // unexpected error toast (?error=&context=&anime=) and a pending crash (?crash=1).
  import { onMount } from "svelte";
  import { querystring } from "svelte-spa-router";
  import { AlertTriangle, Download, ExternalLink } from "@lucide/svelte";
  import { dismissPendingCrash, getDiagnostics, getStatus, type DiagnosticsReport } from "../lib/api/client.js";
  import Button from "../components/ui/Button.svelte";
  import Checkbox from "../components/ui/Checkbox.svelte";
  import Loading from "../components/Loading.svelte";
  import { downloadText, issueUrl, reportFileContent, reportFileName, type ReportContext } from "../lib/report.js";
  import * as m from "../lib/i18n/messages.js";
  import { locale } from "../lib/stores/locale.js";

  $: T = $locale && {
    title: m.report_title(),
    subtitle: m.report_subtitle(),
    whatLabel: m.report_what_happened_label(),
    whatPlaceholder: m.report_what_happened_placeholder(),
    contextLabel: m.report_context_label(),
    titlesLabel: m.report_titles_label(),
    titlesHint: m.report_titles_hint(),
    previewLabel: m.report_preview_label(),
    loading: m.report_loading(),
    loadError: m.report_load_error(),
    publicNotice: m.report_public_notice(),
    openIssue: m.report_btn_open_issue(),
    download: m.report_btn_download(),
    dismiss: m.report_btn_dismiss(),
    textRequired: m.report_text_required(),
  };

  const params = new URLSearchParams($querystring ?? "");
  const ctx: ReportContext = {
    error: params.get("error") ?? undefined,
    context: params.get("context") ?? undefined,
    anime: params.get("anime") ?? undefined,
  };
  let isCrash = params.get("crash") === "1";

  let whatHappened = "";
  let includeTitles = true;
  let report: DiagnosticsReport | null = null;
  let loadFailed = false;
  let crashAt: Date | null = null;
  let downloadedFile = "";
  let showTextRequired = false;
  // Ignores the answer of a request a newer checkbox toggle has already replaced.
  let requestSeq = 0;

  async function loadReport() {
    const seq = ++requestSeq;
    loadFailed = false;
    try {
      const result = await getDiagnostics(includeTitles, ctx);
      if (seq === requestSeq) report = result;
    } catch {
      if (seq === requestSeq) loadFailed = true;
    }
  }

  // The checkbox regenerates the bundle: the preview must be exactly what the file carries.
  let loadedTitles = includeTitles;
  $: if (includeTitles !== loadedTitles) {
    loadedTitles = includeTitles;
    report = null;
    loadReport();
  }

  function currentFile(): { name: string; text: string } | null {
    if (!report) return null;
    if (!whatHappened.trim()) {
      showTextRequired = true;
      return null;
    }
    const version = report.environment.split("\n")[0].replace("Version: ", "");
    return { name: reportFileName(version, new Date()), text: reportFileContent(whatHappened, report.bundle) };
  }

  function handled() {
    if (!isCrash) return;
    isCrash = false;
    dismissPendingCrash().catch(() => {});
  }

  // Synchronous on purpose, from the bundle already loaded for the preview: an await between the
  // click and window.open and popup blockers swallow the tab.
  function openIssue() {
    const file = currentFile();
    if (!file || !report) return;
    downloadText(file.name, file.text);
    window.open(issueUrl(whatHappened, report.environment), "_blank", "noopener");
    downloadedFile = file.name;
    handled();
  }

  function downloadOnly() {
    const file = currentFile();
    if (!file) return;
    downloadText(file.name, file.text);
    handled();
  }

  $: if (whatHappened.trim()) showTextRequired = false;

  $: crashText =
    $locale && crashAt
      ? m.report_crash_context({ date: crashAt.toLocaleString($locale, { dateStyle: "short", timeStyle: "short" }) })
      : "";

  onMount(() => {
    loadReport();
    if (isCrash) {
      getStatus({ silent: true })
        .then((s) => {
          if (s.pending_crash_at) crashAt = new Date(s.pending_crash_at);
          else isCrash = false; // already handled in another tab
        })
        .catch(() => {});
    }
  });
</script>

<div class="flex flex-col gap-4">
  <div>
    <h1 class="text-screen-title text-heading">{T && T.title}</h1>
    <p class="mt-0.5 text-caption text-subtle">{T && T.subtitle}</p>
  </div>

  <div class="flex flex-col gap-4 rounded-card border border-default bg-card p-4">
    <div class="flex flex-col gap-1.5">
      <label for="report-what-happened" class="text-copy font-semibold text-heading">{T && T.whatLabel}</label>
      <textarea
        id="report-what-happened"
        bind:value={whatHappened}
        rows="5"
        required
        placeholder={(T && T.whatPlaceholder) || ""}
        class="rounded-field border border-default bg-control px-3 py-2 text-copy text-heading outline-none focus:border-accent"
      ></textarea>
      {#if showTextRequired}
        <p class="text-caption text-danger" role="alert">{T && T.textRequired}</p>
      {/if}
    </div>

    {#if ctx.error || ctx.context || crashText}
      <div class="flex flex-col gap-1">
        <span class="text-caption font-semibold text-subtle">{T && T.contextLabel}</span>
        <div class="rounded-field border border-danger-tint/28 bg-danger-tint/12 px-3 py-2 font-mono text-caption text-danger">
          {#if crashText}<div>{crashText}</div>{/if}
          {#if ctx.error}<div>{ctx.error}</div>{/if}
          {#if ctx.context}<div class="opacity-80">{ctx.context}</div>{/if}
        </div>
      </div>
    {/if}

    <div class="flex flex-col gap-1">
      <Checkbox bind:checked={includeTitles} label={(T && T.titlesLabel) || ""} />
      <p class="pl-6 text-caption text-subtle">{T && T.titlesHint}</p>
    </div>

    <div class="flex flex-col gap-1.5">
      <span class="text-caption font-semibold text-subtle">{T && T.previewLabel}</span>
      {#if report}
        <pre
          data-testid="report-preview"
          class="max-h-96 overflow-auto whitespace-pre-wrap break-all rounded-field border border-default bg-sunken p-3 font-mono text-[12px] text-body">{report.bundle}</pre>
      {:else if loadFailed}
        <p class="text-copy text-danger">{T && T.loadError}</p>
      {:else}
        <Loading message={(T && T.loading) || ""} />
      {/if}
    </div>

    <p class="flex items-start gap-2 text-caption text-warn">
      <AlertTriangle class="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
      {T && T.publicNotice}
    </p>

    <div class="flex flex-wrap items-center gap-2">
      <Button variant="solid" disabled={!report} on:click={openIssue}>
        <ExternalLink size={16} strokeWidth={2} />
        {T && T.openIssue}
      </Button>
      <Button disabled={!report} on:click={downloadOnly}>
        <Download size={16} strokeWidth={2} />
        {T && T.download}
      </Button>
      {#if isCrash}
        <Button on:click={handled}>{T && T.dismiss}</Button>
      {/if}
    </div>

    {#if downloadedFile}
      <p class="text-copy font-semibold text-ok" role="status">
        {$locale && m.report_after_open({ file: downloadedFile })}
      </p>
    {/if}
  </div>
</div>
