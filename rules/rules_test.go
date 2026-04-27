package rules

import "testing"

// --- Canonical fixture tests (CLAUDE.md source of truth) ---
//
// SOMMARNATT: gross=372000 cents (3720.00 SEK), net=124000 cents (1240.00 SEK)
//   observed = 124000/372000 = 1/3 (exact)
//   expected = 1/3 × 10000/10000 = 1/3
//   deviation = 0 → CLEAN
//
// DROMMAR: gross=102600 cents (1026.00 SEK), net=102600 cents (1026.00 SEK)
//   observed = 102600/102600 = 1
//   expected = 1/3 × 1 = 1/3
//   deviation = 2/3, relDev = 2 >> 1/100000 → FLAG
//   ratio_excess = 3 ≥ 2.5 → CRITICAL
//   overpayment = 1026.00 - 342.00 = 684.00 SEK

func TestEvaluate_SOMMARNATT_Clean(t *testing.T) {
	got, err := Evaluate(Input{
		GrossCents:            372000,
		NetCents:              124000,
		ControlledNumerator:   10000,
		ControlledDenominator: 10000,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Flagged {
		t.Errorf("SOMMARNATT: want CLEAN, got flagged (deviation=%.10f, severity=%s)",
			got.DeviationAmount, got.Severity)
	}
}

func TestEvaluate_DROMMAR_Critical(t *testing.T) {
	got, err := Evaluate(Input{
		GrossCents:            102600,
		NetCents:              102600,
		ControlledNumerator:   10000,
		ControlledDenominator: 10000,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.Flagged {
		t.Fatalf("DROMMAR: want CRITICAL, got CLEAN (deviation=%.10f)", got.DeviationAmount)
	}
	if got.Severity != SeverityCritical {
		t.Errorf("DROMMAR: severity=%q want CRITICAL", got.Severity)
	}

	// Overpayment = received − expected = 1026.00 − 342.00 = 684.00 SEK
	const wantOverpayment = 684.00
	if got.DeviationAmount < wantOverpayment-0.005 || got.DeviationAmount > wantOverpayment+0.005 {
		t.Errorf("DROMMAR: overpayment=%.2f want %.2f SEK", got.DeviationAmount, wantOverpayment)
	}
}

// --- Severity boundary tests ---

func TestEvaluate_Severity(t *testing.T) {
	tests := []struct {
		name        string
		in          Input
		wantFlagged bool
		wantSev     string
	}{
		{
			// ratio_excess = 3 ≥ 2.5 → CRITICAL
			name: "CRITICAL: ratio 1/1 vs expected 1/3 (full override)",
			in: Input{
				GrossCents:            102600,
				NetCents:              102600,
				ControlledNumerator:   10000,
				ControlledDenominator: 10000,
			},
			wantFlagged: true,
			wantSev:     SeverityCritical,
		},
		{
			// observed=0.5/1=0.5, expected=1/3, ratio_excess=1.5 — boundary HIGH
			name: "HIGH: ratio_excess exactly 1.5",
			in: Input{
				GrossCents:            100,
				NetCents:              50,
				ControlledNumerator:   10000,
				ControlledDenominator: 10000,
			},
			wantFlagged: true,
			wantSev:     SeverityHigh,
		},
		{
			// observed=0.4, expected=1/3, ratio_excess=1.2 → MEDIUM
			name: "MEDIUM: ratio_excess 1.2",
			in: Input{
				GrossCents:            100,
				NetCents:              40,
				ControlledNumerator:   10000,
				ControlledDenominator: 10000,
			},
			wantFlagged: true,
			wantSev:     SeverityMedium,
		},
		{
			// observed=0.36, expected=1/3, ratio_excess≈1.08 < 1.1 → POSSIBLE
			name: "POSSIBLE: ratio_excess just under 1.1",
			in: Input{
				GrossCents:            10000,
				NetCents:              3600,
				ControlledNumerator:   10000,
				ControlledDenominator: 10000,
			},
			wantFlagged: true,
			wantSev:     SeverityPossible,
		},
		{
			// exact payment: observed = expected → CLEAN
			name: "CLEAN: exact STIM payment",
			in: Input{
				GrossCents:            372000,
				NetCents:              124000,
				ControlledNumerator:   10000,
				ControlledDenominator: 10000,
			},
			wantFlagged: false,
			wantSev:     "",
		},
		{
			// partial controlled share: 50%
			// expected = 1/3 × 0.5 = 1/6
			// observed = 12400/37200 = 1/3, ratio_excess = 2 → HIGH (1.5 ≤ 2 < 2.5)
			name: "HIGH: 50% controlled share, ratio_excess 2",
			in: Input{
				GrossCents:            37200,
				NetCents:              12400,
				ControlledNumerator:   5000,
				ControlledDenominator: 10000,
			},
			wantFlagged: true,
			wantSev:     SeverityHigh,
		},
		{
			// zero controlled share → no flag
			name: "CLEAN: zero controlled share",
			in: Input{
				GrossCents:            100000,
				NetCents:              0,
				ControlledNumerator:   0,
				ControlledDenominator: 10000,
			},
			wantFlagged: false,
			wantSev:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Evaluate(tt.in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Flagged != tt.wantFlagged {
				t.Errorf("Flagged=%v want %v (expected=%.4f received=%.4f deviationPct=%.4f)",
					got.Flagged, tt.wantFlagged, got.Expected, got.Received, got.DeviationPct)
			}
			if got.Severity != tt.wantSev {
				t.Errorf("Severity=%q want %q", got.Severity, tt.wantSev)
			}
		})
	}
}

// --- Error cases ---

func TestEvaluate_ZeroDenominator(t *testing.T) {
	_, err := Evaluate(Input{
		GrossCents:            100000,
		NetCents:              33333,
		ControlledNumerator:   10000,
		ControlledDenominator: 0, // invalid
	})
	if err == nil {
		t.Fatal("expected error for zero denominator, got nil")
	}
}

func TestEvaluate_ZeroGross(t *testing.T) {
	_, err := Evaluate(Input{
		GrossCents:            0, // invalid
		NetCents:              0,
		ControlledNumerator:   10000,
		ControlledDenominator: 10000,
	})
	if err == nil {
		t.Fatal("expected error for zero gross, got nil")
	}
}
