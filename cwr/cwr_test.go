package cwr

import (
	"strings"
	"testing"
)

// padRight right-pads s with spaces to exactly n chars.
func padRight(s string, n int) string {
	if len(s) >= n {
		return s[:n]
	}
	return s + strings.Repeat(" ", n-len(s))
}

// buildNWR constructs a synthetic CWR NWR line from key fields.
// Field layout follows CWR 2.1 spec (0-indexed):
//   [0:3]    record type
//   [3:11]   transaction seq
//   [11:19]  record seq
//   [19:79]  title (60)
//   [79:81]  language code
//   [81:95]  submitter work # (14)
//   [95:106] ISWC (11)
func buildNWR(txSeq, title, submitterRef, iswc string) string {
	line := "NWR" + padRight(txSeq, 8) + "00000000" +
		padRight(title, 60) +
		"EN" +
		padRight(submitterRef, 14) +
		padRight(iswc, 11)
	return padRight(line, 240)
}

// buildSWR constructs a synthetic CWR SWR line from key fields.
// Field layout follows CWR 2.1 spec (0-indexed):
//   [3:11]   transaction seq
//   [28:73]  last name (45)
//   [73:103] first name (30)
//   [104:106] designation code (2)
//   [116:127] IPI name (11)
//   [130:135] PR ownership share (5)
//   [154:167] IPI base (13)
func buildSWR(txSeq, lastName, firstName, desig, ipiName, prShare, ipiBase string) string {
	line := "SWR" + padRight(txSeq, 8) + "00000001" +
		strings.Repeat(" ", 9) + // interested party # [19:28]
		padRight(lastName, 45) + // [28:73]
		padRight(firstName, 30) + // [73:103]
		" " + // unknown indicator [103]
		padRight(desig, 2) + // [104:106]
		strings.Repeat(" ", 10) + // tax ID [106:116]
		padRight(ipiName, 11) + // [116:127]
		"021" + // PR society [127:130]
		padRight(prShare, 5) + // [130:135]
		"   " + strings.Repeat(" ", 5) + // MR society + share [135:143]
		"   " + strings.Repeat(" ", 5) + // SR society + share [143:151]
		"   " + // reversionary, first recording refusal, filler [151:154]
		padRight(ipiBase, 13) // IPI base [154:167]
	return padRight(line, 200)
}

func TestParseFile_BasicWork(t *testing.T) {
	nwr := buildNWR("00000001", "HAPPY BIRTHDAY TO YOU", "HB001", "T0102690505")
	swr := buildSWR("00000001", "HILL", "MILDRED", "CA", "00000000000", "05000", "I-000000000-8")

	records, errs := ParseFile(strings.NewReader(nwr + "\n" + swr))
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 WorkRecord, got %d", len(records))
	}

	w := records[0].Work
	if w.Title != "HAPPY BIRTHDAY TO YOU" {
		t.Errorf("Title = %q, want %q", w.Title, "HAPPY BIRTHDAY TO YOU")
	}
	if w.ISWC != "T0102690505" {
		t.Errorf("ISWC = %q, want %q", w.ISWC, "T0102690505")
	}
	if w.SubmitterRef != "HB001" {
		t.Errorf("SubmitterRef = %q, want %q", w.SubmitterRef, "HB001")
	}

	if len(records[0].Writers) != 1 {
		t.Fatalf("expected 1 Writer, got %d", len(records[0].Writers))
	}
	wr := records[0].Writers[0]
	if wr.LastName != "HILL" {
		t.Errorf("LastName = %q, want %q", wr.LastName, "HILL")
	}
	if wr.FirstName != "MILDRED" {
		t.Errorf("FirstName = %q, want %q", wr.FirstName, "MILDRED")
	}
	if wr.DesignationCode != "CA" {
		t.Errorf("DesignationCode = %q, want %q", wr.DesignationCode, "CA")
	}
	if wr.IPIName != "00000000000" {
		t.Errorf("IPIName = %q, want %q", wr.IPIName, "00000000000")
	}
	if wr.ManuscriptShare != 0.5 {
		t.Errorf("ManuscriptShare = %v, want 0.5", wr.ManuscriptShare)
	}
	if wr.IPIBase != "I-000000000-8" {
		t.Errorf("IPIBase = %q, want %q", wr.IPIBase, "I-000000000-8")
	}
}

func TestParseFile_MultipleWorks(t *testing.T) {
	lines := strings.Join([]string{
		buildNWR("00000001", "WORK ONE", "W001", "T0102690505"),
		buildSWR("00000001", "SMITH", "JOHN", "C", "00000000000", "10000", ""),
		buildNWR("00000002", "WORK TWO", "W002", ""),
		buildSWR("00000002", "JONES", "ALICE", "CA", "00000000000", "05000", ""),
		buildSWR("00000002", "JONES", "BOB", "A", "00000000199", "05000", ""),
	}, "\n")

	records, errs := ParseFile(strings.NewReader(lines))
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 WorkRecords, got %d", len(records))
	}
	if len(records[0].Writers) != 1 {
		t.Errorf("work 1: expected 1 writer, got %d", len(records[0].Writers))
	}
	if len(records[1].Writers) != 2 {
		t.Errorf("work 2: expected 2 writers, got %d", len(records[1].Writers))
	}
	if records[0].Writers[0].ManuscriptShare != 1.0 {
		t.Errorf("full share: got %v, want 1.0", records[0].Writers[0].ManuscriptShare)
	}
}

func TestParseFile_SkipsNonNWRSWR(t *testing.T) {
	hdr := padRight("HDR", 203)
	nwr := buildNWR("00000001", "ONLY WORK", "OW001", "T0102690505")

	records, errs := ParseFile(strings.NewReader(hdr + "\n" + nwr))
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(records) != 1 {
		t.Errorf("expected 1 record, got %d", len(records))
	}
}

func TestParseFile_ShortNWRLineReturnsError(t *testing.T) {
	_, errs := ParseFile(strings.NewReader("NWR00000001"))
	if len(errs) == 0 {
		t.Error("expected error for short NWR line, got none")
	}
}

func TestParseShare(t *testing.T) {
	tests := []struct {
		input string
		want  float64
	}{
		{"05000", 0.5},
		{"10000", 1.0},
		{"00000", 0.0},
		{"03333", 0.3333},
		{"     ", 0.0},
		{"", 0.0},
	}
	for _, tt := range tests {
		got := parseShare(tt.input)
		if got != tt.want {
			t.Errorf("parseShare(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}
