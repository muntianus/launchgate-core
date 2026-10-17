package main

import "fmt"

type Verdict string

const (
	VerdictGo            Verdict = "GO"
	VerdictConditionalGo Verdict = "CONDITIONAL_GO"
	VerdictNoGo          Verdict = "NO_GO"
)

type SLOPolicy struct {
	MaxP95Ms     float64
	MaxErrorRate float64
}

func Evaluate(p95Ms, errRate float64, policy SLOPolicy) Verdict {
	if errRate > policy.MaxErrorRate || p95Ms > policy.MaxP95Ms*1.15 {
		return VerdictNoGo
	}
	if p95Ms > policy.MaxP95Ms {
		return VerdictConditionalGo
	}
	return VerdictGo
}

func main() {
	fmt.Println("LaunchGate Core Verdict:", Evaluate(185.0, 0.001, SLOPolicy{MaxP95Ms: 250.0, MaxErrorRate: 0.01}))
}
