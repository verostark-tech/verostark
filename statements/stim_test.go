package statements

import (
	"math"
	"strings"
	"testing"

	"encore.app/rules"
)

// syntheticCSV is a copy of synthetic_statement_MEC_2025Q1.csv.
// Replace with a real STIM statement when one becomes available.
const syntheticCSV = `Work ID,Title,Source,Right Type,Gross,ISWC,Controlled by Publisher (%),Interested Party,Role,Manuscript Share (%),Amount before fee,Fee (%),Fee Amount,Net Amount
BM001,Sommarnatt,Internet,M,48.90,T-000.000.001-0,1.0,Lindqvist Erik [00234567890],Composer&Lyricist,1.0,48.90,0.03,1.47,15.81
BM002,Langtan,Internet,M,44.47,T-000.000.002-1,0.5,Holm Marcus [00456789012],Composer&Lyricist,1.0,44.47,0.03,1.33,7.19
BM003,Vintervag,Internet,M,56.77,T-000.000.003-2,1.0,Strand Anna [00567890123],Composer&Lyricist,0.5,56.77,0.03,1.70,18.35
BM003,Vintervag,Internet,M,56.77,T-000.000.003-2,1.0,Bjork Sara [00345678901],Lyricist,0.5,56.77,0.03,1.70,18.35
BM004,Drommar,Internet,M,84.02,T-000.000.004-3,1.0,Lindqvist Erik [00234567890],Composer&Lyricist,1.0,84.02,0.03,2.52,81.50
BM005,Frihet,Internet,M,21.47,T-000.000.005-4,1.0,Bjork Sara [00345678901],Composer&Lyricist,1.0,21.47,0.03,0.64,6.94
`

func TestParseSTIM_LineCount(t *testing.T) {
	lines, err := parseSTIM(strings.NewReader(syntheticCSV))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(lines) != 5 {
		t.Errorf("got %d lines, want 5", len(lines))
	}
}

func TestParseSTIM_RightTypeMappedToMechanical(t *testing.T) {
	lines, err := parseSTIM(strings.NewReader(syntheticCSV))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for i, l := range lines {
		if l.RightType != "mechanical" {
			t.Errorf("line %d: got right_type=%q, want \"mechanical\"", i, l.RightType)
		}
	}
}

func TestParseSTIM_GrossAmountsPopulated(t *testing.T) {
	lines, err := parseSTIM(strings.NewReader(syntheticCSV))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wants := []float64{48.90, 44.47, 113.54, 84.02, 21.47}
	for i, l := range lines {
		if l.GrossAmount != wants[i] {
			t.Errorf("line %d: GrossAmount=%.2f want %.2f", i, l.GrossAmount, wants[i])
		}
	}
}

func TestParseSTIM_NetAmounts(t *testing.T) {
	lines, err := parseSTIM(strings.NewReader(syntheticCSV))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wants := []float64{15.81, 7.19, 36.70, 81.50, 6.94}
	for i, l := range lines {
		if l.NetAmount != wants[i] {
			t.Errorf("line %d: NetAmount=%.2f want %.2f", i, l.NetAmount, wants[i])
		}
	}
}

func TestParseSTIM_WorkRefs(t *testing.T) {
	lines, err := parseSTIM(strings.NewReader(syntheticCSV))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wants := []string{"BM001", "BM002", "BM003", "BM004", "BM005"}
	for i, l := range lines {
		if l.WorkRef != wants[i] {
			t.Errorf("line %d: WorkRef=%q want %q", i, l.WorkRef, wants[i])
		}
	}
}

func TestParseSTIM_WorkTitles(t *testing.T) {
	lines, err := parseSTIM(strings.NewReader(syntheticCSV))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wants := []string{"Sommarnatt", "Langtan", "Vintervag", "Drommar", "Frihet"}
	for i, l := range lines {
		if l.WorkTitle != wants[i] {
			t.Errorf("line %d: WorkTitle=%q want %q", i, l.WorkTitle, wants[i])
		}
	}
}

func TestParseSTIM_ControlledShares(t *testing.T) {
	// Values in the CSV are 0-1 decimals (1.0 = 100%, 0.5 = 50%).
	// controlled_share = controlled_by_publisher × manuscript_share.
	//
	// BM001: 1.0 × 1.0 = 1.0 (sole author, fully controlled)
	// BM002: 0.5 × 1.0 = 0.5 (sole author, 50% controlled)
	// BM003: (1.0 × 0.5) + (1.0 × 0.5) = 1.0 (two authors, each 50%, both controlled)
	// BM004: 1.0 × 1.0 = 1.0 (sole author, fully controlled)
	// BM005: 1.0 × 1.0 = 1.0 (sole author, fully controlled)
	lines, err := parseSTIM(strings.NewReader(syntheticCSV))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wants := []float64{1.0, 0.5, 1.0, 1.0, 1.0}
	for i, l := range lines {
		if math.Abs(l.ControlledShare-wants[i]) > 1e-9 {
			t.Errorf("line %d (%s): ControlledShare=%.6f want %.6f",
				i, l.WorkRef, l.ControlledShare, wants[i])
		}
	}
}

func TestParseSTIM_EmptyInput(t *testing.T) {
	_, err := parseSTIM(strings.NewReader(""))
	if err == nil {
		t.Fatal("expected error for empty input, got nil")
	}
}

func TestParseSTIM_MissingRequiredColumn(t *testing.T) {
	bad := "Work ID,Title,Net Amount\nBM001,Sommarnatt,15.81\n"
	_, err := parseSTIM(strings.NewReader(bad))
	if err == nil {
		t.Fatal("expected error for missing Gross column, got nil")
	}
}

// TestParseSTIM_AggregatesMultiWriterWork verifies that two CSV rows for the same
// work (BM003) are collapsed into one line with net amounts and controlled shares summed.
func TestParseSTIM_AggregatesMultiWriterWork(t *testing.T) {
	lines, err := parseSTIM(strings.NewReader(syntheticCSV))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var bm003 *StatementLine
	for i := range lines {
		if lines[i].WorkRef == "BM003" {
			if bm003 != nil {
				t.Fatal("BM003 appears more than once — aggregation failed")
			}
			bm003 = &lines[i]
		}
	}
	if bm003 == nil {
		t.Fatal("BM003 not found in parsed lines")
	}
	if bm003.NetAmount != 36.70 {
		t.Errorf("BM003 NetAmount=%.2f want 36.70", bm003.NetAmount)
	}
	if math.Abs(bm003.ControlledShare-1.0) > 1e-9 {
		t.Errorf("BM003 ControlledShare=%.6f want 1.0 (0.5+0.5 across two writers)",
			bm003.ControlledShare)
	}
}

// TestDetectionPipeline_SyntheticCSV is the end-to-end detection pipeline test.
// It feeds the synthetic CSV through parseSTIM and rules.Evaluate to verify
// exactly which works are flagged and why — without a running server or database.
//
// Expected results from first-principles calculation:
//   - BM001: expected=16.30 received=15.81 dev=-3.0%  → not flagged (below 25%)
//   - BM002: expected= 7.41 received= 7.19 dev=-3.0%  → not flagged
//   - BM003: expected=18.92 received=36.70 dev=+94.0% → CRITICAL overpayment
//   - BM004: expected=28.01 received=81.50 dev=+191%  → CRITICAL overpayment
//   - BM005: expected= 7.16 received= 6.94 dev=-3.1%  → not flagged
func TestDetectionPipeline_SyntheticCSV(t *testing.T) {
	lines, err := parseSTIM(strings.NewReader(syntheticCSV))
	if err != nil {
		t.Fatalf("parseSTIM: %v", err)
	}

	type wantFlag struct {
		workRef     string
		patternType string
		severity    string
	}

	var got []wantFlag
	for _, l := range lines {
		if l.GrossAmount == 0 || l.ControlledShare == 0 {
			continue
		}
		result, err := rules.Evaluate(rules.Input{
			Gross:                     l.GrossAmount,
			Received:                  l.NetAmount,
			ControlledManuscriptShare: l.ControlledShare,
			RightType:                 l.RightType,
		})
		if err != nil || !result.Flagged {
			continue
		}
		pt := "overpayment"
		if result.DeviationAmount < 0 {
			pt = "underpayment"
		}
		got = append(got, wantFlag{l.WorkRef, pt, result.Severity})
	}

	want := []wantFlag{
		{"BM004", "overpayment", rules.SeverityCritical},
	}

	if len(got) != len(want) {
		t.Fatalf("got %d flags, want %d\nflags: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("flag[%d]: got %+v, want %+v", i, got[i], w)
		}
	}
}
