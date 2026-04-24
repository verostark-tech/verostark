// Package detection orchestrates deviation analysis for a royalty statement.
// For each statement line it evaluates the distribution key formula using the
// controlled share already present on the line, generates an AI explanation for
// flagged lines, and stores the results as detection_flags. No catalogue lookup
// is required — all inputs come from the statement itself.
//
// Two additional checks run after the per-line loop:
//   - Cross right type check: compares MEC vs PERF observed ratios for the same
//     work. A large divergence flags a systematic misapplication of the key.
//   - Unmatched lines report: lines that could not be evaluated (unknown right
//     type, zero controlled share) are recorded in detection_unmatched.
package detection

import (
	"context"
	"fmt"
	"math"
	"time"

	encoreauth "encore.dev/beta/auth"
	"encore.dev/beta/errs"
	"encore.dev/storage/sqldb"

	aisvc "encore.app/ai"
	authsvc "encore.app/auth"
	"encore.app/rules"
	"encore.app/statements"
)

var db = sqldb.NewDatabase("detection", sqldb.DatabaseConfig{
	Migrations: "./migrations",
})

// crossRightTypeDivergenceThreshold is the minimum absolute difference between
// the observed MEC and PERF ratios (net/gross) that triggers a flag.
const crossRightTypeDivergenceThreshold = 0.05

// --- Domain types ---

// Flag is a detected deviation between what was paid and what was expected.
type Flag struct {
	ID              int64     `json:"id"`
	OrgID           string    `json:"org_id"`
	DetectionRunID  int64     `json:"detection_run_id"`
	StatementLineID *int64    `json:"statement_line_id,omitempty"`
	WorkID          *int64    `json:"work_id,omitempty"`
	WorkTitle       string    `json:"work_title"`
	ISWC            string    `json:"iswc"`
	ExpectedAmount  float64   `json:"expected_amount"`
	ReceivedAmount  float64   `json:"received_amount"`
	DeviationAmount float64   `json:"deviation_amount"`
	DeviationPct    float64   `json:"deviation_pct"`
	Severity        string    `json:"severity"`
	PatternType     string    `json:"pattern_type"`
	Explanation     string    `json:"explanation"`
	Status          string    `json:"status"`
	CreatedAt       time.Time `json:"created_at"`
}

// UnmatchedLine is a statement line that could not be evaluated.
type UnmatchedLine struct {
	ID              int64   `json:"id"`
	StatementLineID int64   `json:"statement_line_id"`
	ISWC            string  `json:"iswc"`
	WorkRef         string  `json:"work_ref"`
	RightType       string  `json:"right_type"`
	NetAmount       float64 `json:"net_amount"`
	Period          string  `json:"period"`
	// Reason is one of: unknown_right_type | no_controlled_share
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"created_at"`
}

// --- Request / response types ---

type RunDetectionRequest struct {
	StatementID int64 `json:"statement_id"`
}

type RunDetectionResponse struct {
	RunID          int64 `json:"run_id"`
	FlagCount      int   `json:"flag_count"`
	UnmatchedCount int   `json:"unmatched_count"`
}

type ListFlagsRequest struct {
	StatementID int64  `json:"statement_id,omitempty"`
	Severity    string `json:"severity,omitempty"`
	Status      string `json:"status,omitempty"`
}

type ListFlagsResponse struct {
	Flags []Flag `json:"flags"`
}

type GetFlagRequest struct {
	ID int64 `json:"id"`
}

type GetUnmatchedRequest struct {
	StatementID int64 `json:"statement_id"`
}

type GetUnmatchedResponse struct {
	Lines []UnmatchedLine `json:"lines"`
}

// rightTypeEntry collects per-line data needed for the cross right type check.
type rightTypeEntry struct {
	lineID          int64
	title           string
	ratio           float64 // net / gross — the observed distribution ratio
	netAmt          float64
	grossAmt        float64
	period          string
	controlledShare float64
}

// crossWorkData holds the MEC and PERF entries for a single work.
type crossWorkData struct {
	mec  *rightTypeEntry
	perf *rightTypeEntry
}

// --- Private API ---

// RunDetection evaluates every line in the given statement, flags deviations,
// generates AI explanations, runs the cross right type check, and records
// unmatched lines. Statement status is updated throughout.
//
//encore:api private
func RunDetection(ctx context.Context, req *RunDetectionRequest) (*RunDetectionResponse, error) {
	data := encoreauth.Data().(*authsvc.AuthData)
	orgID := data.OrgID

	stmt, err := statements.GetStatement(ctx, &statements.GetStatementRequest{ID: req.StatementID})
	if err != nil {
		return nil, err
	}

	statements.UpdateStatementStatus(ctx, &statements.UpdateStatementStatusRequest{
		ID: stmt.ID, Status: "processing",
	})

	var runID int64
	if err := db.QueryRow(ctx,
		`INSERT INTO detection_runs (org_id, statement_id, status, started_at)
		 VALUES ($1, $2, 'running', $3) RETURNING id`,
		orgID, stmt.ID, time.Now(),
	).Scan(&runID); err != nil {
		return nil, &errs.Error{Code: errs.Internal, Message: "could not start detection run"}
	}

	linesResp, err := statements.ListStatementLines(ctx, &statements.ListStatementLinesRequest{
		StatementID: stmt.ID,
	})
	if err != nil {
		return nil, err
	}

	// Delete flags from any previous runs for this statement so re-runs replace, not accumulate.
	db.Exec(ctx,
		`DELETE FROM detection_flags
		 WHERE detection_run_id IN (
		     SELECT id FROM detection_runs WHERE statement_id=$1 AND org_id=$2 AND id != $3
		 )`,
		stmt.ID, orgID, runID,
	)

	var flagCount, unmatchedCount int
	crossWorks := map[string]*crossWorkData{}

	for _, line := range linesResp.Lines {
		if line.GrossAmount == 0 {
			continue
		}
		if line.RightType != "mechanical" && line.RightType != "performance" {
			addUnmatched(ctx, orgID, runID, line, "unknown_right_type")
			unmatchedCount++
			continue
		}
		if line.ControlledShare == 0 {
			addUnmatched(ctx, orgID, runID, line, "no_controlled_share")
			unmatchedCount++
			continue
		}

		period := line.Period
		if period == "" {
			period = stmt.Period
		}

		// --- Collect for cross right type check (keyed by ISWC when present) ---

		if line.ISWC != "" {
			entry := &rightTypeEntry{
				lineID:          line.ID,
				title:           line.WorkTitle,
				ratio:           line.NetAmount / line.GrossAmount,
				netAmt:          line.NetAmount,
				grossAmt:        line.GrossAmount,
				controlledShare: line.ControlledShare,
				period:          period,
			}
			if _, ok := crossWorks[line.ISWC]; !ok {
				crossWorks[line.ISWC] = &crossWorkData{}
			}
			if line.RightType == "mechanical" {
				crossWorks[line.ISWC].mec = entry
			} else {
				crossWorks[line.ISWC].perf = entry
			}
		}

		// --- Individual deviation check ---

		result, err := rules.Evaluate(rules.Input{
			Gross:                     line.GrossAmount,
			Received:                  line.NetAmount,
			ControlledManuscriptShare: line.ControlledShare,
			RightType:                 line.RightType,
		})
		if err != nil || !result.Flagged {
			continue
		}

		patternType := "overpayment"
		if result.DeviationAmount < 0 {
			patternType = "underpayment"
		}

		explanation := "An explanation could not be generated at this time."
		explain, err := aisvc.ExplainDeviation(ctx, &aisvc.ExplainRequest{
			WorkTitle:       line.WorkTitle,
			ISWC:            line.ISWC,
			RightType:       line.RightType,
			Period:          period,
			Severity:        result.Severity,
			GrossSEK:        line.GrossAmount,
			ControlledShare: line.ControlledShare,
			ExpectedSEK:     result.Expected,
			ReceivedSEK:     result.Received,
			DeviationSEK:    result.DeviationAmount,
			DeviationPct:    result.DeviationPct,
		})
		if err == nil {
			explanation = explain.Explanation
		}

		lineID := line.ID
		if _, insertErr := db.Exec(ctx,
			`INSERT INTO detection_flags
			    (org_id, detection_run_id, statement_line_id, work_title, iswc,
			     expected_amount, received_amount, deviation_amount, deviation_pct,
			     severity, pattern_type, explanation, status)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,'open')`,
			orgID, runID, lineID, line.WorkTitle, line.ISWC,
			result.Expected, result.Received, result.DeviationAmount, result.DeviationPct,
			result.Severity, patternType, explanation,
		); insertErr != nil {
			return nil, &errs.Error{Code: errs.Internal, Message: "could not save detection flag"}
		}
		flagCount++
	}

	// --- Cross right type check ---
	//
	// For any work where both MEC and PERF lines exist, compare their observed
	// ratios. A significant divergence means STIM applied the key differently
	// across right types for the same work — a separate class of error from the
	// per-line deviation check.

	for iswc, w := range crossWorks {
		if w.mec == nil || w.perf == nil {
			continue
		}

		divergence := math.Abs(w.mec.ratio - w.perf.ratio)
		if divergence < crossRightTypeDivergenceThreshold {
			continue
		}

		severity := rules.SeverityHigh
		if divergence > 0.15 {
			severity = rules.SeverityCritical
		}

		// Express the divergence in SEK: how much the MEC amount would change
		// if it had matched the PERF ratio.
		deviationSEK := (w.mec.ratio - w.perf.ratio) * w.mec.grossAmt
		expectedSEK := w.mec.grossAmt * w.mec.controlledShare / 3.0

		explanation := "An explanation could not be generated at this time."
		explain, err := aisvc.ExplainDeviation(ctx, &aisvc.ExplainRequest{
			WorkTitle:    w.mec.title,
			ISWC:         iswc,
			RightType:    "mechanical vs performance",
			Period:       w.mec.period,
			Severity:     severity,
			ExpectedSEK:  expectedSEK,
			ReceivedSEK:  w.mec.netAmt,
			DeviationSEK: deviationSEK,
			DeviationPct: w.mec.ratio - w.perf.ratio,
		})
		if err == nil {
			explanation = explain.Explanation
		}

		if _, crossErr := db.Exec(ctx,
			`INSERT INTO detection_flags
			    (org_id, detection_run_id, work_title, iswc,
			     expected_amount, received_amount, deviation_amount, deviation_pct,
			     severity, pattern_type, explanation, status)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,'open')`,
			orgID, runID, w.mec.title, iswc,
			expectedSEK, w.mec.netAmt, deviationSEK, w.mec.ratio-w.perf.ratio,
			severity, "right_type_divergence", explanation,
		); crossErr != nil {
			return nil, &errs.Error{Code: errs.Internal, Message: "could not save cross right type flag"}
		}
		flagCount++
	}

	db.Exec(ctx,
		`UPDATE detection_runs
		 SET status='completed', flag_count=$1, unmatched_count=$2, completed_at=$3
		 WHERE id=$4`,
		flagCount, unmatchedCount, time.Now(), runID,
	)
	statements.UpdateStatementStatus(ctx, &statements.UpdateStatementStatusRequest{
		ID: stmt.ID, Status: "completed",
	})

	return &RunDetectionResponse{
		RunID:          runID,
		FlagCount:      flagCount,
		UnmatchedCount: unmatchedCount,
	}, nil
}

// addUnmatched records a line that could not be evaluated.
func addUnmatched(ctx context.Context, orgID string, runID int64, line statements.StatementLine, reason string) {
	db.Exec(ctx,
		`INSERT INTO detection_unmatched
		    (org_id, detection_run_id, statement_line_id, iswc, work_ref, right_type, net_amount, period, reason)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		orgID, runID, line.ID, line.ISWC, line.WorkRef,
		line.RightType, line.NetAmount, line.Period, reason,
	)
}

// ListFlags returns deviation flags for the org, with optional filters.
//
//encore:api private
func ListFlags(ctx context.Context, req *ListFlagsRequest) (*ListFlagsResponse, error) {
	data := encoreauth.Data().(*authsvc.AuthData)
	orgID := data.OrgID

	// Always scope to the latest detection run per statement so re-runs replace
	// what the administrator sees rather than accumulating historical duplicates.
	query := `SELECT f.id, f.org_id, f.detection_run_id, f.statement_line_id, f.work_id,
	                 f.work_title, f.iswc, f.expected_amount, f.received_amount,
	                 f.deviation_amount, f.deviation_pct, f.severity, f.pattern_type,
	                 f.explanation, f.status, f.created_at
	          FROM detection_flags f
	          WHERE f.org_id = $1
	            AND f.detection_run_id IN (
	                SELECT MAX(id) FROM detection_runs WHERE org_id = $1 GROUP BY statement_id
	            )`
	args := []interface{}{orgID}
	n := 2

	if req.Severity != "" {
		query += fmt.Sprintf(" AND f.severity = $%d", n)
		args = append(args, req.Severity)
		n++
	}
	if req.Status != "" {
		query += fmt.Sprintf(" AND f.status = $%d", n)
		args = append(args, req.Status)
		n++
	}
	if req.StatementID != 0 {
		query += fmt.Sprintf(
			" AND f.detection_run_id IN (SELECT id FROM detection_runs WHERE statement_id=$%d AND org_id=$1)",
			n,
		)
		args = append(args, req.StatementID)
	}
	query += " ORDER BY f.created_at DESC"

	rows, err := db.Query(ctx, query, args...)
	if err != nil {
		return nil, &errs.Error{Code: errs.Internal, Message: "could not load deviations"}
	}
	defer rows.Close()

	var out []Flag
	for rows.Next() {
		var f Flag
		rows.Scan(&f.ID, &f.OrgID, &f.DetectionRunID, &f.StatementLineID, &f.WorkID,
			&f.WorkTitle, &f.ISWC, &f.ExpectedAmount, &f.ReceivedAmount,
			&f.DeviationAmount, &f.DeviationPct, &f.Severity, &f.PatternType,
			&f.Explanation, &f.Status, &f.CreatedAt)
		out = append(out, f)
	}
	return &ListFlagsResponse{Flags: out}, nil
}

// GetFlag returns a single deviation flag by ID, verified against the caller's org.
//
//encore:api private
func GetFlag(ctx context.Context, req *GetFlagRequest) (*Flag, error) {
	data := encoreauth.Data().(*authsvc.AuthData)
	orgID := data.OrgID

	var f Flag
	err := db.QueryRow(ctx,
		`SELECT id, org_id, detection_run_id, statement_line_id, work_id,
		        work_title, iswc, expected_amount, received_amount,
		        deviation_amount, deviation_pct, severity, pattern_type,
		        explanation, status, created_at
		 FROM detection_flags WHERE id=$1 AND org_id=$2`,
		req.ID, orgID,
	).Scan(&f.ID, &f.OrgID, &f.DetectionRunID, &f.StatementLineID, &f.WorkID,
		&f.WorkTitle, &f.ISWC, &f.ExpectedAmount, &f.ReceivedAmount,
		&f.DeviationAmount, &f.DeviationPct, &f.Severity, &f.PatternType,
		&f.Explanation, &f.Status, &f.CreatedAt)
	if err != nil {
		return nil, &errs.Error{Code: errs.NotFound, Message: "deviation not found"}
	}
	return &f, nil
}

// GetUnmatched returns lines from the latest detection run for the given
// statement that could not be matched or evaluated, so the administrator
// knows which works to investigate or register.
//
//encore:api private
func GetUnmatched(ctx context.Context, req *GetUnmatchedRequest) (*GetUnmatchedResponse, error) {
	data := encoreauth.Data().(*authsvc.AuthData)
	orgID := data.OrgID

	// Resolve the latest detection run for this statement.
	var runID int64
	err := db.QueryRow(ctx,
		`SELECT id FROM detection_runs
		 WHERE statement_id=$1 AND org_id=$2
		 ORDER BY created_at DESC LIMIT 1`,
		req.StatementID, orgID,
	).Scan(&runID)
	if err != nil {
		return nil, &errs.Error{Code: errs.NotFound, Message: "no detection run found for this statement"}
	}

	rows, err := db.Query(ctx,
		`SELECT id, statement_line_id, iswc, work_ref, right_type,
		        net_amount, period, reason, created_at
		 FROM detection_unmatched
		 WHERE detection_run_id=$1 AND org_id=$2
		 ORDER BY created_at`,
		runID, orgID,
	)
	if err != nil {
		return nil, &errs.Error{Code: errs.Internal, Message: "could not load unmatched lines"}
	}
	defer rows.Close()

	var out []UnmatchedLine
	for rows.Next() {
		var u UnmatchedLine
		rows.Scan(&u.ID, &u.StatementLineID, &u.ISWC, &u.WorkRef,
			&u.RightType, &u.NetAmount, &u.Period, &u.Reason, &u.CreatedAt)
		out = append(out, u)
	}
	return &GetUnmatchedResponse{Lines: out}, nil
}
