package attachment

// UploadResponse is returned to the client after a successful upload.
// ObjectKey and UploaderID are intentionally omitted — they are internal
// details stored server-side in the upload record and must not leak.
type UploadResponse struct {
	AttachmentID string `json:"attachment_id"`
	Filename     string `json:"filename"`
	MIMEType     string `json:"mime_type"`
	Size         int64  `json:"size"`
}

// Metadata is used internally for the Download flow.
// ObjectKey is kept here because it is needed to fetch from MinIO,
// but it is never serialised into an HTTP response.
type Metadata struct {
	AttachmentID string `json:"attachment_id"`
	ObjectKey    string `json:"-"`
	Filename     string `json:"filename"`
	MIMEType     string `json:"mime_type"`
	Size         int64  `json:"size"`
}
