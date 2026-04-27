// Package rules implements the STIM distribution key rule engine.
// It evaluates a single CRD statement line and returns whether the received
// publisher amount deviates materially from the expected amount.
//
// All royalty calculations use exact rational arithmetic (math/big.Rat).
// The flag decision is never made with float64.
//
// Formula (CISAC CRD 3.0 R5 / CLAUDE.md):
//
//	controlled_share = numerator / denominator          (from MIP record)
//	observed_ratio   = net_cents / gross_cents          (scale-independent)
//	expected_ratio   = (1/3) × controlled_share         (STIM key, written in stone)
//	deviation        = observed_ratio − expected_ratio  (signed)
//
// Flagged when abs(deviation / expected_ratio) > 1/100_000.
// Severity is based on ratio_excess = observed_ratio / expected_ratio.
//
// V0.1 scope: STIM Sweden, MEC detection only. Distribution key is 1/3 for both
// MEC and PERF. PERF detection is Phase 2 and must not be built here.
package rules

import (
	"fmt"
	"math/big"
)

// Severity constants. POSSIBLE replaces LOW in the CRD-based severity model.
// LOW is retained for backward compatibility with recommendations.go.
const (
	SeverityPossible = "POSSIBLE"  // ratio_excess < 1.1  (may be rounding)
	SeverityMedium   = "MEDIUM"    // 1.1 ≤ ratio_excess < 1.5
	SeverityHigh     = "HIGH"      // 1.5 ≤ ratio_excess < 2.5
	SeverityCritical = "CRITICAL"  // ratio_excess ≥ 2.5  (territorial override pattern)
	SeverityLow      = "LOW"       // kept for backward compatibility
)

// stimKey is the STIM publisher distribution key: 1/3.
// Applies to both MEC and PERF for Nordic publishers. Written in stone for V0.1.
var stimKey = new(big.Rat).SetFrac64(1, 3)

// flagThreshold: a line is flagged when abs(deviation / expected_ratio) exceeds this.
// 1/100_000 = 0.00001 — catches any non-trivial error; tolerates only rounding dust.
var flagThreshold = new(big.Rat).SetFrac64(1, 100_000)

// Input carries the exact integer amounts for one detection evaluation.
// Amounts are in cent units (2 implied decimal places for SEK):
// GrossCents=372000 represents 3720.00 SEK.
type Input struct {
	GrossCents            int64
	NetCents              int64
	ControlledNumerator   int64
	ControlledDenominator int64
}

// Result is the outcome of evaluating one line.
//
// The Flagged and Severity fields are determined by exact rational arithmetic.
// Expected, Received, DeviationAmount, and DeviationPct are float64 approximations
// for display and AI explanation only — they must not be used in any calculation.
type Result struct {
	Flagged  bool
	Severity string
	// Display-only float64 values. Do not use in royalty calculations.
	Expected        float64 // expected net in SEK
	Received        float64 // received net in SEK
	DeviationAmount float64 // received − expected, signed
	DeviationPct    float64 // deviation / expected, signed
}

// Evaluate applies the STIM distribution key formula to a single CRD line.
// Returns an error if controlled_denominator is zero or gross_cents ≤ 0.
func Evaluate(in Input) (Result, error) {
	if in.ControlledDenominator == 0 {
		return Result{}, fmt.Errorf("controlled_denominator is zero — cannot evaluate share")
	}
	if in.GrossCents <= 0 {
		return Result{}, fmt.Errorf("gross_cents must be positive, got %d", in.GrossCents)
	}

	// --- Exact rational arithmetic ---

	cs := new(big.Rat).SetFrac64(in.ControlledNumerator, in.ControlledDenominator)

	// observed = NetCents / GrossCents — the cent unit cancels, giving a pure ratio.
	observed := new(big.Rat).SetFrac64(in.NetCents, in.GrossCents)

	// expected = (1/3) × controlled_share
	expected := new(big.Rat).Mul(stimKey, cs)

	// --- Display values (float64, computed from exact rationals) ---

	hundred := new(big.Rat).SetInt64(100)
	grossRat := new(big.Rat).SetInt64(in.GrossCents)

	// expectedNetSEK = GrossCents × expected_ratio / 100
	expectedNetRat := new(big.Rat).Quo(new(big.Rat).Mul(grossRat, expected), hundred)
	expectedSEK, _ := expectedNetRat.Float64()
	receivedSEK, _ := new(big.Rat).SetFrac64(in.NetCents, 100).Float64()
	deviationSEK := receivedSEK - expectedSEK
	deviationPct := 0.0
	if expectedSEK != 0 {
		deviationPct = deviationSEK / expectedSEK
	}

	// Zero expected (zero controlled share) — no flag, no error.
	if expected.Sign() == 0 {
		return Result{
			Expected:        expectedSEK,
			Received:        receivedSEK,
			DeviationAmount: deviationSEK,
			DeviationPct:    deviationPct,
		}, nil
	}

	// deviation = observed − expected
	deviation := new(big.Rat).Sub(observed, expected)

	// relDev = abs(deviation) / expected
	relDev := new(big.Rat).Quo(new(big.Rat).Abs(deviation), expected)

	if relDev.Cmp(flagThreshold) <= 0 {
		return Result{
			Flagged:         false,
			Expected:        expectedSEK,
			Received:        receivedSEK,
			DeviationAmount: deviationSEK,
			DeviationPct:    deviationPct,
		}, nil
	}

	// ratio_excess = observed / expected — determines severity.
	ratioExcess := new(big.Rat).Quo(observed, expected)

	return Result{
		Flagged:         true,
		Severity:        classifySeverity(ratioExcess),
		Expected:        expectedSEK,
		Received:        receivedSEK,
		DeviationAmount: deviationSEK,
		DeviationPct:    deviationPct,
	}, nil
}

// classifySeverity maps ratio_excess (observed / expected) to a severity string.
// Designed for the overpayment detection pattern (ratio > 1).
// Underpayments have ratio_excess < 1 and resolve to POSSIBLE.
func classifySeverity(ratioExcess *big.Rat) string {
	two5 := new(big.Rat).SetFrac64(5, 2)   // 2.5
	one5 := new(big.Rat).SetFrac64(3, 2)   // 1.5
	one1 := new(big.Rat).SetFrac64(11, 10) // 1.1

	switch {
	case ratioExcess.Cmp(two5) >= 0:
		return SeverityCritical
	case ratioExcess.Cmp(one5) >= 0:
		return SeverityHigh
	case ratioExcess.Cmp(one1) >= 0:
		return SeverityMedium
	default:
		return SeverityPossible
	}
}
