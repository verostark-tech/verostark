package statements

import (
	"strings"
	"testing"
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
	wants := []float64{48.90, 44.47, 56.77, 84.02, 21.47}
	for i, l := range lines {
		if l.GrossAmount == nil {
			t.Errorf("line %d: GrossAmount is nil", i)
			continue
		}
		if *l.GrossAmount != wants[i] {
			t.Errorf("line %d: GrossAmount=%.2f want %.2f", i, *l.GrossAmount, wants[i])
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
// work (BM003) are collapsed into one line with net amounts summed.
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
	const wantNet = 36.70
	if bm003.NetAmount != wantNet {
		t.Errorf("BM003 NetAmount=%.2f want %.2f", bm003.NetAmount, wantNet)
	}
}
