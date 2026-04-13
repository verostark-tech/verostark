package files

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	encoreauth "encore.dev/beta/auth"
	"encore.dev/beta/errs"
	"encore.dev/storage/objects"

	authsvc "encore.app/auth"
)

const maxUploadSize = 50 << 20 // 50 MB

// Uploads is the shared object storage bucket for all uploaded files.
// Other services that need to read uploaded files should reference this directly.
var Uploads = objects.NewBucket("verostark-uploads", objects.BucketConfig{})

// UploadResult is returned after a successful upload.
type UploadResult struct {
	Key      string `json:"key"`
	Filename string `json:"filename"`
	Size     int64  `json:"size"`
	MIMEType string `json:"mime_type"`
}

// Upload accepts a multipart file upload, validates the format, and stores
// the file in object storage. Returns the storage key for use by the
// statements service when creating a statement record.
//
//encore:api auth raw method=POST path=/files/upload
func Upload(w http.ResponseWriter, req *http.Request) {
	data := encoreauth.Data().(*authsvc.AuthData)

	if err := req.ParseMultipartForm(maxUploadSize); err != nil {
		errs.HTTPError(w, &errs.Error{
			Code:    errs.InvalidArgument,
			Message: "file exceeds the 50 MB limit or the request is malformed",
		})
		return
	}

	file, header, err := req.FormFile("file")
	if err != nil {
		errs.HTTPError(w, &errs.Error{
			Code:    errs.InvalidArgument,
			Message: "no file found in request — send the file under the field name \"file\"",
		})
		return
	}
	defer file.Close()

	// Read the first 512 bytes for MIME sniffing, then reconstruct the reader.
	sniff := make([]byte, 512)
	n, err := file.Read(sniff)
	if err != nil && err != io.EOF {
		errs.HTTPError(w, &errs.Error{Code: errs.Internal, Message: "could not read file"})
		return
	}
	sniff = sniff[:n]

	mimeType, err := validate(header.Filename, sniff)
	if err != nil {
		errs.HTTPError(w, err)
		return
	}

	// Reconstruct the full reader: sniffed bytes + remainder.
	fullReader := io.MultiReader(strings.NewReader(string(sniff)), file)

	key := storageKey(data.OrgID, header.Filename)
	writer := Uploads.Upload(req.Context(), key)

	size, err := io.Copy(writer, fullReader)
	if err != nil {
		writer.Abort(err)
		errs.HTTPError(w, &errs.Error{Code: errs.Internal, Message: "upload failed — please try again"})
		return
	}

	if err := writer.Close(); err != nil {
		errs.HTTPError(w, &errs.Error{Code: errs.Internal, Message: "upload failed — please try again"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(UploadResult{
		Key:      key,
		Filename: header.Filename,
		Size:     size,
		MIMEType: mimeType,
	})
}

// storageKey returns a namespaced object key: {orgID}/{random16hex}{ext}
func storageKey(orgID, filename string) string {
	var b [8]byte
	rand.Read(b[:])
	ext := strings.ToLower(filepath.Ext(filename))
	return orgID + "/" + hex.EncodeToString(b[:]) + ext
}
