// Package cwr parses CWR 2.x fixed-width registration files.
// It extracts Work (NWR) and Writer (SWR) records needed to populate
// the Verostark catalogue (works, writers, work_writers tables).
//
// Field positions follow the CISAC CWR 2.1 specification (1-indexed in spec,
// 0-indexed in code). Only NWR and SWR records are parsed; all others are skipped.
//
// Inspired by django-music-publisher (MIT) by Matija Kolarič.
// Source: https://github.com/matijakolaric-com/django-music-publisher
package cwr

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Work holds the fields extracted from a CWR NWR record.
type Work struct {
	Title        string // positions 19–78 (60 chars)
	ISWC         string // positions 95–105 (11 chars), normalised to TNNNNNNNNNC
	SubmitterRef string // positions 81–94 (14 chars) — publisher's internal work reference
}

// Writer holds the fields extracted from a CWR SWR record.
type Writer struct {
	LastName        string  // positions 28–72 (45 chars)
	FirstName       string  // positions 73–102 (30 chars)
	IPIName         string  // positions 116–126 (11 chars)
	IPIBase         string  // positions 154–166 (13 chars)
	DesignationCode string  // positions 104–105 (2 chars) e.g. CA, C, A, E
	ManuscriptShare float64 // PR ownership share as fraction 0–1 (positions 130–134)
}

// WorkRecord groups one Work with its associated Writers from the same transaction.
type WorkRecord struct {
	Work    Work
	Writers []Writer
}

// ParseFile reads a CWR 2.x file and returns all work records with their writers.
// Lines that are too short to yield required fields are skipped with a note in
// the returned error list — parsing continues on all remaining lines.
func ParseFile(r io.Reader) ([]WorkRecord, []error) {
	var (
		records []WorkRecord
		errs    []error
		txIndex = map[string]int{} // transaction seq → index in records
	)

	scanner := bufio.NewScanner(r)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		if len(line) < 3 {
			continue
		}

		recType := line[:3]
		switch recType {
		case "NWR", "REV":
			w, err := parseNWR(line)
			if err != nil {
				errs = append(errs, fmt.Errorf("line %d: %w", lineNum, err))
				continue
			}
			txSeq := safeField(line, 3, 11)
			txIndex[txSeq] = len(records)
			records = append(records, WorkRecord{Work: w})

		case "SWR":
			w, err := parseSWR(line)
			if err != nil {
				errs = append(errs, fmt.Errorf("line %d: %w", lineNum, err))
				continue
			}
			txSeq := safeField(line, 3, 11)
			if idx, ok := txIndex[txSeq]; ok {
				records[idx].Writers = append(records[idx].Writers, w)
			}
			// SWR before NWR is malformed CWR — silently skip.
		}
	}

	if err := scanner.Err(); err != nil {
		errs = append(errs, fmt.Errorf("scanner: %w", err))
	}
	return records, errs
}

// parseNWR extracts Work fields from a CWR NWR/REV record line.
// Minimum required line length: 106 chars (up to end of ISWC field).
func parseNWR(line string) (Work, error) {
	if len(line) < 106 {
		return Work{}, fmt.Errorf("NWR line too short (%d chars, need 106)", len(line))
	}
	iswc := safeField(line, 95, 106)
	// Normalise ISWC: strip dashes and dots that appear in some CWR variants.
	if iswc != "" {
		r := strings.NewReplacer("-", "", ".", "")
		iswc = strings.ToUpper(r.Replace(iswc))
	}
	return Work{
		Title:        safeField(line, 19, 79),
		SubmitterRef: safeField(line, 81, 95),
		ISWC:         iswc,
	}, nil
}

// parseSWR extracts Writer fields from a CWR SWR record line.
// Minimum required line length: 135 chars (up to end of PR share field).
// IPI Base requires 167 chars — extracted only when available.
func parseSWR(line string) (Writer, error) {
	if len(line) < 135 {
		return Writer{}, fmt.Errorf("SWR line too short (%d chars, need 135)", len(line))
	}
	w := Writer{
		LastName:        safeField(line, 28, 73),
		FirstName:       safeField(line, 73, 103),
		DesignationCode: safeField(line, 104, 106),
		IPIName:         safeField(line, 116, 127),
		ManuscriptShare: parseShare(safeField(line, 130, 135)),
	}
	if len(line) >= 167 {
		w.IPIBase = safeField(line, 154, 167)
	}
	return w, nil
}

// safeField extracts line[start:end] trimmed of spaces.
// Returns "" if the line is too short.
func safeField(line string, start, end int) string {
	if len(line) <= start {
		return ""
	}
	if len(line) < end {
		end = len(line)
	}
	return strings.TrimSpace(line[start:end])
}

// parseShare converts a 5-char CWR ownership share string to a fraction (0–1).
// "05000" = 50.00% → 0.5000. "10000" = 100.00% → 1.0000.
func parseShare(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return float64(n) / 10000.0
}
