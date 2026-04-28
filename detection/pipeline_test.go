// Package detection pipeline_test verifies the full crd→rules chain using
// known fixtures. These tests catch regressions across the critical path without
// requiring a running database or AI service.
package detection

import (
	"os"
	"testing"

	"encore.app/crd"
	"encore.app/rules"
)

// TestPipeline_CanonicalFixture runs the two CLAUDE.md canonical works
// (SOMMARNATT=CLEAN, DROMMAR=CRITICAL) through the parser and evaluator.
// This is the definition-of-done test for the detection pipeline.
func TestPipeline_CanonicalFixture(t *testing.T) {
	f, err := os.Open("../testdata/verostark_edge_cases.crd")
	if err != nil {
		t.Fatalf("could not open fixture: %v", err)
	}
	defer f.Close()

	lines, period, parseErrs := crd.ParseFile(f)
	if len(parseErrs) != 0 {
		t.Errorf("want 0 parse errors, got %d: %v", len(parseErrs), parseErrs)
	}
	if period != "2025-Q1" {
		t.Errorf("period = %q, want 2025-Q1", period)
	}
	if len(lines) != 5 {
		t.Fatalf("want 5 WER lines, got %d", len(lines))
	}

	type wantRow struct {
		title    string
		flagged  bool
		severity string
	}
	want := []wantRow{
		{"ÅSKAN ÖVER FJÄLLEN", false, ""},        // Work 1: Swedish chars, CLEAN
		{"EMPTY FIELDS TEST", false, ""},          // Work 2: space-padded fields, CLEAN
		{"RECOVERY AFTER ADJ", true, "CRITICAL"},  // Work 3: after ADJ record, CRITICAL
		{"RECOVERY AFTER FEO", false, ""},         // Work 4: after FEO record, CLEAN
		{"ICC INSIDE BLOCK", true, "CRITICAL"},    // Work 5: ICC ignored, CRITICAL
	}

	for i, l := range lines {
		result, err := rules.Evaluate(rules.Input{
			GrossCents:            l.GrossCents,
			NetCents:              l.NetCents,
			ControlledNumerator:   l.ControlledNumerator,
			ControlledDenominator: l.ControlledDenominator,
		})
		if err != nil {
			t.Errorf("work %d %q: evaluate error: %v", i+1, l.WorkTitle, err)
			continue
		}
		w := want[i]
		if l.WorkTitle != w.title {
			t.Errorf("work %d: title=%q want %q", i+1, l.WorkTitle, w.title)
		}
		if result.Flagged != w.flagged {
			t.Errorf("work %d %q: flagged=%v want %v", i+1, l.WorkTitle, result.Flagged, w.flagged)
		}
		if result.Flagged && result.Severity != w.severity {
			t.Errorf("work %d %q: severity=%q want %q", i+1, l.WorkTitle, result.Severity, w.severity)
		}
	}
}

// TestPipeline_InlineCanonical tests the SOMMARNATT/DROMMAR numbers from
// CLAUDE.md directly without a file — guards against rules regressions.
func TestPipeline_InlineCanonical(t *testing.T) {
	cases := []struct {
		name     string
		gross    int64
		net      int64
		flagged  bool
		severity string
		overpay  float64 // SEK, only checked when flagged
	}{
		// SOMMARNATT: gross=3720.00 SEK, net=1240.00 SEK, share=1/1 → ratio 1/3 = CLEAN
		{"SOMMARNATT", 372000, 124000, false, "", 0},
		// DROMMAR: gross=1026.00 SEK, net=1026.00 SEK, share=1/1 → ratio 1/1 = CRITICAL, overpay=684.00 SEK
		{"DROMMAR", 102600, 102600, true, "CRITICAL", 684.00},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result, err := rules.Evaluate(rules.Input{
				GrossCents:            c.gross,
				NetCents:              c.net,
				ControlledNumerator:   10000,
				ControlledDenominator: 10000,
			})
			if err != nil {
				t.Fatalf("evaluate error: %v", err)
			}
			if result.Flagged != c.flagged {
				t.Errorf("flagged=%v want %v", result.Flagged, c.flagged)
			}
			if !c.flagged {
				return
			}
			if result.Severity != c.severity {
				t.Errorf("severity=%q want %q", result.Severity, c.severity)
			}
			if result.DeviationAmount != c.overpay {
				t.Errorf("overpayment=%.2f SEK want %.2f SEK", result.DeviationAmount, c.overpay)
			}
		})
	}
}

// TestPipeline_NoiseRecords verifies that ADJ, FEO, WBI, WUI, SIN, SID, ICC
// records do not produce WER lines and do not cause parse errors.
func TestPipeline_NoiseRecords(t *testing.T) {
	f, err := os.Open("../testdata/verostark_stress_1500_works.crd")
	if err != nil {
		t.Fatalf("could not open fixture: %v", err)
	}
	defer f.Close()

	lines, _, parseErrs := crd.ParseFile(f)
	if len(parseErrs) != 0 {
		t.Errorf("want 0 parse errors in 1500-work file, got %d", len(parseErrs))
	}
	if len(lines) != 1954 {
		t.Errorf("want 1954 WER lines, got %d", len(lines))
	}

	// Every line must evaluate without error.
	for i, l := range lines {
		if _, err := rules.Evaluate(rules.Input{
			GrossCents:            l.GrossCents,
			NetCents:              l.NetCents,
			ControlledNumerator:   l.ControlledNumerator,
			ControlledDenominator: l.ControlledDenominator,
		}); err != nil {
			t.Errorf("line %d %q: evaluate error: %v", i+1, l.WorkTitle, err)
		}
	}
}
