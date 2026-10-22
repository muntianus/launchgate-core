package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func postProbe(t *testing.T, target string) *httptest.ResponseRecorder {
	t.Helper()
	payload, _ := json.Marshal(map[string]any{"target_url": target, "samples": 1, "timeout_ms": 500})
	rec := httptest.NewRecorder()
	NewHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/probe", bytes.NewReader(payload)))
	return rec
}

// TestProbeRefusesTargetsOutsideAllowlist: the probe must not reach internal
// or foreign hosts (SSRF / port scanning through the gate).
func TestProbeRefusesTargetsOutsideAllowlist(t *testing.T) {
	var hits atomic.Int32
	internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
	}))
	defer internal.Close()

	for _, target := range []string{
		internal.URL,
		"http://169.254.169.254/latest/meta-data/",
		"http://localhost:8090/api/collections",
		"https://evil.example/",
	} {
		if rec := postProbe(t, target); rec.Code != http.StatusForbidden {
			t.Errorf("%s: got %d, want 403 (%s)", target, rec.Code, rec.Body.String())
		}
	}
	for _, target := range []string{"file:///etc/passwd", "gopher://api.globlex.pro/", "::bad::"} {
		if rec := postProbe(t, target); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: got %d, want 400", target, rec.Code)
		}
	}
	if n := hits.Load(); n != 0 {
		t.Fatalf("refused target was contacted %d times", n)
	}
}

// TestProbeDoesNotFollowRedirects: an allowed host cannot bounce the probe.
func TestProbeDoesNotFollowRedirects(t *testing.T) {
	var landed atomic.Int32
	landing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { landed.Add(1) }))
	defer landing.Close()
	bouncer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, landing.URL, http.StatusFound)
	}))
	defer bouncer.Close()
	t.Setenv("LAUNCHGATE_PROBE_ALLOWED_HOSTS", "127.0.0.1")

	rec := postProbe(t, bouncer.URL)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d: %s", rec.Code, rec.Body.String())
	}
	var out ProbeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Probe.StatusCodes[http.StatusFound] != 1 {
		t.Fatalf("expected the 302 itself to be recorded, got %v", out.Probe.StatusCodes)
	}
	if landed.Load() != 0 {
		t.Fatal("redirect target was contacted")
	}
}

func TestAllowedProbeHostsFromEnv(t *testing.T) {
	t.Setenv("LAUNCHGATE_PROBE_ALLOWED_HOSTS", "")
	if hosts := AllowedProbeHosts(); len(hosts) == 0 || hosts[0] != "api.globlex.pro" {
		t.Fatalf("default allowlist = %v", hosts)
	}
	t.Setenv("LAUNCHGATE_PROBE_ALLOWED_HOSTS", " Staging.Example.com , ,api.test ")
	if hosts := AllowedProbeHosts(); len(hosts) != 2 || hosts[0] != "staging.example.com" || hosts[1] != "api.test" {
		t.Fatalf("env allowlist = %v", hosts)
	}
}
