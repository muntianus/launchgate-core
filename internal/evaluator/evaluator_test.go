package evaluator

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/muntianus/launchgate-core/internal/policy"
)

func floatPtr(v float64) *float64 {
	return &v
}

func TestEvaluateVerdictGo(t *testing.T) {
	pol, ok := policy.GetPreset("globlex-b2b")
	if !ok {
		t.Fatalf("expected preset globlex-b2b")
	}

	report := Evaluate(EvaluationInput{
		PolicyName: "globlex-b2b",
		ReleaseSHA: "abc1234",
		Metrics: &K6SummaryMetrics{
			HTTPReqDuration: &K6DurationMetrics{
				Avg: 95.0,
				P95: 240.0,
				P99: 480.0,
			},
			HTTPReqFailed: &K6RateMetrics{
				Rate: 0.001,
			},
			HTTPReqs: &K6CounterMetrics{
				Rate: 185.0,
			},
		},
		FinOps: &FinOpsTelemetry{
			HourlyCostUSD: 32.5,
		},
		Chaos: &ChaosTelemetry{
			Injected:   true,
			Passed:     true,
			RecoveryMs: 1200.0,
			DropRate:   0.004,
		},
		Baseline: &BaselineMetrics{
			P95Ms:     230.0,
			ErrorRate: 0.001,
			RPS:       180.0,
		},
	}, pol)

	if report.Verdict != VerdictGo {
		t.Fatalf("expected GO verdict, got %s (violations: %v)", report.Verdict, report.Violations)
	}
	if report.Score != 100 {
		t.Fatalf("expected score 100, got %d", report.Score)
	}
	if len(report.Violations) != 0 {
		t.Fatalf("expected 0 violations, got %v", report.Violations)
	}
}

func TestEvaluateVerdictConditionalGo(t *testing.T) {
	pol, ok := policy.GetPreset("globlex-b2b")
	if !ok {
		t.Fatalf("expected preset globlex-b2b")
	}

	// p95 = 380ms is above 350ms target, but below 350 * 1.15 = 402.5ms hard limit -> CONDITIONAL_GO
	report := Evaluate(EvaluationInput{
		PolicyName: "globlex-b2b",
		P95Ms:      380.0,
		P99Ms:      650.0,
		ErrorRate:  floatPtr(0.002),
		RPS:        140.0,
	}, pol)

	if report.Verdict != VerdictConditionalGo {
		t.Fatalf("expected CONDITIONAL_GO verdict, got %s", report.Verdict)
	}
	if report.Score >= 100 || report.Score <= 50 {
		t.Fatalf("expected moderate score deduction for CONDITIONAL_GO, got %d", report.Score)
	}
	if len(report.Violations) == 0 {
		t.Fatalf("expected violation detail for conditional p95 breach")
	}
}

func TestEvaluateVerdictNoGo(t *testing.T) {
	pol, ok := policy.GetPreset("perfops-gateway")
	if !ok {
		t.Fatalf("expected preset perfops-gateway")
	}

	// p95 = 290ms (> 180 * 1.15 = 207ms) and errorRate = 0.015 (> 0.002) -> NO_GO
	report := Evaluate(EvaluationInput{
		PolicyName: "perfops-gateway",
		P95Ms:      290.0,
		P99Ms:      550.0,
		ErrorRate:  floatPtr(0.015),
		RPS:        120.0,
		FinOps: &FinOpsTelemetry{
			HourlyCostUSD: 55.0,
		},
	}, pol)

	if report.Verdict != VerdictNoGo {
		t.Fatalf("expected NO_GO verdict, got %s", report.Verdict)
	}
	if len(report.Violations) < 3 {
		t.Fatalf("expected multiple violations, got %v", report.Violations)
	}
}

func TestLoadYAMLPolicyAndEvaluate(t *testing.T) {
	yamlPath := filepath.Join("..", "..", "policies", "globlex-enterprise-slo.yaml")
	pol, err := policy.LoadPolicyFile(yamlPath)
	if err != nil {
		t.Fatalf("failed to load YAML policy: %v", err)
	}
	if pol.Name != "globlex-b2b" || pol.P95Ms != 350 {
		t.Fatalf("unexpected parsed policy: %+v", pol)
	}

	report := Evaluate(EvaluationInput{
		P95Ms:     310.0,
		P99Ms:     520.0,
		ErrorRate: floatPtr(0.001),
		RPS:       150.0,
	}, pol)
	if report.Verdict != VerdictGo {
		t.Fatalf("expected GO verdict against YAML policy, got %s", report.Verdict)
	}
}

func TestProbeTarget(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer ts.Close()

	res, err := ProbeTarget(context.Background(), ts.URL, 4, 2*time.Second)
	if err != nil {
		t.Fatalf("ProbeTarget failed: %v", err)
	}
	if res.Samples != 4 || res.Successes != 4 || res.ErrorRate != 0 {
		t.Fatalf("unexpected probe result: %+v", res)
	}
	if res.StatusCodes[http.StatusOK] != 4 {
		t.Fatalf("expected 4 HTTP 200 responses, got %+v", res.StatusCodes)
	}
}
