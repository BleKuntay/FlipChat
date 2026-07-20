package attachment

type Metadata struct {
	AttachmentID string `json:"attachment_id"`
	ObjectKey    string `json:"object_key"`
	Filename     string `json:"filename"`
	MIMEType     string `json:"mime_type"`
	Size         int64  `json:"size"`
	UploaderID   string `json:"uploader_id"`
}

type UploadResponse struct {
	AttachmentID string `json:"attachment_id"`
	ObjectKey    string `json:"object_key"`
	Filename     string `json:"filename"`
	MIMEType     string `json:"mime_type"`
	Size         int64  `json:"size"`
	UploaderID   string `json:"uploader_id"`
}
