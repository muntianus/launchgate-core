package main

import (
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
	os.Exit(Run(os.Args[1:], os.Stdout, os.Stderr))
}

// Run executes the LaunchGate CLI or HTTP server and returns an exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		if os.Getenv("PORT") != "" {
			return serve(nil, stdout, stderr)
		}
		return evaluate([]string{"--p95", "185", "--error-rate", "0.001", "--rps", "160", "--policy", "globlex-b2b"}, stdout, stderr)
	}

	switch args[0] {
	case "serve", "server":
		return serve(args[1:], stdout, stderr)
	case "evaluate", "eval":
		return evaluate(args[1:], stdout, stderr)
	default:
		return evaluate(args, stdout, stderr)
	}
}

func serve(args []string, stdout, stderr io.Writer) int {
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

func evaluate(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("evaluate", flag.ContinueOnError)
	fs.SetOutput(stderr)

	p95 := fs.Float64("p95", 0, "p95 latency in milliseconds")
	p99 := fs.Float64("p99", 0, "p99 latency in milliseconds")
	errRate := fs.Float64("error-rate", 0, "HTTP error rate ratio")
	rps := fs.Float64("rps", 0, "Throughput in requests per second")
	policyName := fs.String("policy", "globlex-b2b", "Preset policy name or YAML path")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	pol, err := policy.ResolvePolicy(*policyName, nil)
	if err != nil {
		fmt.Fprintf(stderr, "policy error: %v\n", err)
		return 2
	}

	report := evaluator.Evaluate(evaluator.EvaluationInput{
		PolicyName: *policyName,
		P95Ms:      *p95,
		P99Ms:      *p99,
		ErrorRate:  errRate,
		RPS:        *rps,
	}, pol)

	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(report)
	return 0
}
