package files

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"encore.dev/beta/errs"
)

// allowedExts is the complete set of accepted file extensions.
var allowedExts = map[string]bool{
	".txt":  true,
	".csv":  true,
	".cwr":  true,
	".pdf":  true,
	".xlsx": true,
}

// expectedMIMEPrefix maps each allowed extension to the MIME prefix
// that http.DetectContentType must return for the file to pass.
// xlsx is a ZIP-based format, so its detected MIME is application/zip.
// cwr files are plain ASCII text.
var expectedMIMEPrefix = map[string]string{
	".txt":  "text/plain",
	".csv":  "text/plain",
	".cwr":  "text/plain",
	".pdf":  "application/pdf",
	".xlsx": "application/zip",
}

// validate checks the filename extension and sniffs the first 512 bytes
// of content to confirm the file is what it claims to be.
// Returns the detected MIME type on success.
func validate(filename string, header []byte) (string, error) {
	ext := strings.ToLower(filepath.Ext(filename))

	if !allowedExts[ext] {
		return "", &errs.Error{
			Code:    errs.InvalidArgument,
			Message: fmt.Sprintf("file type %q is not supported — accepted formats: .txt, .csv, .cwr, .pdf, .xlsx", ext),
		}
	}

	detected := http.DetectContentType(header)
	expected := expectedMIMEPrefix[ext]
	if !strings.HasPrefix(detected, expected) {
		return "", &errs.Error{
			Code:    errs.InvalidArgument,
			Message: fmt.Sprintf("file content does not match extension %q — the file may be corrupt or mislabelled", ext),
		}
	}

	return detected, nil
}
