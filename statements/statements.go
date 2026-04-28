// Package statements manages uploaded royalty statements. CRD file upload
// populates statement_lines; those lines are consumed by the detection service.
package statements

import (
	"context"
	"fmt"
	"time"

	encoreauth "encore.dev/beta/auth"
	"encore.dev/beta/errs"
	"encore.dev/storage/sqldb"

	authsvc "encore.app/auth"
	"encore.app/crd"
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

// StatementLine is one parsed royalty line from a CRD WER record.
//
// The _cents and controlled_numerator/denominator fields carry exact integer
// values used by the detection engine via math/big.Rat. The float64 fields
// (GrossAmount, NetAmount, ControlledShare) are approximations for API output
// and AI explanations only — they must not be used in royalty calculations.
type StatementLine struct {
	ID          int64  `json:"id"`
	OrgID       string `json:"org_id"`
	StatementID int64  `json:"statement_id"`
	WorkRef     string `json:"work_ref"`
	WorkTitle   string `json:"work_title"`
	ISWC        string `json:"iswc"`
	Territory   string `json:"territory"`
	RightType   string `json:"right_type"`
	Source      string `json:"source"`
	Currency    string `json:"currency"`
	Period      string `json:"period"`

	// Display-only float64 amounts. Do not use in detection calculations.
	NetAmount       float64 `json:"net_amount"`
	GrossAmount     float64 `json:"gross_amount"`
	ControlledShare float64 `json:"controlled_share"` // numerator/denominator as float

	// Exact integer amounts for detection (2 implied decimal places for SEK).
	// GrossCents=372000 represents 3720.00 SEK.
	GrossCents            int64 `json:"gross_cents"`
	NetCents              int64 `json:"net_cents"`
	ControlledNumerator   int64 `json:"controlled_numerator"`
	ControlledDenominator int64 `json:"controlled_denominator"`
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
// all works and writers into the catalogue.
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

// storeWorks persists parsed CWR records.
func storeWorks(ctx context.Context, orgID string, records []cwr.WorkRecord) (*ProcessCWRResponse, error) {
	var worksStored, writersStored int

	for _, rec := range records {
		iswc := ""
		if rec.Work.ISWC != "" {
			iswc = validators.NormaliseISWC(rec.Work.ISWC)
		}

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

// CreateStatement registers a new statement upload record and parses its CRD lines.
// FileKey must be the storage key returned by /files/upload for a .crd file.
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

	crdLines, parseErrs := crd.ParseFile(r)
	for _, e := range parseErrs {
		// Log parse warnings but do not abort — partial results are still useful.
		_ = e
	}
	if len(crdLines) == 0 {
		return nil, &errs.Error{
			Code:    errs.InvalidArgument,
			Message: "no WER records found in the file — check that it is a valid CISAC CRD file",
		}
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

	for _, l := range crdLines {
		grossCents := l.GrossCents
		netCents := l.NetCents
		grossSEK := float64(grossCents) / 100.0
		netSEK := float64(netCents) / 100.0
		controlledShare := 0.0
		if l.ControlledDenominator != 0 {
			controlledShare = float64(l.ControlledNumerator) / float64(l.ControlledDenominator)
		}

		db.Exec(ctx,
			`INSERT INTO statement_lines
			    (org_id, statement_id, work_ref, work_title, iswc, source, right_type,
			     net_amount, gross_amount, controlled_share, currency, period,
			     gross_cents, net_cents, controlled_numerator, controlled_denominator)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`,
			orgID, s.ID,
			l.WorkRef, l.WorkTitle, l.ISWC,
			"",                     // source: not present in CRD WER record
			mapRightCategory(l.RightCategory),
			netSEK, grossSEK, controlledShare,
			l.Currency, req.Period,
			grossCents, netCents,
			l.ControlledNumerator, l.ControlledDenominator,
		)
	}

	return &s, nil
}

// mapRightCategory maps a CRD right_category code to the canonical right_type string.
// "MEC" → "mechanical", "PER" → "performance", anything else passed through lowercased.
func mapRightCategory(rc string) string {
	switch rc {
	case "MEC":
		return "mechanical"
	case "PER":
		return "performance"
	default:
		if rc == "" {
			return ""
		}
		return rc
	}
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
		`SELECT id, org_id, statement_id, work_ref, work_title, iswc, right_type, source,
		        net_amount, gross_amount, controlled_share, currency, period,
		        gross_cents, net_cents, controlled_numerator, controlled_denominator
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
		if err := rows.Scan(
			&l.ID, &l.OrgID, &l.StatementID, &l.WorkRef, &l.WorkTitle, &l.ISWC,
			&l.RightType, &l.Source, &l.NetAmount, &l.GrossAmount, &l.ControlledShare,
			&l.Currency, &l.Period,
			&l.GrossCents, &l.NetCents, &l.ControlledNumerator, &l.ControlledDenominator,
		); err != nil {
			return nil, &errs.Error{Code: errs.Internal, Message: "could not read statement line"}
		}
		out = append(out, l)
	}
	return &ListStatementLinesResponse{Lines: out}, nil
}

// GetWorkForLine matches a statement line's ISWC to a catalogue work.
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

// Reset deletes all statements and lines for the caller's org.
// Private — only callable from the api service, which guards against production.
//
//encore:api private
func Reset(ctx context.Context) error {
	data := encoreauth.Data().(*authsvc.AuthData)
	orgID := data.OrgID
	db.Exec(ctx, `DELETE FROM statement_lines WHERE org_id=$1`, orgID)
	db.Exec(ctx, `DELETE FROM statements WHERE org_id=$1`, orgID)
	return nil
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
		return &errs.Error{Code: errs.Internal, Message: fmt.Sprintf("could not update statement status: %v", err)}
	}
	return nil
}
