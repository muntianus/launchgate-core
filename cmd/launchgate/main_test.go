package main

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/muntianus/launchgate-core/internal/evaluator"
)

func TestCLIEvaluate(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := RunCLI([]string{
		"evaluate",
		"--p95", "320",
		"--error-rate", "0.002",
		"--rps", "150",
		"--policy", "globlex-b2b",
	}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("expected exit code 0, got %d (stderr: %s)", code, stderr.String())
	}

	var report evaluator.EvaluationReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("failed to decode CLI JSON output: %v", err)
	}
	if report.Verdict != evaluator.VerdictGo {
		t.Fatalf("expected GO verdict, got %s", report.Verdict)
	}
	if report.Policy != "globlex-b2b" {
		t.Fatalf("expected policy globlex-b2b, got %s", report.Policy)
	}
}

func TestCLIEvaluateFailOnNoGo(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := RunCLI([]string{
		"evaluate",
		"--p95", "650",
		"--error-rate", "0.05",
		"--rps", "40",
		"--policy", "globlex-b2b",
		"--fail-on-nogo",
	}, &stdout, &stderr)

	if code != 1 {
		t.Fatalf("expected exit code 1 when --fail-on-nogo and NO_GO verdict, got %d", code)
	}
}
