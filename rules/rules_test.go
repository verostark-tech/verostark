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
