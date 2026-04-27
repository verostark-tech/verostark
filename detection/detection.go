// Package detection orchestrates deviation analysis for a CRD royalty statement.
// For each statement line it evaluates the STIM distribution key formula using
// exact rational arithmetic (via rules.Evaluate), generates an AI explanation
// for flagged lines, and stores the results as detection_flags.
//
// All inputs come from the statement_lines table (populated by crd.ParseFile).
// No catalogue lookup is required.
package detection

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	encoreauth "encore.dev/beta/auth"
	"encore.dev/beta/errs"
	"encore.dev/storage/sqldb"

	aisvc "encore.app/ai"
	authsvc "encore.app/auth"
	"encore.app/rules"
	"encore.app/statements"
)

// aiConcurrency is the maximum number of Claude API calls in-flight at once.
// Keeps us well within rate limits while cutting wall-clock time significantly.
const aiConcurrency = 5

// possibleExplanation is used instead of a Claude call for POSSIBLE-severity flags.
// The deviation is small enough that it may be rounding — AI adds no value here.
const possibleExplanation = "The payment received is slightly above the expected publisher share. The difference is small and may reflect rounding in the collecting society's distribution calculation for this period."
const possibleNextStep = "Compare the gross amount reported in this statement against your own records for the same period. If the pattern repeats across multiple periods, consider raising it with your collecting society."

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
	NextStep        string    `json:"next_step"`
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

// ProgressResponse is the payload polled by the frontend every ~500 ms.
// Phase values: reading | identifying | loading_key | checking_ratios | explaining | done | failed
type ProgressResponse struct {
	Phase           string  `json:"phase"`
	WorksTotal      int     `json:"works_total"`
	WorksChecked    int     `json:"works_checked"`
	DistributionKey string  `json:"distribution_key"`
	FlagCount       int     `json:"flag_count"`
	UnmatchedCount  int     `json:"unmatched_count"`
	Error           *string `json:"error"`
}

type GetProgressRequest struct {
	StatementID int64 `json:"statement_id"`
}

const distributionKey = "Standard 1/3 mechanical share · Sweden"

// setProgress upserts the current detection phase into detection_progress.
// Errors are silently ignored — progress is best-effort.
func setProgress(ctx context.Context, stmtID int64, orgID string, p ProgressResponse) {
	db.Exec(ctx,
		`INSERT INTO detection_progress
		     (statement_id, org_id, phase, works_total, works_checked,
		      flag_count, unmatched_count, error, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,NOW())
		 ON CONFLICT (statement_id) DO UPDATE SET
		     phase=EXCLUDED.phase, works_total=EXCLUDED.works_total,
		     works_checked=EXCLUDED.works_checked, flag_count=EXCLUDED.flag_count,
		     unmatched_count=EXCLUDED.unmatched_count, error=EXCLUDED.error,
		     updated_at=NOW()`,
		stmtID, orgID, p.Phase, p.WorksTotal, p.WorksChecked,
		p.FlagCount, p.UnmatchedCount, p.Error,
	)
}

// GetProgress returns the current detection phase and counts for a statement.
// Returns phase="reading" with zero counts when detection has not started yet.
//
//encore:api private
func GetProgress(ctx context.Context, req *GetProgressRequest) (*ProgressResponse, error) {
	data := encoreauth.Data().(*authsvc.AuthData)
	orgID := data.OrgID

	p := &ProgressResponse{DistributionKey: distributionKey}
	err := db.QueryRow(ctx,
		`SELECT phase, works_total, works_checked, flag_count, unmatched_count, error
		 FROM detection_progress WHERE statement_id=$1 AND org_id=$2`,
		req.StatementID, orgID,
	).Scan(&p.Phase, &p.WorksTotal, &p.WorksChecked, &p.FlagCount, &p.UnmatchedCount, &p.Error)
	if err != nil {
		p.Phase = "reading"
	}
	return p, nil
}

// --- Private API ---

// RunDetection evaluates every line in the given statement using exact rational
// arithmetic, flags deviations, generates AI explanations for flagged lines,
// and records lines that could not be evaluated. Statement status is updated
// throughout.
//
//encore:api private
func RunDetection(ctx context.Context, req *RunDetectionRequest) (*RunDetectionResponse, error) {
	data := encoreauth.Data().(*authsvc.AuthData)
	orgID := data.OrgID

	// Phase: reading — fetch statement metadata.
	setProgress(ctx, req.StatementID, orgID, ProgressResponse{
		Phase: "reading", DistributionKey: distributionKey,
	})

	stmt, err := statements.GetStatement(ctx, &statements.GetStatementRequest{ID: req.StatementID})
	if err != nil {
		errMsg := err.Error()
		setProgress(ctx, req.StatementID, orgID, ProgressResponse{Phase: "failed", Error: &errMsg})
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

	// Phase: identifying — load all statement lines, report total count.
	linesResp, err := statements.ListStatementLines(ctx, &statements.ListStatementLinesRequest{
		StatementID: stmt.ID,
	})
	if err != nil {
		return nil, err
	}
	totalLines := len(linesResp.Lines)
	setProgress(ctx, stmt.ID, orgID, ProgressResponse{
		Phase: "identifying", WorksTotal: totalLines, DistributionKey: distributionKey,
	})

	// Delete flags from any previous runs for this statement so re-runs replace, not accumulate.
	db.Exec(ctx,
		`DELETE FROM detection_flags
		 WHERE detection_run_id IN (
		     SELECT id FROM detection_runs WHERE statement_id=$1 AND org_id=$2 AND id != $3
		 )`,
		stmt.ID, orgID, runID,
	)

	// Phase: loading_key — distribution key is loaded, about to evaluate.
	setProgress(ctx, stmt.ID, orgID, ProgressResponse{
		Phase: "loading_key", WorksTotal: totalLines, DistributionKey: distributionKey,
	})

	// pendingFlag holds everything needed to insert one detection_flags row.
	type pendingFlag struct {
		lineID      int64
		workTitle   string
		iswc        string
		patternType string
		result      rules.Result
		aiReq       *aisvc.ExplainRequest // nil when AI is skipped (POSSIBLE severity)
		explanation string
		nextStep    string
	}

	var unmatchedCount int
	var pending []pendingFlag

	// ── Pass 1: evaluate every line (pure arithmetic, no I/O) ─────────────────
	for i, line := range linesResp.Lines {
		if i%5 == 0 {
			setProgress(ctx, stmt.ID, orgID, ProgressResponse{
				Phase:           "checking_ratios",
				WorksTotal:      totalLines,
				WorksChecked:    i,
				UnmatchedCount:  unmatchedCount,
				DistributionKey: distributionKey,
			})
		}
		if line.GrossCents == 0 {
			continue
		}
		if line.RightType != "mechanical" {
			addUnmatched(ctx, orgID, runID, line, "unknown_right_type")
			unmatchedCount++
			continue
		}
		if line.ControlledDenominator == 0 || line.ControlledNumerator == 0 {
			addUnmatched(ctx, orgID, runID, line, "no_controlled_share")
			unmatchedCount++
			continue
		}

		period := line.Period
		if period == "" {
			period = stmt.Period
		}

		result, err := rules.Evaluate(rules.Input{
			GrossCents:            line.GrossCents,
			NetCents:              line.NetCents,
			ControlledNumerator:   line.ControlledNumerator,
			ControlledDenominator: line.ControlledDenominator,
		})
		if err != nil || !result.Flagged {
			continue
		}

		patternType := "overpayment"
		if result.DeviationAmount < 0 {
			patternType = "underpayment"
		}

		pf := pendingFlag{
			lineID:      line.ID,
			workTitle:   line.WorkTitle,
			iswc:        line.ISWC,
			patternType: patternType,
			result:      result,
			explanation: "An explanation could not be generated at this time.",
			nextStep:    "",
		}

		if result.Severity == rules.SeverityPossible {
			// Small deviation — skip Claude, use static text.
			pf.explanation = possibleExplanation
			pf.nextStep = possibleNextStep
		} else {
			pf.aiReq = &aisvc.ExplainRequest{
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
			}
		}

		pending = append(pending, pf)
	}

	// ── Pass 2: fan out Claude calls (max aiConcurrency in-flight) ────────────
	var aiCallCount int
	for _, pf := range pending {
		if pf.aiReq != nil {
			aiCallCount++
		}
	}
	setProgress(ctx, stmt.ID, orgID, ProgressResponse{
		Phase:           "explaining",
		WorksTotal:      aiCallCount,
		WorksChecked:    0,
		FlagCount:       0,
		UnmatchedCount:  unmatchedCount,
		DistributionKey: distributionKey,
	})

	sem := make(chan struct{}, aiConcurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var explained int32 // atomic counter for progress updates

	for i := range pending {
		if pending[i].aiReq == nil {
			continue // POSSIBLE — already has static text
		}
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			resp, err := aisvc.ExplainDeviation(ctx, pending[i].aiReq)
			if err == nil {
				mu.Lock()
				pending[i].explanation = resp.Explanation
				pending[i].nextStep = resp.NextStep
				mu.Unlock()
			}
			n := int(atomic.AddInt32(&explained, 1))
			setProgress(ctx, stmt.ID, orgID, ProgressResponse{
				Phase:           "explaining",
				WorksTotal:      aiCallCount,
				WorksChecked:    n,
				FlagCount:       n,
				UnmatchedCount:  unmatchedCount,
				DistributionKey: distributionKey,
			})
		}(i)
	}
	wg.Wait()

	// ── Pass 3: insert all flags (sequential DB writes) ───────────────────────
	var flagCount int
	for _, pf := range pending {
		if _, insertErr := db.Exec(ctx,
			`INSERT INTO detection_flags
			    (org_id, detection_run_id, statement_line_id, work_title, iswc,
			     expected_amount, received_amount, deviation_amount, deviation_pct,
			     severity, pattern_type, explanation, next_step, status)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,'open')`,
			orgID, runID, pf.lineID, pf.workTitle, pf.iswc,
			pf.result.Expected, pf.result.Received, pf.result.DeviationAmount, pf.result.DeviationPct,
			pf.result.Severity, pf.patternType, pf.explanation, pf.nextStep,
		); insertErr != nil {
			return nil, &errs.Error{Code: errs.Internal, Message: "could not save detection flag"}
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

	// Phase: done — all flags written.
	setProgress(ctx, stmt.ID, orgID, ProgressResponse{
		Phase:           "done",
		WorksTotal:      totalLines,
		WorksChecked:    totalLines,
		FlagCount:       flagCount,
		UnmatchedCount:  unmatchedCount,
		DistributionKey: distributionKey,
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

	query := `SELECT f.id, f.org_id, f.detection_run_id, f.statement_line_id, f.work_id,
	                 f.work_title, f.iswc, f.expected_amount, f.received_amount,
	                 f.deviation_amount, f.deviation_pct, f.severity, f.pattern_type,
	                 f.explanation, f.next_step, f.status, f.created_at
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
			&f.Explanation, &f.NextStep, &f.Status, &f.CreatedAt)
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
		        explanation, next_step, status, created_at
		 FROM detection_flags WHERE id=$1 AND org_id=$2`,
		req.ID, orgID,
	).Scan(&f.ID, &f.OrgID, &f.DetectionRunID, &f.StatementLineID, &f.WorkID,
		&f.WorkTitle, &f.ISWC, &f.ExpectedAmount, &f.ReceivedAmount,
		&f.DeviationAmount, &f.DeviationPct, &f.Severity, &f.PatternType,
		&f.Explanation, &f.NextStep, &f.Status, &f.CreatedAt)
	if err != nil {
		return nil, &errs.Error{Code: errs.NotFound, Message: "deviation not found"}
	}
	return &f, nil
}

// GetUnmatched returns lines from the latest detection run for the given
// statement that could not be evaluated.
//
//encore:api private
func GetUnmatched(ctx context.Context, req *GetUnmatchedRequest) (*GetUnmatchedResponse, error) {
	data := encoreauth.Data().(*authsvc.AuthData)
	orgID := data.OrgID

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
