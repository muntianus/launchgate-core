package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/muntianus/launchgate-core/internal/evaluator"
	"github.com/muntianus/launchgate-core/internal/policy"
	"github.com/muntianus/launchgate-core/internal/server"
)

func main() {
	os.Exit(RunCLI(os.Args[1:], os.Stdout, os.Stderr))
}

// RunCLI executes the LaunchGate CLI or HTTP server and returns an exit code.
func RunCLI(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		if os.Getenv("PORT") != "" {
			return runServe(nil, stdout, stderr)
		}
		// Default demo evaluation when invoked with no arguments and no PORT
		return runEvaluate([]string{"--p95", "185", "--error-rate", "0.001", "--rps", "160", "--policy", "globlex-b2b"}, stdout, stderr)
	}

	sub := args[0]
	switch sub {
	case "serve", "server":
		return runServe(args[1:], stdout, stderr)
	case "evaluate", "eval":
		return runEvaluate(args[1:], stdout, stderr)
	case "probe":
		return runProbe(args[1:], stdout, stderr)
	case "policies":
		presets := policy.ListPresets()
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(presets)
		return 0
	default:
		// Support passing flags directly (`launchgate --p95 320 ...`)
		return runEvaluate(args, stdout, stderr)
	}
}

func runServe(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	defaultPort := os.Getenv("PORT")
	if defaultPort == "" {
		defaultPort = "4020"
	}
	port := fs.String("port", defaultPort, "HTTP listen port")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	addr := ":" + *port
	fmt.Fprintf(stdout, "LaunchGate Core v%s listening on %s\n", server.Version, addr)
	srv := &http.Server{
		Addr:              addr,
		Handler:           server.NewHandler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Fprintf(stderr, "server error: %v\n", err)
		return 1
	}
	return 0
}

func runEvaluate(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("evaluate", flag.ContinueOnError)
	fs.SetOutput(stderr)

	p95 := fs.Float64("p95", 0, "p95 latency in milliseconds")
	p99 := fs.Float64("p99", 0, "p99 latency in milliseconds")
	errRate := fs.Float64("error-rate", 0, "HTTP error rate ratio (0.0 - 1.0)")
	rps := fs.Float64("rps", 0, "Throughput in requests per second")
	hourlyUSD := fs.Float64("hourly-usd", 0, "Estimated hourly workload cost in USD")
	policyName := fs.String("policy", "globlex-b2b", "Preset policy name or path to YAML policy file")
	service := fs.String("service", "", "Service name override")
	releaseSHA := fs.String("sha", "", "Release commit SHA")
	summaryFile := fs.String("summary", "", "Optional k6 summary JSON file path")
	failOnNoGo := fs.Bool("fail-on-nogo", false, "Return exit code 1 when verdict is NO_GO")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	input := evaluator.EvaluationInput{
		PolicyName: *policyName,
		Service:    *service,
		ReleaseSHA: *releaseSHA,
		P95Ms:      *p95,
		P99Ms:      *p99,
		ErrorRate:  errRate,
		RPS:        *rps,
	}
	if *hourlyUSD > 0 {
		input.FinOps = &evaluator.FinOpsTelemetry{HourlyCostUSD: *hourlyUSD}
	}

	if *summaryFile != "" {
		raw, err := os.ReadFile(*summaryFile)
		if err != nil {
			fmt.Fprintf(stderr, "failed to read summary file: %v\n", err)
			return 2
		}
		var fileInput evaluator.EvaluationInput
		if err := json.Unmarshal(raw, &fileInput); err != nil {
			fmt.Fprintf(stderr, "invalid summary JSON: %v\n", err)
			return 2
		}
		if input.P95Ms == 0 {
			input.P95Ms = fileInput.P95Ms
		}
		if input.P99Ms == 0 {
			input.P99Ms = fileInput.P99Ms
		}
		if input.RPS == 0 {
			input.RPS = fileInput.RPS
		}
		if fileInput.ErrorRate != nil && *errRate == 0 {
			input.ErrorRate = fileInput.ErrorRate
		}
		if fileInput.Metrics != nil {
			input.Metrics = fileInput.Metrics
		}
	}

	pol, err := policy.ResolvePolicy(*policyName, nil)
	if err != nil {
		fmt.Fprintf(stderr, "policy error: %v\n", err)
		return 2
	}

	report := evaluator.Evaluate(input, pol)
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(report)

	if *failOnNoGo && report.Verdict == evaluator.VerdictNoGo {
		return 1
	}
	return 0
}

func runProbe(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("probe", flag.ContinueOnError)
	fs.SetOutput(stderr)

	targetURL := fs.String("url", "https://api.globlex.pro/health", "Target HTTP URL to probe")
	samples := fs.Int("samples", 5, "Number of HTTP samples")
	policyName := fs.String("policy", "globlex-b2b", "SLO policy preset or file")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	pol, err := policy.ResolvePolicy(*policyName, nil)
	if err != nil {
		fmt.Fprintf(stderr, "policy error: %v\n", err)
		return 2
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	probeRes, err := evaluator.ProbeTarget(ctx, *targetURL, *samples, 4*time.Second)
	if err != nil {
		fmt.Fprintf(stderr, "probe error: %v\n", err)
		return 1
	}

	er := probeRes.ErrorRate
	report := evaluator.Evaluate(evaluator.EvaluationInput{
		PolicyName: pol.Name,
		P95Ms:      probeRes.P95Ms,
		P99Ms:      probeRes.P99Ms,
		AvgMs:      probeRes.AvgMs,
		ErrorRate:  &er,
	}, pol)

	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(server.ProbeResponse{
		Probe:  probeRes,
		Report: report,
	})
	return 0
}
