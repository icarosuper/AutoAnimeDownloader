package main

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"testing"
)

func runCLI(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	outputJSON = false
	verbose = false
	apiEndpoint = "http://localhost:8091"

	oldStdout, oldStderr := os.Stdout, os.Stderr
	outR, outW, pipeErr := os.Pipe()
	if pipeErr != nil {
		t.Fatal(pipeErr)
	}
	errR, errW, pipeErr := os.Pipe()
	if pipeErr != nil {
		t.Fatal(pipeErr)
	}
	os.Stdout, os.Stderr = outW, errW

	runErr := run(append([]string{"autoanimedownloader"}, args...))

	_ = outW.Close()
	_ = errW.Close()
	os.Stdout, os.Stderr = oldStdout, oldStderr

	outBytes, _ := io.ReadAll(outR)
	errBytes, _ := io.ReadAll(errR)
	return string(outBytes), string(errBytes), runErr
}

func cliCode(err error) (code string, exit int) {
	var ce *cliError
	if errors.As(err, &ce) {
		return ce.code, ce.exitCode
	}
	return "", 1
}

func decodeJSON(t *testing.T, raw string) any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatalf("json: %v\n%s", err, raw)
	}
	return v
}
