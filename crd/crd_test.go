package crd

import (
	"fmt"
	"strings"
	"testing"
)

// --- Fixture builders ---
// All positions are 1-indexed per CISAC CRD 3.0 R5.

// buildSDN creates a CRD SDN record.
// amount_decimals at pos 113, size 1.
// Minimum length: 122 chars (currency at pos 120, size 3).
func buildSDN(amountDecimals int, currency string) string {
	b := []byte(strings.Repeat(" ", 122))
	copy(b[0:3], "SDN")
	copy(b[112:113], fmt.Sprintf("%d", amountDecimals)) // pos 113 size 1
	copy(b[119:122], currency)                           // pos 120 size 3
	return string(b)
}

// buildMWN creates a CRD MWN record.
//   - work_ref  at pos 20  size 14
//   - work_title at pos 34  size 60
//   - iswc       at pos 154 size 11
//
// Minimum length: 164 chars.
func buildMWN(workRef, workTitle, iswc string) string {
	b := []byte(strings.Repeat(" ", 164))
	copy(b[0:3], "MWN")
	copy(b[19:33], fmt.Sprintf("%-14s", workRef))   // pos 20 size 14
	copy(b[33:93], fmt.Sprintf("%-60s", workTitle)) // pos 34 size 60
	copy(b[153:164], fmt.Sprintf("%-11s", iswc))    // pos 154 size 11
	return string(b)
}

// buildMDR creates a CRD MDR record.
//   - right_code     at pos 20 size 2
//   - right_category at pos 22 size 3
//
// Minimum length: 24 chars.
func buildMDR(rightCode, rightCategory string) string {
	b := []byte(strings.Repeat(" ", 24))
	copy(b[0:3], "MDR")
	copy(b[19:21], rightCode)     // pos 20 size 2
	copy(b[21:24], rightCategory) // pos 22 size 3
	return string(b)
}

// buildMIP creates a CRD MIP record.
//   - numerator   at pos 36 size 5
//   - denominator at pos 41 size 5
//
// Minimum length: 45 chars.
func buildMIP(numerator, denominator int64) string {
	b := []byte(strings.Repeat(" ", 45))
	copy(b[0:3], "MIP")
	copy(b[35:40], fmt.Sprintf("%05d", numerator))   // pos 36 size 5
	copy(b[40:45], fmt.Sprintf("%05d", denominator)) // pos 41 size 5
	return string(b)
}

// buildWER creates a CRD WER record (367 chars).
//   - distCat       at pos 35  size 2
//   - currency      at pos 246 size 3
//   - grossCents    at pos 249 size 18  (2 implied decimal places)
//   - netCents      at pos 350 size 18
func buildWER(distCat, currency string, grossCents, netCents int64) string {
	b := []byte(strings.Repeat(" ", 367))
	copy(b[0:3], "WER")
	copy(b[34:36], distCat)                             // pos 35 size 2
	copy(b[245:248], currency)                           // pos 246 size 3
	copy(b[248:266], fmt.Sprintf("%018d", grossCents))  // pos 249 size 18
	copy(b[349:367], fmt.Sprintf("%018d", netCents))    // pos 350 size 18
	return string(b)
}

// canonicalFixture is the synthetic test file from CLAUDE.md.
//
// SOMMARNATT (CLEAN):
//   gross=372000 cents (3720.00 SEK), net=124000 cents (1240.00 SEK)
//   observed=1/3, expected=1/3×1=1/3, deviation=0
//
// DROMMAR (CRITICAL — planted deviation):
//   gross=102600 cents (1026.00 SEK), net=102600 cents (1026.00 SEK)
//   observed=1, expected=1/3, deviation=2/3, overpayment=684.00 SEK
var canonicalFixture = strings.Join([]string{
	buildSDN(2, "SEK"),
	buildMWN("STIM20190442", "SOMMARNATT", "T1234560013"),
	buildMDR("MD", "MEC"),
	buildMIP(10000, 10000),
	buildWER("ME", "SEK", 372000, 124000),
	buildMWN("STIM20180331", "DROMMAR", "T1234560042"),
	buildMDR("MD", "MEC"),
	buildMIP(10000, 10000),
	buildWER("ME", "SEK", 102600, 102600),
}, "\n")

// --- Parser tests ---

func TestParseFile_LineCount(t *testing.T) {
	lines, _, _ := ParseFile(strings.NewReader(canonicalFixture))
	if len(lines) != 2 {
		t.Fatalf("got %d WER lines, want 2", len(lines))
	}
}

func TestParseFile_WorkRefs(t *testing.T) {
	lines, _, _ := ParseFile(strings.NewReader(canonicalFixture))
	if lines[0].WorkRef != "STIM20190442" {
		t.Errorf("lines[0].WorkRef=%q want STIM20190442", lines[0].WorkRef)
	}
	if lines[1].WorkRef != "STIM20180331" {
		t.Errorf("lines[1].WorkRef=%q want STIM20180331", lines[1].WorkRef)
	}
}

func TestParseFile_WorkTitles(t *testing.T) {
	lines, _, _ := ParseFile(strings.NewReader(canonicalFixture))
	if lines[0].WorkTitle != "SOMMARNATT" {
		t.Errorf("lines[0].WorkTitle=%q want SOMMARNATT", lines[0].WorkTitle)
	}
	if lines[1].WorkTitle != "DROMMAR" {
		t.Errorf("lines[1].WorkTitle=%q want DROMMAR", lines[1].WorkTitle)
	}
}

func TestParseFile_GrossAmounts(t *testing.T) {
	// CLAUDE.md: SOMMARNATT gross=372000 (3720.00 SEK), DROMMAR gross=102600 (1026.00 SEK).
	lines, _, _ := ParseFile(strings.NewReader(canonicalFixture))
	if lines[0].GrossCents != 372000 {
		t.Errorf("SOMMARNATT GrossCents=%d want 372000", lines[0].GrossCents)
	}
	if lines[1].GrossCents != 102600 {
		t.Errorf("DROMMAR GrossCents=%d want 102600", lines[1].GrossCents)
	}
}

func TestParseFile_NetAmounts(t *testing.T) {
	// CLAUDE.md: SOMMARNATT net=124000 (1240.00 SEK), DROMMAR net=102600 (1026.00 SEK).
	lines, _, _ := ParseFile(strings.NewReader(canonicalFixture))
	if lines[0].NetCents != 124000 {
		t.Errorf("SOMMARNATT NetCents=%d want 124000", lines[0].NetCents)
	}
	if lines[1].NetCents != 102600 {
		t.Errorf("DROMMAR NetCents=%d want 102600", lines[1].NetCents)
	}
}

func TestParseFile_RightCategory(t *testing.T) {
	lines, _, _ := ParseFile(strings.NewReader(canonicalFixture))
	for i, l := range lines {
		if l.RightCategory != "MEC" {
			t.Errorf("lines[%d].RightCategory=%q want MEC", i, l.RightCategory)
		}
	}
}

func TestParseFile_ControlledShares(t *testing.T) {
	lines, _, _ := ParseFile(strings.NewReader(canonicalFixture))
	for i, l := range lines {
		if l.ControlledNumerator != 10000 {
			t.Errorf("lines[%d].ControlledNumerator=%d want 10000", i, l.ControlledNumerator)
		}
		if l.ControlledDenominator != 10000 {
			t.Errorf("lines[%d].ControlledDenominator=%d want 10000", i, l.ControlledDenominator)
		}
	}
}

func TestParseFile_Currency(t *testing.T) {
	lines, _, _ := ParseFile(strings.NewReader(canonicalFixture))
	for i, l := range lines {
		if l.Currency != "SEK" {
			t.Errorf("lines[%d].Currency=%q want SEK", i, l.Currency)
		}
	}
}

func TestParseFile_WERTooShort(t *testing.T) {
	fixture := "WER" + strings.Repeat(" ", 50) // only 53 chars — too short
	lines, _, errs := ParseFile(strings.NewReader(fixture))
	if len(lines) != 0 {
		t.Errorf("got %d lines, want 0", len(lines))
	}
	if len(errs) == 0 {
		t.Error("expected parse error for short WER line, got none")
	}
}

func TestParseFile_AmountDecimals_Override(t *testing.T) {
	// SDN with amountDecimals=0: raw "000000000000003720" → 372000 cents (normalised to 2dp)
	fixture := strings.Join([]string{
		buildSDN(0, "SEK"),
		buildMWN("REF001", "WORK", ""),
		buildMDR("MD", "MEC"),
		buildMIP(10000, 10000),
		buildWER("ME", "SEK", 3720, 1240), // raw integers with 0dp → normalised to *100
	}, "\n")

	lines, _, _ := ParseFile(strings.NewReader(fixture))
	if len(lines) != 1 {
		t.Fatalf("got %d lines, want 1", len(lines))
	}
	// With amountDecimals=0, raw 3720 → 372000 cents (×100 normalisation)
	if lines[0].GrossCents != 372000 {
		t.Errorf("GrossCents=%d want 372000 (amountDecimals=0 normalised)", lines[0].GrossCents)
	}
}
