// Package detection orchestrates deviation analysis for a royalty statement.
// For each statement line it: matches the work in the catalogue, evaluates the
// distribution key formula, generates an AI explanation for flagged lines, and
// stores the results as detection_flags.
package detection

import (
	"context"
	"fmt"
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
	Recommendation  string    `json:"recommendation"`
	Status          string    `json:"status"`
	CreatedAt       time.Time `json:"created_at"`
}

// --- Request / response types ---

type RunDetectionRequest struct {
	StatementID int64 `json:"statement_id"`
}

type RunDetectionResponse struct {
	RunID     int64 `json:"run_id"`
	FlagCount int   `json:"flag_count"`
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

// --- Private API ---

// RunDetection evaluates every line in the given statement, flags deviations,
// generates AI explanations, and stores results. The statement's status is
// updated from "pending" → "processing" → "completed".
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

	var flagCount int
	for _, line := range linesResp.Lines {
		if line.ISWC == "" || line.GrossAmount == nil {
			continue
		}
		if line.RightType != "mechanical" && line.RightType != "performance" {
			continue
		}

		match, err := statements.GetWorkForLine(ctx, &statements.GetWorkForLineRequest{ISWC: line.ISWC})
		if err != nil || match.ControlledShare == 0 {
			continue
		}

		result, err := rules.Evaluate(rules.Input{
			Gross:                     *line.GrossAmount,
			Received:                  line.NetAmount,
			ControlledManuscriptShare: match.ControlledShare,
			RightType:                 line.RightType,
		})
		if err != nil || !result.Flagged {
			continue
		}

		period := line.Period
		if period == "" {
			period = stmt.Period
		}

		patternType := "overpayment"
		if result.DeviationAmount < 0 {
			patternType = "underpayment"
		}

		explanation := "An explanation could not be generated at this time."
		explain, err := aisvc.ExplainDeviation(ctx, &aisvc.ExplainRequest{
			WorkTitle:    match.Title,
			ISWC:         line.ISWC,
			RightType:    line.RightType,
			Period:       period,
			Severity:     result.Severity,
			ExpectedSEK:  result.Expected,
			ReceivedSEK:  result.Received,
			DeviationSEK: result.DeviationAmount,
			DeviationPct: result.DeviationPct,
		})
		if err == nil {
			explanation = explain.Explanation
		}

		// Recommendation is always rule-based — available even when the AI call fails.
		recommendation := rules.Recommend(result.Severity, patternType)

		lineID := line.ID
		workID := match.WorkID
		if _, err := db.Exec(ctx,
			`INSERT INTO detection_flags
			    (org_id, detection_run_id, statement_line_id, work_id, work_title, iswc,
			     expected_amount, received_amount, deviation_amount, deviation_pct,
			     severity, pattern_type, explanation, recommendation, status)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,'open')`,
			orgID, runID, lineID, workID, match.Title, line.ISWC,
			result.Expected, result.Received, result.DeviationAmount, result.DeviationPct,
			result.Severity, patternType, explanation, recommendation,
		); err == nil {
			flagCount++
		}
	}

	db.Exec(ctx,
		`UPDATE detection_runs SET status='completed', flag_count=$1, completed_at=$2 WHERE id=$3`,
		flagCount, time.Now(), runID,
	)
	statements.UpdateStatementStatus(ctx, &statements.UpdateStatementStatusRequest{
		ID: stmt.ID, Status: "completed",
	})

	return &RunDetectionResponse{RunID: runID, FlagCount: flagCount}, nil
}

// ListFlags returns deviation flags for the org, with optional filters.
//
//encore:api private
func ListFlags(ctx context.Context, req *ListFlagsRequest) (*ListFlagsResponse, error) {
	data := encoreauth.Data().(*authsvc.AuthData)
	orgID := data.OrgID

	query := `SELECT f.id, f.org_id, f.detection_run_id, f.statement_line_id, f.work_id,
	                 f.work_title, f.iswc, f.expected_amount, f.received_amount,
	                 f.deviation_amount, f.deviation_pct, f.severity, f.pattern_type,
	                 f.explanation, f.recommendation, f.status, f.created_at
	          FROM detection_flags f
	          WHERE f.org_id = $1`
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
			&f.Explanation, &f.Recommendation, &f.Status, &f.CreatedAt)
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
		        explanation, recommendation, status, created_at
		 FROM detection_flags WHERE id=$1 AND org_id=$2`,
		req.ID, orgID,
	).Scan(&f.ID, &f.OrgID, &f.DetectionRunID, &f.StatementLineID, &f.WorkID,
		&f.WorkTitle, &f.ISWC, &f.ExpectedAmount, &f.ReceivedAmount,
		&f.DeviationAmount, &f.DeviationPct, &f.Severity, &f.PatternType,
		&f.Explanation, &f.Recommendation, &f.Status, &f.CreatedAt)
	if err != nil {
		return nil, &errs.Error{Code: errs.NotFound, Message: "deviation not found"}
	}
	return &f, nil
}
