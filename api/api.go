// Package api exposes all public REST endpoints consumed by the Lovable frontend.
// It is a thin layer: validate input, delegate to the statements or detection service,
// return the response. No business logic lives here.
package api

import (
	"context"

	"encore.dev/beta/errs"

	detectionsvc "encore.app/detection"
	"encore.app/statements"
)

// =============================================================================
// CWR catalogue import
// =============================================================================

type ProcessCWRRequest struct {
	FileKey string `json:"file_key"`
}

// ProcessCWR downloads an uploaded CWR file, parses it, and stores the works
// and writers into the publisher catalogue. Call /files/upload first to obtain
// the file_key.
//
//encore:api auth method=POST path=/api/cwr
func ProcessCWR(ctx context.Context, req *ProcessCWRRequest) (*statements.ProcessCWRResponse, error) {
	if req.FileKey == "" {
		return nil, &errs.Error{Code: errs.InvalidArgument, Message: "file_key is required"}
	}
	return statements.ProcessCWR(ctx, &statements.ProcessCWRRequest{FileKey: req.FileKey})
}

// =============================================================================
// Works catalogue
// =============================================================================

// ListWorks returns all registered catalogue works for the organisation.
//
//encore:api auth method=GET path=/api/works
func ListWorks(ctx context.Context) (*statements.ListWorksResponse, error) {
	return statements.ListWorks(ctx)
}

// =============================================================================
// Statements
// =============================================================================

type CreateStatementRequest struct {
	Filename string `json:"filename"`
	Period   string `json:"period"`
}

// CreateStatement registers an uploaded STIM statement file. Call /files/upload
// first; the returned filename and the billing period are the only required fields.
//
//encore:api auth method=POST path=/api/statements
func CreateStatement(ctx context.Context, req *CreateStatementRequest) (*statements.Statement, error) {
	if req.Filename == "" {
		return nil, &errs.Error{Code: errs.InvalidArgument, Message: "filename is required"}
	}
	if req.Period == "" {
		return nil, &errs.Error{Code: errs.InvalidArgument, Message: "period is required (e.g. \"2024-Q1\")"}
	}
	return statements.CreateStatement(ctx, &statements.CreateStatementRequest{
		Filename: req.Filename,
		Period:   req.Period,
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
