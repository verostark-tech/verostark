// Package validators provides ISWC and IPI number validation and normalisation.
// Algorithms ported from django-music-publisher (MIT licence) by Matija Kolarič.
// Source: https://github.com/matijakolaric-com/django-music-publisher
package validators

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	reISWC        = regexp.MustCompile(`^T\d{10}$`)
	reIPIBase     = regexp.MustCompile(`^I-\d{9}-\d$`)
	reIPIBaseNorm = regexp.MustCompile(`(?i)I.?(\d{9}).?(\d)`)
	reNonDigit    = regexp.MustCompile(`\D`)
)

// NormaliseISWC strips dashes, dots, and spaces and uppercases the result.
// Accepts T-NNN.NNN.NNN-C, T-NNNNNNNNN-C, and TNNNNNNNNNC.
func NormaliseISWC(iswc string) string {
	r := strings.NewReplacer("-", "", ".", "", " ", "")
	return strings.ToUpper(r.Replace(iswc))
}

// NormaliseIPIBase normalises to I-NNNNNNNNN-C.
// Accepts dots, dashes, missing separators, and mixed case.
func NormaliseIPIBase(ipi string) string {
	ipi = strings.ReplaceAll(ipi, ".", "")
	ipi = strings.ToUpper(ipi)
	return reIPIBaseNorm.ReplaceAllString(ipi, "I-$1-$2")
}

// NormaliseIPIName zero-pads an IPI name number to 11 digits.
func NormaliseIPIName(ipi string) string {
	ipi = strings.TrimSpace(ipi)
	for len(ipi) < 11 {
		ipi = "0" + ipi
	}
	return ipi
}

// ValidateISWC validates a normalised ISWC (TNNNNNNNNNC format).
// Call NormaliseISWC first if the input may contain dashes or dots.
func ValidateISWC(iswc string) error {
	if !reISWC.MatchString(iswc) {
		return fmt.Errorf("ISWC %q must be in format TNNNNNNNNNC", iswc)
	}
	return checkISWCDigit(iswc, 1)
}

// ValidateIPIBase validates a normalised IPI Base Number (I-NNNNNNNNN-C format).
// Call NormaliseIPIBase first if the input may use dots or missing separators.
func ValidateIPIBase(ipi string) error {
	if !reIPIBase.MatchString(ipi) {
		return fmt.Errorf("IPI Base %q must be in format I-NNNNNNNNN-C", ipi)
	}
	return checkISWCDigit(ipi, 2)
}

// ValidateIPIName validates an 11-digit IPI Name Number.
// The last two digits are check digits computed via the mod-101 algorithm.
func ValidateIPIName(ipi string) error {
	if len(ipi) != 11 {
		return fmt.Errorf("IPI Name %q must be exactly 11 digits", ipi)
	}
	for _, c := range ipi {
		if c < '0' || c > '9' {
			return fmt.Errorf("IPI Name %q must be numeric", ipi)
		}
	}
	return checkIPINameDigit(ipi)
}

// checkISWCDigit implements the ISWC / IPI Base checksum algorithm.
// weight is 1 for ISWC, 2 for IPI Base.
// Ported from check_iswc_digit in django-music-publisher validators.py.
func checkISWCDigit(s string, weight int) error {
	digits := reNonDigit.ReplaceAllString(s, "")
	if len(digits) < 10 {
		return fmt.Errorf("not enough digits in %q", s)
	}
	total := weight
	for i, d := range digits[:9] {
		total += (i + 1) * int(d-'0')
	}
	checksum := (10 - total%10) % 10
	if checksum != int(digits[9]-'0') {
		return fmt.Errorf("invalid checksum in %q", s)
	}
	return nil
}

// checkIPINameDigit validates the two check digits of an IPI Name Number.
// Ported from check_ipi_digit in django-music-publisher validators.py.
func checkIPINameDigit(ipi string) error {
	digits := ipi[:9]
	total := 0
	for i, d := range digits {
		total += int(d-'0') * (10 - i)
	}
	total %= 101
	if total != 0 {
		total = (101 - total) % 100
	}
	expected := fmt.Sprintf("%02d", total)
	if ipi[9:] != expected {
		return fmt.Errorf("invalid check digits in IPI Name %q", ipi)
	}
	return nil
}
