// Package crd parses CISAC CRD EDI Format Specifications Version 3.0 Revision 5.
//
// It extracts WER (Work Entry Record) records and carries forward parent context:
//   - SDN — distribution period, currency, amount decimal places
//   - MWN — work identity (work_ref, work_title, iswc)
//   - MDR — right type (right_code, right_category)
//   - MIP — controlled share (numerator / denominator)
//
// All positions follow the CISAC CRD 3.0 R5 specification (1-indexed).
// In code: pos N, size S → line[N-1 : N-1+S].
//
// Amounts are returned as GrossCents and NetCents — raw integers normalised
// to 2 implied decimal places (cent units for SEK). Ratios and detection
// arithmetic are performed by the rules package using math/big.Rat.
package crd

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Line is one WER record with its accumulated parent-record context.
type Line struct {
	// From MWN
	WorkRef   string // pos 20 size 14
	WorkTitle string // pos 34 size 60
	ISWC      string // pos 154 size 11

	// From MDR
	RightCode     string // pos 20 size 2  (e.g., "MD" = MEC streaming)
	RightCategory string // pos 22 size 3  (e.g., "MEC")

	// From MIP
	ControlledNumerator   int64 // pos 36 size 5
	ControlledDenominator int64 // pos 41 size 5

	// From WER
	DistributionCategory string // pos 35 size 2
	Currency             string // pos 246 size 3

	// Amounts normalised to 2 implied decimal places (cent units).
	// GrossCents=372000 represents 3720.00 SEK.
	GrossCents int64 // pos 249 size 18
	NetCents   int64 // pos 350 size 18
}

// ParseFile reads a CRD 3.0 R5 file and returns all WER records with their
// accumulated parent context. SDN, MWN, MDR, and MIP state carries forward
// until replaced by a new record of the same type.
//
// The second return value is the distribution period formatted as "YYYY-QN"
// (e.g. "2025-Q1"), derived from the SDN period_start field. Empty string if
// no SDN record is present.
//
// Parse errors on individual records are collected without halting — parsing
// always continues to the end of file. Call sites should log the returned errors.
func ParseFile(r io.Reader) ([]Line, string, []error) {
	var (
		out  []Line
		errs []error

		amountDecimals = 2 // overridden by SDN amount_decimals field
		period         string
		workRef        string
		workTitle      string
		iswc           string
		rightCode      string
		rightCategory  string
		ctrlNum        int64
		ctrlDen        int64
	)

	scanner := bufio.NewScanner(r)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		raw := scanner.Text()
		if len(raw) < 3 {
			continue
		}

		switch raw[:3] {
		case "SDN":
			if d, err := parseSDNDecimals(raw); err == nil {
				amountDecimals = d
			}
			if p := parseSDNPeriod(raw); p != "" && period == "" {
				period = p // use the first SDN period seen
			}
		case "MWN":
			workRef = field(raw, 20, 14)
			workTitle = field(raw, 34, 60)
			iswc = field(raw, 154, 11)
		case "MDR":
			rightCode = field(raw, 20, 2)
			rightCategory = field(raw, 22, 3)
		case "MIP":
			n, d, err := parseMIPShares(raw)
			if err != nil {
				errs = append(errs, fmt.Errorf("line %d MIP: %w", lineNum, err))
				continue
			}
			ctrlNum, ctrlDen = n, d
		case "WER":
			l, err := parseWER(raw, amountDecimals)
			if err != nil {
				errs = append(errs, fmt.Errorf("line %d WER: %w", lineNum, err))
				continue
			}
			l.WorkRef = workRef
			l.WorkTitle = workTitle
			l.ISWC = iswc
			l.RightCode = rightCode
			l.RightCategory = rightCategory
			l.ControlledNumerator = ctrlNum
			l.ControlledDenominator = ctrlDen
			out = append(out, l)
		}
	}

	if err := scanner.Err(); err != nil {
		errs = append(errs, fmt.Errorf("scanner: %w", err))
	}
	return out, period, errs
}

// parseSDNPeriod reads period_start (pos 33, size 8, YYYYMMDD) from an SDN
// record and returns it formatted as "YYYY-QN". Returns "" if absent or invalid.
func parseSDNPeriod(line string) string {
	s := field(line, 33, 8)
	if len(s) < 6 {
		return ""
	}
	year := s[:4]
	month, err := strconv.Atoi(s[4:6])
	if err != nil || month < 1 || month > 12 {
		return ""
	}
	q := (month-1)/3 + 1
	return fmt.Sprintf("%s-Q%d", year, q)
}

// parseSDNDecimals reads the amount_decimals field from an SDN record.
// pos 113, size 1. Returns 2 (the STIM default) if the field is absent.
func parseSDNDecimals(line string) (int, error) {
	s := field(line, 113, 1)
	if s == "" {
		return 2, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("SDN amount_decimals: %w", err)
	}
	return n, nil
}

// parseWER extracts the royalty amount fields from a CRD WER record.
// Minimum required line length: 367 chars (pos 350 + size 18 − 1).
func parseWER(line string, amountDecimals int) (Line, error) {
	if len(line) < 367 {
		return Line{}, fmt.Errorf("WER line too short (%d chars, need 367)", len(line))
	}
	gross, err := parseAmountCents(field(line, 249, 18), amountDecimals)
	if err != nil {
		return Line{}, fmt.Errorf("gross_amount: %w", err)
	}
	net, err := parseAmountCents(field(line, 350, 18), amountDecimals)
	if err != nil {
		return Line{}, fmt.Errorf("remitted_amount: %w", err)
	}
	return Line{
		DistributionCategory: field(line, 35, 2),
		Currency:             field(line, 246, 3),
		GrossCents:           gross,
		NetCents:             net,
	}, nil
}

// parseMIPShares extracts controlled share numerator and denominator from a MIP record.
// Minimum required line length: 45 chars (pos 41 + size 5 − 1).
func parseMIPShares(line string) (num, den int64, err error) {
	if len(line) < 45 {
		return 0, 0, fmt.Errorf("MIP line too short (%d chars, need 45)", len(line))
	}
	numStr := field(line, 36, 5)
	denStr := field(line, 41, 5)
	num, err = strconv.ParseInt(numStr, 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("numerator %q: %w", numStr, err)
	}
	den, err = strconv.ParseInt(denStr, 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("denominator %q: %w", denStr, err)
	}
	return num, den, nil
}

// field extracts and trims a fixed-width field from line.
// pos is 1-indexed (CISAC spec convention); size is the field width in chars.
// Returns "" if the line is too short to contain the field start.
func field(line string, pos, size int) string {
	start := pos - 1
	end := start + size
	if start >= len(line) {
		return ""
	}
	if end > len(line) {
		end = len(line)
	}
	return strings.TrimSpace(line[start:end])
}

// parseAmountCents parses an 18-char right-justified zero-padded integer and
// normalises it to 2 implied decimal places (cent units).
//
// "000000000000372000" with amountDecimals=2 → 372000  (3720.00 SEK)
// "000000000000372000" with amountDecimals=0 → 37200000 (372000.00 — unlikely)
func parseAmountCents(s string, amountDecimals int) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty amount field")
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse %q: %w", s, err)
	}
	// Normalise to 2 decimal places.
	shift := 2 - amountDecimals
	for i := 0; i < shift; i++ {
		n *= 10
	}
	for i := 0; i > shift; i-- {
		n /= 10
	}
	return n, nil
}
