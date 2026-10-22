package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/muntianus/launchgate-core/internal/evaluator"
	"github.com/muntianus/launchgate-core/internal/policy"
)

const Version = "1.0.0"

// ProbeRequest is the payload for POST /api/v1/probe.
type ProbeRequest struct {
	TargetURL    string         `json:"target_url"`
	Samples      int            `json:"samples,omitempty"`
	TimeoutMs    int            `json:"timeout_ms,omitempty"`
	PolicyName   string         `json:"policy,omitempty"`
	CustomPolicy *policy.Policy `json:"custom_policy,omitempty"`
	Service      string         `json:"service,omitempty"`
	ReleaseSHA   string         `json:"release_sha,omitempty"`
}

// ProbeResponse combines live probe telemetry with the SLO evaluation report.
type ProbeResponse struct {
	Probe  evaluator.ProbeResult      `json:"probe"`
	Report evaluator.EvaluationReport `json:"report"`
}

// Route is one documented endpoint. Handlers still reject other methods with a
// JSON 405 themselves, so Methods documents rather than enforces.
type Route struct {
	Methods []string
	Path    string
	Handler http.HandlerFunc
}

// Routes lists every endpoint the service answers. contract_test.go keeps this
// table and docs/api/openapi.json in lockstep.
var Routes = []Route{
	{Methods: []string{http.MethodGet, http.MethodHead}, Path: "/health", Handler: handleHealth},
	{Methods: []string{http.MethodGet}, Path: "/api/v1/policies", Handler: handlePolicies},
	{Methods: []string{http.MethodPost}, Path: "/api/v1/evaluate", Handler: handleEvaluate},
	{Methods: []string{http.MethodPost}, Path: "/api/v1/probe", Handler: handleProbe},
}

// NewHandler constructs the HTTP router for LaunchGate Core.
func NewHandler() http.Handler {
	mux := http.NewServeMux()
	for _, route := range Routes {
		mux.HandleFunc(route.Path, route.Handler)
	}
	return withCORS(mux)
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"service": "launchgate-core",
		"version": Version,
	})
}

func handlePolicies(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	presets := policy.ListPresets()
	writeJSON(w, http.StatusOK, map[string]any{
		"count":    len(presets),
		"policies": presets,
	})
}

func handleEvaluate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 2<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read request body")
		return
	}
	var input evaluator.EvaluationInput
	if err := json.Unmarshal(body, &input); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON payload: %v", err))
		return
	}

	pol, err := policy.ResolvePolicy(input.PolicyName, input.CustomPolicy)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	report := evaluator.Evaluate(input, pol)
	writeJSON(w, http.StatusOK, report)
}

func handleProbe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read request body")
		return
	}

	req := ProbeRequest{
		TargetURL:  "https://api.globlex.pro/health",
		Samples:    5,
		TimeoutMs:  4000,
		PolicyName: "globlex-b2b",
	}
	if len(body) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON payload: %v", err))
			return
		}
	}
	if req.TargetURL == "" {
		req.TargetURL = "https://api.globlex.pro/health"
	}

	pol, err := policy.ResolvePolicy(req.PolicyName, req.CustomPolicy)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	timeout := time.Duration(req.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = 4 * time.Second
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	probeRes, err := evaluator.ProbeTarget(ctx, req.TargetURL, req.Samples, timeout)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	errRate := probeRes.ErrorRate
	evalInput := evaluator.EvaluationInput{
		PolicyName: pol.Name,
		Service:    req.Service,
		ReleaseSHA: req.ReleaseSHA,
		P95Ms:      probeRes.P95Ms,
		P99Ms:      probeRes.P99Ms,
		AvgMs:      probeRes.AvgMs,
		ErrorRate:  &errRate,
		RPS:        0, // live single-client probe does not enforce bulk load RPS floor unless configured via k6
	}
	report := evaluator.Evaluate(evalInput, pol)

	writeJSON(w, http.StatusOK, ProbeResponse{
		Probe:  probeRes,
		Report: report,
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(payload)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{
		"error": msg,
	})
}
