package rules

import "testing"

// Reference case from CLAUDE.md:
// 1000 SEK received vs ~333 SEK expected → CRITICAL flag.
// gross=1000, controlled_share=1.0 → expected = 1000 × 1.0 × 0.3333 = 333.3 SEK

func TestEvaluate(t *testing.T) {
	tests := []struct {
		name     string
		in       Input
		wantFlag bool
		wantSev  string
	}{
		{
			name: "CRITICAL overpayment — 1000 received vs 333 expected",
			in: Input{
				Gross:                     1000,
				Received:                  1000,
				ControlledManuscriptShare: 1.0,
				RightType:                 "performance",
			},
			wantFlag: true,
			wantSev:  SeverityCritical,
		},
		{
			name: "CRITICAL underpayment — received 10% of expected",
			in: Input{
				Gross:                     1000,
				Received:                  33,
				ControlledManuscriptShare: 1.0,
				RightType:                 "mechanical",
			},
			wantFlag: true,
			wantSev:  SeverityCritical,
		},
		{
			name: "HIGH underpayment — received ~50% of expected",
			in: Input{
				Gross:                     1000,
				Received:                  166.65,
				ControlledManuscriptShare: 1.0,
				RightType:                 "performance",
			},
			wantFlag: true,
			wantSev:  SeverityHigh,
		},
		{
			name: "no flag — deviation below 25% threshold",
			in: Input{
				Gross:                     1000,
				Received:                  340, // ~2% over expected 333.3
				ControlledManuscriptShare: 1.0,
				RightType:                 "performance",
			},
			wantFlag: false,
			wantSev:  "",
		},
		{
			name: "no flag — exact payment",
			in: Input{
				Gross:                     1000,
				Received:                  333.3,
				ControlledManuscriptShare: 1.0,
				RightType:                 "mechanical",
			},
			wantFlag: false,
			wantSev:  "",
		},
		{
			name: "no flag — zero controlled share",
			in: Input{
				Gross:                     1000,
				Received:                  0,
				ControlledManuscriptShare: 0,
				RightType:                 "performance",
			},
			wantFlag: false,
			wantSev:  "",
		},
		{
			name: "mechanical and performance evaluated independently",
			in: Input{
				Gross:                     500,
				Received:                  500,
				ControlledManuscriptShare: 1.0,
				RightType:                 "mechanical",
			},
			wantFlag: true,
			wantSev:  SeverityCritical,
		},
		{
			name: "partial controlled share — 50% controlled",
			in: Input{
				Gross:                     1000,
				Received:                  500,
				ControlledManuscriptShare: 0.50,
				RightType:                 "performance",
				// expected = 1000 × 0.50 × 0.3333 = 166.65
				// deviation = 500 - 166.65 = 333.35 → 200% → CRITICAL
			},
			wantFlag: true,
			wantSev:  SeverityCritical,
		},
		{
			name: "right type matching is case-insensitive",
			in: Input{
				Gross:                     1000,
				Received:                  1000,
				ControlledManuscriptShare: 1.0,
				RightType:                 "Performance",
			},
			wantFlag: true,
			wantSev:  SeverityCritical,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Evaluate(tt.in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Flagged != tt.wantFlag {
				t.Errorf("Flagged=%v want %v (expected=%.4f received=%.4f deviation_pct=%.4f)",
					got.Flagged, tt.wantFlag, got.Expected, got.Received, got.DeviationPct)
			}
			if got.Severity != tt.wantSev {
				t.Errorf("Severity=%q want %q", got.Severity, tt.wantSev)
			}
		})
	}
}

// TestSyntheticStatement runs the exact values from synthetic_statement_MEC_2025Q1.csv
// through the rules engine. Two lines must be flagged CRITICAL; three must be clean.
//
// BM003 has two writers in the CSV but parseSTIM aggregates them into one line
// with net=36.70 (18.35+18.35) and the catalogue returns controlled_share=1.0 (both writers).
func TestSyntheticStatement(t *testing.T) {
	rows := []struct {
		name                      string
		gross, received           float64
		controlledManuscriptShare float64
		wantFlag                  bool
		wantSev                   string
	}{
		// BM001 Sommarnatt: 1.0 controlled — received 15.81, expected ~16.30. ~3% gap = STIM fee. Clean.
		{"BM001 Sommarnatt", 48.90, 15.81, 1.0, false, ""},
		// BM002 Langtan: 0.5 controlled — received 7.19, expected ~7.41. ~3% gap = STIM fee. Clean.
		{"BM002 Langtan", 44.47, 7.19, 0.5, false, ""},
		// BM003 Vintervag: aggregated net=36.70, controlled=1.0, expected ~18.92. +94% overpayment.
		{"BM003 Vintervag", 56.77, 36.70, 1.0, true, SeverityCritical},
		// BM004 Drommar: 1.0 controlled — received 81.50, expected ~28.00. STIM key not applied.
		{"BM004 Drommar", 84.02, 81.50, 1.0, true, SeverityCritical},
		// BM005 Frihet: 1.0 controlled — received 6.94, expected ~7.16. ~3% gap = STIM fee. Clean.
		{"BM005 Frihet", 21.47, 6.94, 1.0, false, ""},
	}

	var flagCount int
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			got, err := Evaluate(Input{
				Gross:                     r.gross,
				Received:                  r.received,
				ControlledManuscriptShare: r.controlledManuscriptShare,
				RightType:                 "mechanical",
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Flagged != r.wantFlag {
				t.Errorf("Flagged=%v want %v (expected=%.4f received=%.4f deviation_pct=%.2f%%)",
					got.Flagged, r.wantFlag, got.Expected, got.Received, got.DeviationPct*100)
			}
			if got.Severity != r.wantSev {
				t.Errorf("Severity=%q want %q", got.Severity, r.wantSev)
			}
			if got.Flagged {
				flagCount++
			}
		})
	}

	if flagCount != 2 {
		t.Errorf("total flags=%d want 2", flagCount)
	}
}

func TestEvaluate_UnknownRightType(t *testing.T) {
	_, err := Evaluate(Input{
		Gross:                     1000,
		Received:                  500,
		ControlledManuscriptShare: 1.0,
		RightType:                 "sync",
	})
	if err == nil {
		t.Fatal("expected error for unknown right type, got nil")
	}
}
