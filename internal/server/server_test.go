package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/muntianus/launchgate-core/internal/evaluator"
)

func TestHealthEndpoint(t *testing.T) {
	h := NewHandler()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid health JSON: %v", err)
	}
	if body["status"] != "ok" || body["service"] != "launchgate-core" || body["version"] != "1.0.0" {
		t.Fatalf("unexpected health response: %+v", body)
	}
}

func TestPoliciesEndpoint(t *testing.T) {
	h := NewHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/policies", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	var resp struct {
		Count    int              `json:"count"`
		Policies []map[string]any `json:"policies"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid policies JSON: %v", err)
	}
	if resp.Count != 3 || len(resp.Policies) != 3 {
		t.Fatalf("expected 3 preset policies, got %d", resp.Count)
	}
}

func TestEvaluateEndpointVerdicts(t *testing.T) {
	h := NewHandler()

	cases := []struct {
		name     string
		payload  map[string]any
		expected evaluator.Verdict
	}{
		{
			name: "GO verdict on globlex-b2b",
			payload: map[string]any{
				"policy":     "globlex-b2b",
				"p95_ms":     210.0,
				"p99_ms":     450.0,
				"error_rate": 0.001,
				"rps":        150.0,
			},
			expected: evaluator.VerdictGo,
		},
		{
			name: "CONDITIONAL_GO verdict on globlex-b2b",
			payload: map[string]any{
				"policy":     "globlex-b2b",
				"p95_ms":     375.0,
				"p99_ms":     620.0,
				"error_rate": 0.002,
				"rps":        130.0,
			},
			expected: evaluator.VerdictConditionalGo,
		},
		{
			name: "NO_GO verdict on aco-control-plane",
			payload: map[string]any{
				"policy":     "aco-control-plane",
				"p95_ms":     420.0,
				"error_rate": 0.035,
				"rps":        20.0,
			},
			expected: evaluator.VerdictNoGo,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, _ := json.Marshal(tc.payload)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/evaluate", bytes.NewReader(raw))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
			}

			var report evaluator.EvaluationReport
			if err := json.Unmarshal(rec.Body.Bytes(), &report); err != nil {
				t.Fatalf("failed to decode report: %v", err)
			}
			if report.Verdict != tc.expected {
				t.Fatalf("expected verdict %s, got %s (score=%d, violations=%v)",
					tc.expected, report.Verdict, report.Score, report.Violations)
			}
		})
	}
}

func TestProbeEndpoint(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer upstream.Close()
	t.Setenv("LAUNCHGATE_PROBE_ALLOWED_HOSTS", "127.0.0.1")

	h := NewHandler()
	payload, _ := json.Marshal(map[string]any{
		"target_url": upstream.URL,
		"samples":    3,
		"policy":     "globlex-b2b",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/probe", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}

	var out ProbeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("invalid probe response: %v", err)
	}
	if out.Probe.Samples != 3 || out.Probe.Successes != 3 {
		t.Fatalf("expected 3 successful samples, got %+v", out.Probe)
	}
	if out.Report.Verdict != evaluator.VerdictGo {
		t.Fatalf("expected GO verdict for healthy local probe, got %s", out.Report.Verdict)
	}
}
