// Package statements manages the publisher catalogue (works, writers) and
// uploaded royalty statements. CWR import populates the catalogue; statement
// records are created after each file upload and consumed by the detection service.
package statements

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	encoreauth "encore.dev/beta/auth"
	"encore.dev/beta/errs"
	"encore.dev/storage/sqldb"

	authsvc "encore.app/auth"
	"encore.app/cwr"
	filessvc "encore.app/files"
	"encore.app/validators"
)

var db = sqldb.NewDatabase("statements", sqldb.DatabaseConfig{
	Migrations: "./migrations",
})

// --- Domain types ---

// Statement is an uploaded royalty statement file.
type Statement struct {
	ID        int64     `json:"id"`
	OrgID     string    `json:"org_id"`
	Filename  string    `json:"filename"`
	Period    string    `json:"period"`
	PRO       string    `json:"pro"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// StatementLine is one row from a parsed royalty statement.
// GrossAmount is nullable — populated once the STIM CSV parser is implemented.
type StatementLine struct {
	ID          int64    `json:"id"`
	OrgID       string   `json:"org_id"`
	StatementID int64    `json:"statement_id"`
	WorkRef     string   `json:"work_ref"`
	ISWC        string   `json:"iswc"`
	Territory   string   `json:"territory"`
	RightType   string   `json:"right_type"`
	Source      string   `json:"source"`
	NetAmount   float64  `json:"net_amount"`
	GrossAmount *float64 `json:"gross_amount,omitempty"`
	Currency    string   `json:"currency"`
	Period      string   `json:"period"`
}

// Work is a registered catalogue work (imported from CWR).
type Work struct {
	ID          int64     `json:"id"`
	OrgID       string    `json:"org_id"`
	Title       string    `json:"title"`
	ISWC        string    `json:"iswc"`
	InternalRef string    `json:"internal_ref"`
	CreatedAt   time.Time `json:"created_at"`
}

// WorkMatch is the result of matching a statement line to a catalogue work.
// ControlledShare is the summed manuscript share across all controlled writers.
type WorkMatch struct {
	WorkID          int64   `json:"work_id"`
	Title           string  `json:"title"`
	ControlledShare float64 `json:"controlled_share"`
}

// --- Request / response types ---

type ProcessCWRRequest struct {
	FileKey string `json:"file_key"`
}

type ProcessCWRResponse struct {
	WorksStored   int `json:"works_stored"`
	WritersStored int `json:"writers_stored"`
}

type CreateStatementRequest struct {
	Filename string `json:"filename"`
	Period   string `json:"period"`
	FileKey  string `json:"file_key"`
}

type ListStatementsResponse struct {
	Statements []Statement `json:"statements"`
}

type GetStatementRequest struct {
	ID int64 `json:"id"`
}

type ListStatementLinesRequest struct {
	StatementID int64 `json:"statement_id"`
}

type ListStatementLinesResponse struct {
	Lines []StatementLine `json:"lines"`
}

type GetWorkForLineRequest struct {
	ISWC string `json:"iswc"`
}

type UpdateStatementStatusRequest struct {
	ID     int64  `json:"id"`
	Status string `json:"status"`
}

type ListWorksResponse struct {
	Works []Work `json:"works"`
}

// --- Private API ---

// ProcessCWR downloads a previously uploaded CWR file, parses it, and stores
// all works and writers into the catalogue. Idempotent for works with ISWCs
// already in the catalogue — existing works are reused, not duplicated.
//
//encore:api private
func ProcessCWR(ctx context.Context, req *ProcessCWRRequest) (*ProcessCWRResponse, error) {
	data := encoreauth.Data().(*authsvc.AuthData)
	orgID := data.OrgID

	r := filessvc.Uploads.Download(ctx, req.FileKey)
	defer r.Close()

	records, _ := cwr.ParseFile(r)
	if len(records) == 0 {
		return nil, &errs.Error{
			Code:    errs.InvalidArgument,
			Message: "no works found in the file — check that it is a valid CWR file",
		}
	}

	return storeWorks(ctx, orgID, records)
}

// storeWorks persists parsed CWR records. Called only by ProcessCWR.
func storeWorks(ctx context.Context, orgID string, records []cwr.WorkRecord) (*ProcessCWRResponse, error) {
	var worksStored, writersStored int

	for _, rec := range records {
		iswc := ""
		if rec.Work.ISWC != "" {
			iswc = validators.NormaliseISWC(rec.Work.ISWC)
		}

		// Look up existing work by ISWC; insert if not found.
		var workID int64
		if iswc != "" {
			db.QueryRow(ctx,
				`SELECT id FROM works WHERE org_id=$1 AND iswc=$2`,
				orgID, iswc,
			).Scan(&workID)
		}
		if workID == 0 {
			if err := db.QueryRow(ctx,
				`INSERT INTO works (org_id, title, iswc, internal_ref)
				 VALUES ($1, $2, $3, $4) RETURNING id`,
				orgID, rec.Work.Title, iswc, rec.Work.SubmitterRef,
			).Scan(&workID); err != nil {
				continue
			}
			worksStored++
		}

		// Insert each writer and the work_writer link.
		for _, w := range rec.Writers {
			ipiName := validators.NormaliseIPIName(w.IPIName)

			var writerID int64
			db.QueryRow(ctx,
				`SELECT id FROM writers WHERE org_id=$1 AND ipi_name_number=$2`,
				orgID, ipiName,
			).Scan(&writerID)

			if writerID == 0 {
				if err := db.QueryRow(ctx,
					`INSERT INTO writers (org_id, name, ipi_name_number, ipi_base_number, is_controlled)
					 VALUES ($1, $2, $3, $4, true) RETURNING id`,
					orgID,
					w.LastName+" "+w.FirstName,
					ipiName,
					validators.NormaliseIPIBase(w.IPIBase),
				).Scan(&writerID); err != nil {
					continue
				}
				writersStored++
			}

			// Skip if this work_writer link already exists.
			var linkExists bool
			db.QueryRow(ctx,
				`SELECT EXISTS(SELECT 1 FROM work_writers WHERE org_id=$1 AND work_id=$2 AND writer_id=$3)`,
				orgID, workID, writerID,
			).Scan(&linkExists)
			if linkExists {
				continue
			}

			db.Exec(ctx,
				`INSERT INTO work_writers (org_id, work_id, writer_id, manuscript_share, controlled_share)
				 VALUES ($1, $2, $3, $4, $4)`,
				orgID, workID, writerID, w.ManuscriptShare,
			)
		}
	}

	return &ProcessCWRResponse{WorksStored: worksStored, WritersStored: writersStored}, nil
}

// CreateStatement registers a new statement upload record and parses its lines.
// FileKey must be the storage key returned by /files/upload.
//
//encore:api private
func CreateStatement(ctx context.Context, req *CreateStatementRequest) (*Statement, error) {
	data := encoreauth.Data().(*authsvc.AuthData)
	orgID := data.OrgID

	if req.FileKey == "" {
		return nil, &errs.Error{Code: errs.InvalidArgument, Message: "file_key is required — upload the file first via /files/upload"}
	}

	r := filessvc.Uploads.Download(ctx, req.FileKey)
	defer r.Close()

	lines, err := parseSTIM(r)
	if err != nil {
		return nil, err
	}

	var s Statement
	if err := db.QueryRow(ctx,
		`INSERT INTO statements (org_id, filename, period, pro, status)
		 VALUES ($1, $2, $3, 'STIM', 'pending')
		 RETURNING id, org_id, filename, period, pro, status, created_at`,
		orgID, req.Filename, req.Period,
	).Scan(&s.ID, &s.OrgID, &s.Filename, &s.Period, &s.PRO, &s.Status, &s.CreatedAt); err != nil {
		return nil, &errs.Error{Code: errs.Internal, Message: "could not register statement"}
	}

	for _, line := range lines {
		db.Exec(ctx,
			`INSERT INTO statement_lines
			    (org_id, statement_id, work_ref, iswc, source, right_type,
			     net_amount, gross_amount, currency, period)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'SEK',$9)`,
			orgID, s.ID, line.WorkRef, line.ISWC, line.Source, line.RightType,
			line.NetAmount, line.GrossAmount, s.Period,
		)
	}

	return &s, nil
}

// ListStatements returns all statements for the org, newest first.
//
//encore:api private
func ListStatements(ctx context.Context) (*ListStatementsResponse, error) {
	data := encoreauth.Data().(*authsvc.AuthData)
	orgID := data.OrgID

	rows, err := db.Query(ctx,
		`SELECT id, org_id, filename, period, pro, status, created_at
		 FROM statements WHERE org_id=$1
		 ORDER BY created_at DESC`,
		orgID,
	)
	if err != nil {
		return nil, &errs.Error{Code: errs.Internal, Message: "could not load statements"}
	}
	defer rows.Close()

	var out []Statement
	for rows.Next() {
		var s Statement
		rows.Scan(&s.ID, &s.OrgID, &s.Filename, &s.Period, &s.PRO, &s.Status, &s.CreatedAt)
		out = append(out, s)
	}
	return &ListStatementsResponse{Statements: out}, nil
}

// GetStatement returns a single statement, verified against the caller's org.
//
//encore:api private
func GetStatement(ctx context.Context, req *GetStatementRequest) (*Statement, error) {
	data := encoreauth.Data().(*authsvc.AuthData)
	orgID := data.OrgID

	var s Statement
	err := db.QueryRow(ctx,
		`SELECT id, org_id, filename, period, pro, status, created_at
		 FROM statements WHERE id=$1 AND org_id=$2`,
		req.ID, orgID,
	).Scan(&s.ID, &s.OrgID, &s.Filename, &s.Period, &s.PRO, &s.Status, &s.CreatedAt)
	if err != nil {
		return nil, &errs.Error{Code: errs.NotFound, Message: "statement not found"}
	}
	return &s, nil
}

// ListStatementLines returns all lines for a statement, verified against the caller's org.
//
//encore:api private
func ListStatementLines(ctx context.Context, req *ListStatementLinesRequest) (*ListStatementLinesResponse, error) {
	data := encoreauth.Data().(*authsvc.AuthData)
	orgID := data.OrgID

	rows, err := db.Query(ctx,
		`SELECT id, org_id, statement_id, work_ref, iswc, territory, right_type, source,
		        net_amount, gross_amount, currency, period
		 FROM statement_lines WHERE statement_id=$1 AND org_id=$2`,
		req.StatementID, orgID,
	)
	if err != nil {
		return nil, &errs.Error{Code: errs.Internal, Message: "could not load statement lines"}
	}
	defer rows.Close()

	var out []StatementLine
	for rows.Next() {
		var l StatementLine
		rows.Scan(&l.ID, &l.OrgID, &l.StatementID, &l.WorkRef, &l.ISWC, &l.Territory,
			&l.RightType, &l.Source, &l.NetAmount, &l.GrossAmount, &l.Currency, &l.Period)
		out = append(out, l)
	}
	return &ListStatementLinesResponse{Lines: out}, nil
}

// GetWorkForLine matches a statement line's ISWC to a catalogue work and returns
// the summed controlled manuscript share across all controlled writers.
//
//encore:api private
func GetWorkForLine(ctx context.Context, req *GetWorkForLineRequest) (*WorkMatch, error) {
	data := encoreauth.Data().(*authsvc.AuthData)
	orgID := data.OrgID

	iswc := validators.NormaliseISWC(req.ISWC)

	var m WorkMatch
	err := db.QueryRow(ctx,
		`SELECT w.id, w.title, COALESCE(SUM(ww.controlled_share), 0)
		 FROM works w
		 LEFT JOIN work_writers ww ON ww.work_id = w.id AND ww.org_id = $1
		 WHERE w.org_id = $1 AND w.iswc = $2
		 GROUP BY w.id, w.title`,
		orgID, iswc,
	).Scan(&m.WorkID, &m.Title, &m.ControlledShare)
	if err != nil {
		return nil, &errs.Error{Code: errs.NotFound, Message: "work not found in catalogue"}
	}
	return &m, nil
}

// ListWorks returns all catalogue works for the org.
//
//encore:api private
func ListWorks(ctx context.Context) (*ListWorksResponse, error) {
	data := encoreauth.Data().(*authsvc.AuthData)
	orgID := data.OrgID

	rows, err := db.Query(ctx,
		`SELECT id, org_id, title, iswc, internal_ref, created_at
		 FROM works WHERE org_id=$1
		 ORDER BY title`,
		orgID,
	)
	if err != nil {
		return nil, &errs.Error{Code: errs.Internal, Message: "could not load works"}
	}
	defer rows.Close()

	var out []Work
	for rows.Next() {
		var w Work
		rows.Scan(&w.ID, &w.OrgID, &w.Title, &w.ISWC, &w.InternalRef, &w.CreatedAt)
		out = append(out, w)
	}
	return &ListWorksResponse{Works: out}, nil
}

// UpdateStatementStatus sets the processing status on a statement.
//
//encore:api private
func UpdateStatementStatus(ctx context.Context, req *UpdateStatementStatusRequest) error {
	data := encoreauth.Data().(*authsvc.AuthData)
	orgID := data.OrgID

	_, err := db.Exec(ctx,
		`UPDATE statements SET status=$1 WHERE id=$2 AND org_id=$3`,
		req.Status, req.ID, orgID,
	)
	if err != nil {
		return &errs.Error{Code: errs.Internal, Message: "could not update statement status"}
	}
	return nil
}

// parseSTIM parses a STIM Sweden CSV royalty statement.
//
// SYNTHETIC DATA: This parser was built against the synthetic test fixture
// synthetic_statement_MEC_2025Q1.csv. Real STIM statements may differ in
// column names, ordering, encoding, or value formats. Validate against the
// official STIM file specification before using in production.
//
// Expected columns (order-independent, matched by header name):
//
//	Work ID | Title | Source | Right Type | Gross | ISRC |
//	Controlled by Publisher (%) | Interested Party | Role |
//	Manuscript Share (%) | Amount before fee | Fee (%) | Fee Amount | Net Amount
//
// Right Type values: "M" → mechanical, "P" → performance.
// The ISRC column is used as the work identifier (stored in the iswc field)
// since STIM does not include ISWC in this format.
func parseSTIM(r io.Reader) ([]StatementLine, error) {
	reader := csv.NewReader(r)
	reader.TrimLeadingSpace = true

	header, err := reader.Read()
	if err != nil {
		return nil, &errs.Error{Code: errs.InvalidArgument, Message: "could not read CSV header"}
	}

	idx := make(map[string]int, len(header))
	for i, h := range header {
		idx[strings.TrimSpace(h)] = i
	}

	for _, col := range []string{"Work ID", "Gross", "Right Type", "Net Amount", "ISRC"} {
		if _, ok := idx[col]; !ok {
			return nil, &errs.Error{
				Code:    errs.InvalidArgument,
				Message: fmt.Sprintf("missing required column %q — is this a STIM CSV file?", col),
			}
		}
	}

	sourceIdx, hasSource := idx["Source"]

	var lines []StatementLine
	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, &errs.Error{Code: errs.InvalidArgument, Message: "malformed CSV row"}
		}

		gross, err := strconv.ParseFloat(strings.TrimSpace(row[idx["Gross"]]), 64)
		if err != nil {
			continue
		}
		net, err := strconv.ParseFloat(strings.TrimSpace(row[idx["Net Amount"]]), 64)
		if err != nil {
			continue
		}

		rt := strings.ToUpper(strings.TrimSpace(row[idx["Right Type"]]))
		switch rt {
		case "M":
			rt = "mechanical"
		case "P":
			rt = "performance"
		default:
			rt = strings.ToLower(rt)
		}

		source := ""
		if hasSource {
			source = strings.TrimSpace(row[sourceIdx])
		}

		lines = append(lines, StatementLine{
			WorkRef:     strings.TrimSpace(row[idx["Work ID"]]),
			ISWC:        strings.TrimSpace(row[idx["ISRC"]]),
			Source:      source,
			RightType:   rt,
			NetAmount:   net,
			GrossAmount: &gross,
		})
	}

	if len(lines) == 0 {
		return nil, &errs.Error{Code: errs.InvalidArgument, Message: "no statement lines found in the file"}
	}
	return lines, nil
}
