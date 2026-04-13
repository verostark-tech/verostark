// Package rules implements the STIM distribution key rule engine.
// It evaluates a single statement line and returns whether the received
// publisher amount deviates materially from the expected amount.
//
// Formula: expected = gross × controlled_manuscript_share × stim_key
//
// V0.1 scope: STIM Sweden only. Both mechanical and performance carry
// the same key (0.3333). Run independently per right type — do not aggregate.
//
// V0.2 note: extend stimKey map with GEMA (mechanical: 0.40),
// SACEM (mechanical: 0.50), etc. when sub-publisher statements are added.
package rules

import (
	"fmt"
	"strings"
)

// stimKey is the fixed publisher portion applied by STIM to the gross
// for the controlled manuscript share. Invariant for V0.1.
var stimKey = map[string]float64{
	"mechanical":  0.3333,
	"performance": 0.3333,
}

// flagThreshold is the minimum absolute deviation percentage to create a flag.
const flagThreshold = 0.25

// Severity levels. LOW and MEDIUM are defined for completeness but sit below
// flagThreshold and will not be returned by Evaluate in V0.1.
const (
	SeverityLow      = "LOW"      // 1–10%
	SeverityMedium   = "MEDIUM"   // 10–25%
	SeverityHigh     = "HIGH"     // 25–50%
	SeverityCritical = "CRITICAL" // >50%
)

// Input is one statement line to evaluate.
type Input struct {
	Gross                     float64 // total collected by STIM before any split
	Received                  float64 // publisher amount on the statement
	ControlledManuscriptShare float64 // sum of manuscript_share for controlled writers on this work
	RightType                 string  // "mechanical" or "performance"
}

// Result is the outcome of evaluating one statement line.
// DeviationPct is signed: positive = overpayment, negative = underpayment.
type Result struct {
	Expected        float64
	Received        float64
	DeviationAmount float64 // received − expected
	DeviationPct    float64 // deviation / expected, signed
	Flagged         bool
	Severity        string // empty when Flagged is false
}

// Evaluate applies the STIM distribution key formula to a single statement line.
// Returns Flagged=false when the absolute deviation is below flagThreshold,
// or when expected is zero (controlled_manuscript_share is zero).
func Evaluate(in Input) (Result, error) {
	rt := strings.ToLower(strings.TrimSpace(in.RightType))
	key, ok := stimKey[rt]
	if !ok {
		return Result{}, fmt.Errorf("unknown right type %q — expected \"mechanical\" or \"performance\"", in.RightType)
	}

	expected := in.Gross * in.ControlledManuscriptShare * key
	deviationAmt := in.Received - expected

	// Zero expected means no controlled share — skip detection.
	if expected == 0 {
		return Result{
			Expected:        0,
			Received:        in.Received,
			DeviationAmount: deviationAmt,
			DeviationPct:    0,
			Flagged:         false,
		}, nil
	}

	deviationPct := deviationAmt / expected
	absPct := deviationPct
	if absPct < 0 {
		absPct = -absPct
	}

	if absPct < flagThreshold {
		return Result{
			Expected:        expected,
			Received:        in.Received,
			DeviationAmount: deviationAmt,
			DeviationPct:    deviationPct,
			Flagged:         false,
		}, nil
	}

	return Result{
		Expected:        expected,
		Received:        in.Received,
		DeviationAmount: deviationAmt,
		DeviationPct:    deviationPct,
		Flagged:         true,
		Severity:        classifySeverity(absPct),
	}, nil
}

func classifySeverity(absPct float64) string {
	switch {
	case absPct > 0.50:
		return SeverityCritical
	case absPct >= flagThreshold:
		return SeverityHigh
	case absPct > 0.10:
		return SeverityMedium
	default:
		return SeverityLow
	}
}
