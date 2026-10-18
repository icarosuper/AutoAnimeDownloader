package main

import (
	"encoding/json"
	"testing"

	"AutoAnimeDownloader/src/internal/diagnostics"
)

func TestReportWritesBundleWithoutDaemon(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	stdout, _, err := runCLI(t, "--json", "report")
	if err != nil {
		t.Fatalf("report: %v", err)
	}
	var report diagnostics.Report
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("json: %v\n%s", err, stdout)
	}
	if report.Bundle == "" {
		t.Error("bundle is empty")
	}
	if report.Environment == "" {
		t.Error("environment is empty")
	}
}

func TestReportCrashAckMissingFileIsSuccess(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if _, _, err := runCLI(t, "report", "crash-ack"); err != nil {
		t.Fatalf("crash-ack: %v", err)
	}
}
