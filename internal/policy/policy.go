package policy

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// ChaosPolicy defines resilience expectations under fault injection.
type ChaosPolicy struct {
	RequireChaosPass bool    `json:"require_chaos_pass" yaml:"require_chaos_pass"`
	MaxRecoveryMs    float64 `json:"max_recovery_ms" yaml:"max_recovery_ms"`
	MaxDropRate      float64 `json:"max_drop_rate" yaml:"max_drop_rate"`
}

// Policy defines an SLO release gate policy.
type Policy struct {
	Name                    string      `json:"name" yaml:"name"`
	Service                 string      `json:"service" yaml:"service"`
	Environment             string      `json:"environment" yaml:"environment"`
	Description             string      `json:"description" yaml:"description"`
	P95Ms                   float64     `json:"p95_ms" yaml:"p95_ms"`
	P99Ms                   float64     `json:"p99_ms" yaml:"p99_ms"`
	ErrorRateMax            float64     `json:"error_rate_max" yaml:"error_rate_max"`
	MinRPS                  float64     `json:"min_rps" yaml:"min_rps"`
	FinOpsMaxHourlyUSD      float64     `json:"finops_max_hourly_usd" yaml:"finops_max_hourly_usd"`
	MaxRegressionPct        float64     `json:"max_regression_pct" yaml:"max_regression_pct"`
	ConditionalTolerancePct float64     `json:"conditional_tolerance_pct" yaml:"conditional_tolerance_pct"`
	Chaos                   ChaosPolicy `json:"chaos" yaml:"chaos"`
}

var builtinPresets = map[string]Policy{
	"globlex-b2b": {
		Name:                    "globlex-b2b",
		Service:                 "globlex-api",
		Environment:             "production",
		Description:             "Globex B2B cross-border payment & settlement corridor SLO policy",
		P95Ms:                   350.0,
		P99Ms:                   700.0,
		ErrorRateMax:            0.005, // 0.5%
		MinRPS:                  100.0,
		FinOpsMaxHourlyUSD:      45.0,
		MaxRegressionPct:        15.0,
		ConditionalTolerancePct: 15.0,
		Chaos: ChaosPolicy{
			RequireChaosPass: false,
			MaxRecoveryMs:    2500.0,
			MaxDropRate:      0.02,
		},
	},
	"perfops-gateway": {
		Name:                    "perfops-gateway",
		Service:                 "perfops-edge-gateway",
		Environment:             "production",
		Description:             "High-throughput PerfOps telemetry & routing gateway SLO policy",
		P95Ms:                   180.0,
		P99Ms:                   350.0,
		ErrorRateMax:            0.002, // 0.2%
		MinRPS:                  250.0,
		FinOpsMaxHourlyUSD:      30.0,
		MaxRegressionPct:        12.0,
		ConditionalTolerancePct: 15.0,
		Chaos: ChaosPolicy{
			RequireChaosPass: false,
			MaxRecoveryMs:    1500.0,
			MaxDropRate:      0.01,
		},
	},
	"aco-control-plane": {
		Name:                    "aco-control-plane",
		Service:                 "autonomous-chaos-orchestrator",
		Environment:             "production",
		Description:             "Autonomous Chaos Orchestrator (ACO) control plane & agent fleet SLO policy",
		P95Ms:                   250.0,
		P99Ms:                   500.0,
		ErrorRateMax:            0.01, // 1.0%
		MinRPS:                  50.0,
		FinOpsMaxHourlyUSD:      25.0,
		MaxRegressionPct:        20.0,
		ConditionalTolerancePct: 20.0,
		Chaos: ChaosPolicy{
			RequireChaosPass: true,
			MaxRecoveryMs:    2000.0,
			MaxDropRate:      0.015,
		},
	},
}

// GetPreset returns a built-in SLO policy by name.
func GetPreset(name string) (Policy, bool) {
	key := strings.ToLower(strings.TrimSpace(name))
	p, ok := builtinPresets[key]
	return p, ok
}

// ListPresets returns all built-in SLO preset policies in deterministic order.
func ListPresets() []Policy {
	keys := []string{"globlex-b2b", "perfops-gateway", "aco-control-plane"}
	out := make([]Policy, 0, len(keys))
	for _, k := range keys {
		out = append(out, builtinPresets[k])
	}
	return out
}

// ResolvePolicy resolves a policy from a custom struct, preset name, or file path.
func ResolvePolicy(nameOrPath string, custom *Policy) (Policy, error) {
	if custom != nil && (custom.P95Ms > 0 || custom.ErrorRateMax > 0) {
		p := *custom
		if p.Name == "" {
			p.Name = "custom"
		}
		if p.ConditionalTolerancePct <= 0 {
			p.ConditionalTolerancePct = 15.0
		}
		if p.MaxRegressionPct <= 0 {
			p.MaxRegressionPct = 15.0
		}
		return p, nil
	}
	target := strings.TrimSpace(nameOrPath)
	if target == "" {
		target = "globlex-b2b"
	}
	if p, ok := GetPreset(target); ok {
		return p, nil
	}
	if _, err := os.Stat(target); err == nil {
		return LoadPolicyFile(target)
	}
	return Policy{}, fmt.Errorf("unknown policy preset or file: %q", target)
}

// LoadPolicyFile reads and parses a YAML or JSON SLO policy file from disk.
func LoadPolicyFile(path string) (Policy, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Policy{}, fmt.Errorf("read policy file %s: %w", path, err)
	}
	return ParseYAML(raw)
}

// ParseYAML parses a YAML or JSON policy definition using pure Go standard library.
func ParseYAML(data []byte) (Policy, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return Policy{}, fmt.Errorf("empty policy payload")
	}

	// Default baseline policy values
	p := Policy{
		Name:                    "custom-yaml",
		Environment:             "production",
		ConditionalTolerancePct: 15.0,
		MaxRegressionPct:        15.0,
		Chaos: ChaosPolicy{
			MaxRecoveryMs: 2500.0,
			MaxDropRate:   0.02,
		},
	}

	if trimmed[0] == '{' {
		if err := json.Unmarshal(trimmed, &p); err != nil {
			return Policy{}, fmt.Errorf("parse JSON policy: %w", err)
		}
		return p, nil
	}

	scanner := bufio.NewScanner(bytes.NewReader(trimmed))
	section := ""
	for scanner.Scan() {
		line := scanner.Text()
		if idx := strings.Index(line, "#"); idx >= 0 {
			line = line[:idx]
		}
		if strings.TrimSpace(line) == "" || strings.TrimSpace(line) == "---" {
			continue
		}

		indent := len(line) - len(strings.TrimLeft(line, " \t"))
		clean := strings.TrimSpace(line)
		parts := strings.SplitN(clean, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(parts[0]))
		val := strings.TrimSpace(parts[1])
		val = strings.Trim(val, `"'`)

		if val == "" {
			if indent == 0 {
				section = key
			} else {
				section = key
			}
			continue
		}

		if indent == 0 {
			section = ""
		}

		applyKeyValue(&p, section, key, val)
	}

	if err := scanner.Err(); err != nil {
		return Policy{}, fmt.Errorf("scan YAML policy: %w", err)
	}
	if p.P95Ms <= 0 {
		return Policy{}, fmt.Errorf("invalid policy: p95_ms must be > 0")
	}
	return p, nil
}

func applyKeyValue(p *Policy, section, key, val string) {
	switch key {
	case "name", "policy_name":
		if section == "" || section == "metadata" || section == "policy" {
			p.Name = val
		}
	case "service", "target_service":
		p.Service = val
	case "environment", "env":
		p.Environment = val
	case "description":
		p.Description = val
	case "p95_ms", "max_p95_ms":
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			p.P95Ms = f
		}
	case "p99_ms", "max_p99_ms":
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			p.P99Ms = f
		}
	case "error_rate_max", "max_error_rate":
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			p.ErrorRateMax = f
		}
	case "min_rps", "rps_min":
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			p.MinRPS = f
		}
	case "finops_max_hourly_usd", "max_hourly_usd":
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			p.FinOpsMaxHourlyUSD = f
		}
	case "max_regression_pct":
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			p.MaxRegressionPct = f
		}
	case "conditional_tolerance_pct":
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			p.ConditionalTolerancePct = f
		}
	case "require_chaos_pass":
		if b, err := strconv.ParseBool(val); err == nil {
			p.Chaos.RequireChaosPass = b
		}
	case "max_recovery_ms", "chaos_recovery_max_ms":
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			p.Chaos.MaxRecoveryMs = f
		}
	case "max_drop_rate", "chaos_max_drop_rate":
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			p.Chaos.MaxDropRate = f
		}
	}
}
