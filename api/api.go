// Package api exposes all public REST endpoints consumed by the Lovable frontend.
// It is a thin layer: validate input, delegate to the statements or detection service,
// return the response. No business logic lives here.
package api

import (
	"context"

	"encore.dev"
	"encore.dev/beta/errs"

	detectionsvc "encore.app/detection"
	"encore.app/statements"
)

// =============================================================================
// Statements
// =============================================================================

type CreateStatementRequest struct {
	Filename string `json:"filename"`
	Period   string `json:"period"`
	FileKey  string `json:"file_key"`
}

// CreateStatement registers an uploaded STIM statement file and parses its lines.
// Call /files/upload first to obtain the file_key, filename, and store the file.
//
//encore:api auth method=POST path=/api/statements
func CreateStatement(ctx context.Context, req *CreateStatementRequest) (*statements.Statement, error) {
	if req.Filename == "" {
		return nil, &errs.Error{Code: errs.InvalidArgument, Message: "filename is required"}
	}
	if req.Period == "" {
		return nil, &errs.Error{Code: errs.InvalidArgument, Message: "period is required (e.g. \"2024-Q1\")"}
	}
	if req.FileKey == "" {
		return nil, &errs.Error{Code: errs.InvalidArgument, Message: "file_key is required — upload the file first via /files/upload"}
	}
	return statements.CreateStatement(ctx, &statements.CreateStatementRequest{
		Filename: req.Filename,
		Period:   req.Period,
		FileKey:  req.FileKey,
	})
}

// ListStatements returns all royalty statements for the organisation, newest first.
//
//encore:api auth method=GET path=/api/statements
func ListStatements(ctx context.Context) (*statements.ListStatementsResponse, error) {
	return statements.ListStatements(ctx)
}

// RunDetection triggers deviation detection for a statement. Evaluates every
// line against the STIM distribution key, flags deviations, and generates
// AI explanations. Returns the number of flags created.
//
//encore:api auth method=POST path=/api/statements/:id/run
func RunDetection(ctx context.Context, id int64) (*detectionsvc.RunDetectionResponse, error) {
	return detectionsvc.RunDetection(ctx, &detectionsvc.RunDetectionRequest{StatementID: id})
}

// =============================================================================
// Deviations
// =============================================================================

// ListDeviations returns deviation flags for the organisation.
// Optional query parameters: statement_id, severity (LOW|MEDIUM|HIGH|CRITICAL),
// status (open|resolved).
//
//encore:api auth method=GET path=/api/deviations
func ListDeviations(ctx context.Context, req *detectionsvc.ListFlagsRequest) (*detectionsvc.ListFlagsResponse, error) {
	return detectionsvc.ListFlags(ctx, req)
}

// GetDeviation returns a single deviation flag with its AI explanation and
// recommendation.
//
//encore:api auth method=GET path=/api/deviations/:id
func GetDeviation(ctx context.Context, id int64) (*detectionsvc.Flag, error) {
	return detectionsvc.GetFlag(ctx, &detectionsvc.GetFlagRequest{ID: id})
}

// =============================================================================
// Unmatched lines
// =============================================================================

// GetUnmatched returns lines from the latest detection run for a statement that
// could not be evaluated — unknown right type or missing controlled share.
//
//encore:api auth method=GET path=/api/statements/:id/unmatched
func GetUnmatched(ctx context.Context, id int64) (*detectionsvc.GetUnmatchedResponse, error) {
	return detectionsvc.GetUnmatched(ctx, &detectionsvc.GetUnmatchedRequest{StatementID: id})
}

// GetDetectionProgress returns the current phase and counts for a detection run.
// Poll every ~500 ms while phase is not "done" or "failed".
//
// Phases: reading | identifying | loading_key | checking_ratios | done | failed
//
//encore:api auth method=GET path=/api/statements/:id/detection-progress
func GetDetectionProgress(ctx context.Context, id int64) (*detectionsvc.ProgressResponse, error) {
	return detectionsvc.GetProgress(ctx, &detectionsvc.GetProgressRequest{StatementID: id})
}

// =============================================================================
// Admin
// =============================================================================

// AdminReset deletes all statements, lines, and deviation flags for the
// caller's organisation. Blocked in production — returns 403 if called there.
// Use this to get a blank canvas in staging or development.
//
//encore:api auth method=POST path=/api/admin/reset
func AdminReset(ctx context.Context) error {
	if encore.Meta().Environment.Type == encore.EnvProduction {
		return &errs.Error{Code: errs.PermissionDenied, Message: "reset is not available in production"}
	}
	statements.Reset(ctx)
	detectionsvc.Reset(ctx)
	return nil
}

// GenerateExplanation generates the AI explanation and next step for a single
// deviation flag. Idempotent — returns cached text if already generated.
// Never returns an error on AI failure; returns the flag with a fallback message
// and explanation_status="failed" so the frontend can show a retry button.
//
//encore:api auth method=POST path=/api/deviations/:id/explain
func GenerateExplanation(ctx context.Context, id int64) (*detectionsvc.Flag, error) {
	return detectionsvc.GenerateExplanation(ctx, &detectionsvc.GenerateExplanationRequest{FlagID: id})
}
