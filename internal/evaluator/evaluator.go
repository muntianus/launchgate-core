package evaluator

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"sort"
	"time"

	"github.com/muntianus/launchgate-core/internal/policy"
)

// Verdict represents the release gate decision.
type Verdict string

const (
	VerdictGo            Verdict = "GO"
	VerdictConditionalGo Verdict = "CONDITIONAL_GO"
	VerdictNoGo          Verdict = "NO_GO"
)

// K6DurationMetrics models the standard k6 http_req_duration summary block.
type K6DurationMetrics struct {
	Avg float64 `json:"avg"`
	Min float64 `json:"min"`
	Med float64 `json:"med"`
	Max float64 `json:"max"`
	P90 float64 `json:"p(90)"`
	P95 float64 `json:"p(95)"`
	P99 float64 `json:"p(99)"`
}

// K6RateMetrics models k6 rate metrics such as http_req_failed.
type K6RateMetrics struct {
	Rate   float64 `json:"rate"`
	Value  float64 `json:"value"`
	Passes int64   `json:"passes"`
	Fails  int64   `json:"fails"`
}

// K6CounterMetrics models k6 counter metrics such as http_reqs.
type K6CounterMetrics struct {
	Count int64   `json:"count"`
	Rate  float64 `json:"rate"`
}

// K6SummaryMetrics holds nested k6 summary metrics.
type K6SummaryMetrics struct {
	HTTPReqDuration *K6DurationMetrics `json:"http_req_duration,omitempty"`
	HTTPReqFailed   *K6RateMetrics     `json:"http_req_failed,omitempty"`
	HTTPReqs        *K6CounterMetrics  `json:"http_reqs,omitempty"`
}

// ChaosTelemetry captures fault-injection resilience signals.
type ChaosTelemetry struct {
	Injected   bool    `json:"injected"`
	Passed     bool    `json:"passed"`
	RecoveryMs float64 `json:"recovery_ms"`
	DropRate   float64 `json:"drop_rate"`
}

// FinOpsTelemetry captures workload cost signals.
type FinOpsTelemetry struct {
	HourlyCostUSD float64 `json:"hourly_cost_usd"`
}

// BaselineMetrics captures prior release baseline metrics for regression calculation.
type BaselineMetrics struct {
	P95Ms     float64 `json:"p95_ms"`
	P99Ms     float64 `json:"p99_ms"`
	ErrorRate float64 `json:"error_rate"`
	RPS       float64 `json:"rps"`
}

// EvaluationInput supports both raw k6 summary objects and direct metric fields.
type EvaluationInput struct {
	PolicyName   string            `json:"policy,omitempty"`
	CustomPolicy *policy.Policy    `json:"custom_policy,omitempty"`
	Service      string            `json:"service,omitempty"`
	ReleaseSHA   string            `json:"release_sha,omitempty"`
	P95Ms        float64           `json:"p95_ms,omitempty"`
	P99Ms        float64           `json:"p99_ms,omitempty"`
	AvgMs        float64           `json:"avg_ms,omitempty"`
	ErrorRate    *float64          `json:"error_rate,omitempty"`
	RPS          float64           `json:"rps,omitempty"`
	Metrics      *K6SummaryMetrics `json:"metrics,omitempty"`
	Baseline     *BaselineMetrics  `json:"baseline,omitempty"`
	FinOps       *FinOpsTelemetry  `json:"finops,omitempty"`
	Chaos        *ChaosTelemetry   `json:"chaos,omitempty"`
}

// RuleCheck represents the outcome of a single SLO rule check.
type RuleCheck struct {
	Rule      string  `json:"rule"`
	Status    Verdict `json:"status"`
	Actual    float64 `json:"actual"`
	Threshold float64 `json:"threshold"`
	Unit      string  `json:"unit"`
	Message   string  `json:"message"`
}

// RegressionDelta tracks changes between baseline and candidate release.
type RegressionDelta struct {
	HasBaseline       bool    `json:"has_baseline"`
	P95DeltaMs        float64 `json:"p95_delta_ms,omitempty"`
	P95DeltaPct       float64 `json:"p95_delta_pct,omitempty"`
	ErrorRateDelta    float64 `json:"error_rate_delta,omitempty"`
	RPSDeltaPct       float64 `json:"rps_delta_pct,omitempty"`
	RegressionFlagged bool    `json:"regression_flagged"`
}

// NormalizedMetrics shows the resolved telemetry values used during evaluation.
type NormalizedMetrics struct {
	P95Ms         float64 `json:"p95_ms"`
	P99Ms         float64 `json:"p99_ms"`
	AvgMs         float64 `json:"avg_ms"`
	ErrorRate     float64 `json:"error_rate"`
	RPS           float64 `json:"rps"`
	HourlyCostUSD float64 `json:"hourly_cost_usd,omitempty"`
}

// EvaluationReport is the structured release readiness decision output.
type EvaluationReport struct {
	Verdict           Verdict           `json:"verdict"`
	Score             int               `json:"score"`
	Policy            string            `json:"policy"`
	Service           string            `json:"service"`
	ReleaseSHA        string            `json:"release_sha,omitempty"`
	EvaluatedAt       string            `json:"evaluated_at"`
	NormalizedMetrics NormalizedMetrics `json:"normalized_metrics"`
	Checks            []RuleCheck       `json:"checks"`
	Violations        []string          `json:"violations"`
	RemediationAdvice []string          `json:"remediation_advice"`
	Regression        RegressionDelta   `json:"regression"`
}

// Normalize extracts unified metrics from either flat fields or nested k6 summary metrics.
func Normalize(in EvaluationInput) NormalizedMetrics {
	nm := NormalizedMetrics{
		P95Ms: in.P95Ms,
		P99Ms: in.P99Ms,
		AvgMs: in.AvgMs,
		RPS:   in.RPS,
	}
	if in.ErrorRate != nil {
		nm.ErrorRate = *in.ErrorRate
	}
	if in.Metrics != nil {
		if in.Metrics.HTTPReqDuration != nil {
			if nm.P95Ms == 0 {
				nm.P95Ms = in.Metrics.HTTPReqDuration.P95
			}
			if nm.P99Ms == 0 {
				nm.P99Ms = in.Metrics.HTTPReqDuration.P99
			}
			if nm.AvgMs == 0 {
				nm.AvgMs = in.Metrics.HTTPReqDuration.Avg
			}
		}
		if in.Metrics.HTTPReqFailed != nil && in.ErrorRate == nil {
			if in.Metrics.HTTPReqFailed.Rate > 0 {
				nm.ErrorRate = in.Metrics.HTTPReqFailed.Rate
			} else {
				nm.ErrorRate = in.Metrics.HTTPReqFailed.Value
			}
		}
		if in.Metrics.HTTPReqs != nil && nm.RPS == 0 {
			nm.RPS = in.Metrics.HTTPReqs.Rate
		}
	}
	if in.FinOps != nil {
		nm.HourlyCostUSD = in.FinOps.HourlyCostUSD
	}
	return nm
}

// Evaluate runs the deterministic SLO evaluation engine against a policy.
func Evaluate(in EvaluationInput, pol policy.Policy) EvaluationReport {
	nm := Normalize(in)
	tol := pol.ConditionalTolerancePct / 100.0
	if tol <= 0 {
		tol = 0.15
	}

	score := 100
	overall := VerdictGo
	var checks []RuleCheck
	var violations []string
	var advice []string

	promoteVerdict := func(v Verdict) {
		if v == VerdictNoGo {
			overall = VerdictNoGo
		} else if v == VerdictConditionalGo && overall == VerdictGo {
			overall = VerdictConditionalGo
		}
	}

	// 1. P95 Latency Check
	if pol.P95Ms > 0 {
		hardLimit := round2(pol.P95Ms * (1.0 + tol))
		status := VerdictGo
		msg := fmt.Sprintf("p95 latency %.1fms is within SLO budget (<= %.1fms)", nm.P95Ms, pol.P95Ms)
		if nm.P95Ms > hardLimit {
			status = VerdictNoGo
			score -= 40
			msg = fmt.Sprintf("p95 latency %.1fms exceeds hard gate limit %.1fms (SLO %.1fms + %.0f%% buffer)", nm.P95Ms, hardLimit, pol.P95Ms, tol*100)
			violations = append(violations, msg)
			advice = append(advice, "Inspect hot DB queries, connection pool saturation, or upstream RPC tail latency before promoting release.")
		} else if nm.P95Ms > pol.P95Ms {
			status = VerdictConditionalGo
			score -= 15
			msg = fmt.Sprintf("p95 latency %.1fms exceeds target %.1fms but is within conditional tolerance (<= %.1fms)", nm.P95Ms, pol.P95Ms, hardLimit)
			violations = append(violations, msg)
			advice = append(advice, "Deploy with canary traffic cap (10-25%) and monitor p95 latency burn rate.")
		}
		promoteVerdict(status)
		checks = append(checks, RuleCheck{
			Rule:      "p95_latency_ms",
			Status:    status,
			Actual:    round2(nm.P95Ms),
			Threshold: pol.P95Ms,
			Unit:      "ms",
			Message:   msg,
		})
	}

	// 2. P99 Latency Check
	if pol.P99Ms > 0 && nm.P99Ms > 0 {
		hardLimit := round2(pol.P99Ms * (1.0 + tol))
		status := VerdictGo
		msg := fmt.Sprintf("p99 latency %.1fms is within SLO budget (<= %.1fms)", nm.P99Ms, pol.P99Ms)
		if nm.P99Ms > hardLimit {
			status = VerdictNoGo
			score -= 30
			msg = fmt.Sprintf("p99 latency %.1fms exceeds hard gate limit %.1fms (SLO %.1fms)", nm.P99Ms, hardLimit, pol.P99Ms)
			violations = append(violations, msg)
			advice = append(advice, "Investigate GC pauses, lock contention, or cold-start spikes driving p99 tail latency.")
		} else if nm.P99Ms > pol.P99Ms {
			status = VerdictConditionalGo
			score -= 12
			msg = fmt.Sprintf("p99 latency %.1fms exceeds target %.1fms but is within conditional tolerance (<= %.1fms)", nm.P99Ms, pol.P99Ms, hardLimit)
			violations = append(violations, msg)
			advice = append(advice, "Enable tail-based tracing sampling to isolate p99 outliers during rollout.")
		}
		promoteVerdict(status)
		checks = append(checks, RuleCheck{
			Rule:      "p99_latency_ms",
			Status:    status,
			Actual:    round2(nm.P99Ms),
			Threshold: pol.P99Ms,
			Unit:      "ms",
			Message:   msg,
		})
	}

	// 3. Error Rate Check
	if pol.ErrorRateMax > 0 {
		status := VerdictGo
		msg := fmt.Sprintf("error rate %.4f is within error budget (<= %.4f)", nm.ErrorRate, pol.ErrorRateMax)
		if nm.ErrorRate > pol.ErrorRateMax {
			status = VerdictNoGo
			score -= 50
			msg = fmt.Sprintf("error rate %.4f (%.2f%%) breaches maximum SLO error budget %.4f (%.2f%%)", nm.ErrorRate, nm.ErrorRate*100, pol.ErrorRateMax, pol.ErrorRateMax*100)
			violations = append(violations, msg)
			advice = append(advice, "Halt rollout immediately: error budget breached. Inspect 5xx responses and circuit breaker trips.")
		} else if nm.ErrorRate > pol.ErrorRateMax*0.80 && nm.ErrorRate > 0 {
			status = VerdictConditionalGo
			score -= 10
			msg = fmt.Sprintf("error rate %.4f is consuming >80%% of error budget (<= %.4f)", nm.ErrorRate, pol.ErrorRateMax)
			violations = append(violations, msg)
			advice = append(advice, "Error rate is near threshold; keep automated rollback armed on 5xx spikes.")
		}
		promoteVerdict(status)
		checks = append(checks, RuleCheck{
			Rule:      "error_rate_max",
			Status:    status,
			Actual:    round4(nm.ErrorRate),
			Threshold: pol.ErrorRateMax,
			Unit:      "ratio",
			Message:   msg,
		})
	}

	// 4. Minimum RPS Throughput Check
	if pol.MinRPS > 0 && nm.RPS > 0 {
		status := VerdictGo
		msg := fmt.Sprintf("throughput %.1f RPS meets minimum capacity target (>= %.1f RPS)", nm.RPS, pol.MinRPS)
		minSoft := round2(pol.MinRPS * (1.0 - tol))
		if nm.RPS < minSoft {
			status = VerdictNoGo
			score -= 25
			msg = fmt.Sprintf("throughput %.1f RPS is critically below minimum capacity floor %.1f RPS (target >= %.1f RPS)", nm.RPS, minSoft, pol.MinRPS)
			violations = append(violations, msg)
			advice = append(advice, "Throughput saturation detected: verify horizontal pod autoscaler (HPA) limits and worker concurrency.")
		} else if nm.RPS < pol.MinRPS {
			status = VerdictConditionalGo
			score -= 10
			msg = fmt.Sprintf("throughput %.1f RPS is slightly below target %.1f RPS (soft floor %.1f RPS)", nm.RPS, pol.MinRPS, minSoft)
			violations = append(violations, msg)
			advice = append(advice, "Pre-scale replicas before peak traffic windows.")
		}
		promoteVerdict(status)
		checks = append(checks, RuleCheck{
			Rule:      "min_rps",
			Status:    status,
			Actual:    round2(nm.RPS),
			Threshold: pol.MinRPS,
			Unit:      "rps",
			Message:   msg,
		})
	}

	// 5. FinOps Hourly Cost Guardrail
	if pol.FinOpsMaxHourlyUSD > 0 && nm.HourlyCostUSD > 0 {
		status := VerdictGo
		msg := fmt.Sprintf("hourly workload cost $%.2f/hr is within FinOps budget (<= $%.2f/hr)", nm.HourlyCostUSD, pol.FinOpsMaxHourlyUSD)
		hardCost := round2(pol.FinOpsMaxHourlyUSD * 1.25)
		if nm.HourlyCostUSD > hardCost {
			status = VerdictNoGo
			score -= 25
			msg = fmt.Sprintf("hourly cost $%.2f/hr exceeds hard FinOps ceiling $%.2f/hr (budget $%.2f/hr)", nm.HourlyCostUSD, hardCost, pol.FinOpsMaxHourlyUSD)
			violations = append(violations, msg)
			advice = append(advice, "Cost per request regressed >25%%: check over-provisioned CPU/memory requests or redundant LLM/egress calls.")
		} else if nm.HourlyCostUSD > pol.FinOpsMaxHourlyUSD {
			status = VerdictConditionalGo
			score -= 10
			msg = fmt.Sprintf("hourly cost $%.2f/hr exceeds FinOps target $%.2f/hr (within +25%% ceiling)", nm.HourlyCostUSD, pol.FinOpsMaxHourlyUSD)
			violations = append(violations, msg)
			advice = append(advice, "Request FinOps sign-off or right-size container CPU requests post-deploy.")
		}
		promoteVerdict(status)
		checks = append(checks, RuleCheck{
			Rule:      "finops_max_hourly_usd",
			Status:    status,
			Actual:    round2(nm.HourlyCostUSD),
			Threshold: pol.FinOpsMaxHourlyUSD,
			Unit:      "usd/hr",
			Message:   msg,
		})
	}

	// 6. Chaos Resilience Check
	if in.Chaos != nil && (in.Chaos.Injected || pol.Chaos.RequireChaosPass) {
		status := VerdictGo
		msg := fmt.Sprintf("chaos resilience verified (recovery %.0fms <= %.0fms, drop rate %.4f <= %.4f)",
			in.Chaos.RecoveryMs, pol.Chaos.MaxRecoveryMs, in.Chaos.DropRate, pol.Chaos.MaxDropRate)
		if !in.Chaos.Passed || (pol.Chaos.MaxRecoveryMs > 0 && in.Chaos.RecoveryMs > pol.Chaos.MaxRecoveryMs*1.25) ||
			(pol.Chaos.MaxDropRate > 0 && in.Chaos.DropRate > pol.Chaos.MaxDropRate*1.5) {
			status = VerdictNoGo
			score -= 35
			msg = fmt.Sprintf("chaos experiment failed (passed=%v, recovery=%.0fms, drop_rate=%.4f)",
				in.Chaos.Passed, in.Chaos.RecoveryMs, in.Chaos.DropRate)
			violations = append(violations, msg)
			advice = append(advice, "Fix retry/backoff jitter and fallback circuit breakers under fault injection.")
		} else if (pol.Chaos.MaxRecoveryMs > 0 && in.Chaos.RecoveryMs > pol.Chaos.MaxRecoveryMs) ||
			(pol.Chaos.MaxDropRate > 0 && in.Chaos.DropRate > pol.Chaos.MaxDropRate) {
			status = VerdictConditionalGo
			score -= 12
			msg = fmt.Sprintf("chaos recovery %.0fms or drop rate %.4f exceeded target under fault injection",
				in.Chaos.RecoveryMs, in.Chaos.DropRate)
			violations = append(violations, msg)
			advice = append(advice, "Tune upstream timeout and connection retry budgets.")
		}
		promoteVerdict(status)
		checks = append(checks, RuleCheck{
			Rule:      "chaos_resilience",
			Status:    status,
			Actual:    round2(in.Chaos.RecoveryMs),
			Threshold: pol.Chaos.MaxRecoveryMs,
			Unit:      "ms",
			Message:   msg,
		})
	} else if pol.Chaos.RequireChaosPass {
		promoteVerdict(VerdictConditionalGo)
		score -= 10
		msg := "policy requires chaos resilience verification, but no chaos telemetry was supplied"
		violations = append(violations, msg)
		advice = append(advice, "Execute an ACO fault-injection game day or provide chaos telemetry to promote from CONDITIONAL_GO to GO.")
		checks = append(checks, RuleCheck{
			Rule:      "chaos_resilience",
			Status:    VerdictConditionalGo,
			Actual:    0,
			Threshold: pol.Chaos.MaxRecoveryMs,
			Unit:      "ms",
			Message:   msg,
		})
	}

	// 7. Baseline Regression Delta Analysis
	reg := RegressionDelta{}
	if in.Baseline != nil && in.Baseline.P95Ms > 0 {
		reg.HasBaseline = true
		reg.P95DeltaMs = round2(nm.P95Ms - in.Baseline.P95Ms)
		reg.P95DeltaPct = round2(((nm.P95Ms - in.Baseline.P95Ms) / in.Baseline.P95Ms) * 100.0)
		reg.ErrorRateDelta = round4(nm.ErrorRate - in.Baseline.ErrorRate)
		if in.Baseline.RPS > 0 && nm.RPS > 0 {
			reg.RPSDeltaPct = round2(((nm.RPS - in.Baseline.RPS) / in.Baseline.RPS) * 100.0)
		}

		maxReg := pol.MaxRegressionPct
		if maxReg <= 0 {
			maxReg = 15.0
		}
		if reg.P95DeltaPct > maxReg*2.0 {
			reg.RegressionFlagged = true
			promoteVerdict(VerdictNoGo)
			score -= 25
			msg := fmt.Sprintf("severe latency regression: p95 increased by +%.1f%% (+%.1fms) vs baseline (limit +%.1f%%)", reg.P95DeltaPct, reg.P95DeltaMs, maxReg)
			violations = append(violations, msg)
			advice = append(advice, "Bisect commits in candidate release to isolate latency regression >2x threshold.")
		} else if reg.P95DeltaPct > maxReg {
			reg.RegressionFlagged = true
			promoteVerdict(VerdictConditionalGo)
			score -= 10
			msg := fmt.Sprintf("moderate latency regression: p95 increased by +%.1f%% (+%.1fms) vs baseline (limit +%.1f%%)", reg.P95DeltaPct, reg.P95DeltaMs, maxReg)
			violations = append(violations, msg)
			advice = append(advice, "Compare CPU profiles between baseline and candidate release.")
		}
	}

	if score < 0 {
		score = 0
	}
	if len(advice) == 0 {
		advice = append(advice, "All SLO, FinOps, and resilience gates passed. Release is cleared for automated production promotion.")
	}

	svc := in.Service
	if svc == "" {
		svc = pol.Service
	}

	return EvaluationReport{
		Verdict:           overall,
		Score:             score,
		Policy:            pol.Name,
		Service:           svc,
		ReleaseSHA:        in.ReleaseSHA,
		EvaluatedAt:       time.Now().UTC().Format(time.RFC3339),
		NormalizedMetrics: nm,
		Checks:            checks,
		Violations:        violations,
		RemediationAdvice: advice,
		Regression:        reg,
	}
}

// ProbeResult captures live HTTP probe statistics against a target endpoint.
type ProbeResult struct {
	TargetURL    string      `json:"target_url"`
	Samples      int         `json:"samples"`
	Successes    int         `json:"successes"`
	Failures     int         `json:"failures"`
	ErrorRate    float64     `json:"error_rate"`
	MinMs        float64     `json:"min_ms"`
	AvgMs        float64     `json:"avg_ms"`
	P50Ms        float64     `json:"p50_ms"`
	P95Ms        float64     `json:"p95_ms"`
	P99Ms        float64     `json:"p99_ms"`
	MaxMs        float64     `json:"max_ms"`
	EffectiveRPS float64     `json:"effective_rps"`
	StatusCodes  map[int]int `json:"status_codes"`
	SampleMs     []float64   `json:"sample_ms"`
}

// ProbeTarget performs a live multi-sample HTTP probe against targetURL and returns latency/error metrics.
func ProbeTarget(ctx context.Context, targetURL string, samples int, timeout time.Duration) (ProbeResult, error) {
	if targetURL == "" {
		return ProbeResult{}, fmt.Errorf("target_url is required")
	}
	if samples <= 0 {
		samples = 5
	}
	if samples > 50 {
		samples = 50
	}
	if timeout <= 0 {
		timeout = 4 * time.Second
	}

	client := &http.Client{Timeout: timeout}
	res := ProbeResult{
		TargetURL:   targetURL,
		Samples:     samples,
		StatusCodes: make(map[int]int),
		SampleMs:    make([]float64, 0, samples),
	}

	suiteStart := time.Now()
	for i := 0; i < samples; i++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
		if err != nil {
			return ProbeResult{}, fmt.Errorf("invalid target_url %q: %w", targetURL, err)
		}
		req.Header.Set("User-Agent", "LaunchGate-Core-Probe/1.0")

		t0 := time.Now()
		resp, err := client.Do(req)
		elapsedMs := float64(time.Since(t0).Microseconds()) / 1000.0
		res.SampleMs = append(res.SampleMs, round2(elapsedMs))

		if err != nil {
			res.Failures++
			continue
		}
		_ = resp.Body.Close()
		res.StatusCodes[resp.StatusCode]++
		if resp.StatusCode >= 200 && resp.StatusCode < 400 {
			res.Successes++
		} else {
			res.Failures++
		}
	}

	totalSec := time.Since(suiteStart).Seconds()
	if totalSec > 0 {
		res.EffectiveRPS = round2(float64(samples) / totalSec)
	}
	res.ErrorRate = round4(float64(res.Failures) / float64(samples))

	sorted := make([]float64, len(res.SampleMs))
	copy(sorted, res.SampleMs)
	sort.Float64s(sorted)

	if len(sorted) > 0 {
		res.MinMs = sorted[0]
		res.MaxMs = sorted[len(sorted)-1]
		var sum float64
		for _, v := range sorted {
			sum += v
		}
		res.AvgMs = round2(sum / float64(len(sorted)))
		res.P50Ms = percentile(sorted, 0.50)
		res.P95Ms = percentile(sorted, 0.95)
		res.P99Ms = percentile(sorted, 0.99)
	}

	return res, nil
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if len(sorted) == 1 {
		return round2(sorted[0])
	}
	idx := p * float64(len(sorted)-1)
	lower := int(math.Floor(idx))
	upper := int(math.Ceil(idx))
	if lower == upper {
		return round2(sorted[lower])
	}
	weight := idx - float64(lower)
	return round2(sorted[lower]*(1.0-weight) + sorted[upper]*weight)
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

func round4(v float64) float64 {
	return math.Round(v*10000) / 10000
}
