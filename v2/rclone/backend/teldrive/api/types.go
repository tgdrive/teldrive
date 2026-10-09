// Package api provides request and response types for the TelDrive v2 REST API.
package api

import "time"

type ErrorDetail struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

type ErrorEnvelope struct {
	Error ErrorDetail `json:"error"`
}

type FileHash struct {
	Algorithm string `json:"algorithm"`
	Value     string `json:"value"`
}

type FileInfo struct {
	Id         string    `json:"id"`
	ParentId   string    `json:"parentId,omitempty"`
	Name       string    `json:"name"`
	Kind       string    `json:"kind"`
	MimeType   string    `json:"mimeType,omitempty"`
	Size       int64     `json:"size,omitempty"`
	Hash       *FileHash `json:"hash,omitempty"`
	Encryption bool      `json:"encryption"`
	Status     string    `json:"status"`
	ModTime    time.Time `json:"modTime"`
	Generation int64     `json:"generation"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type ReadMetadataResponse struct {
	Files      []FileInfo `json:"items"`
	NextCursor string     `json:"nextCursor,omitempty"`
}

type MetadataRequestOptions struct {
	Cursor string
	Limit  int64
	Status string
}

type FolderCreateRequest struct {
	ParentId       string     `json:"parentId,omitempty"`
	Name           string     `json:"name"`
	ConflictPolicy string     `json:"conflictPolicy,omitempty"`
	ModTime        *time.Time `json:"modTime,omitempty"`
}

type MoveFileRequest struct {
	ParentId       string `json:"parentId,omitempty"`
	ConflictPolicy string `json:"conflictPolicy,omitempty"`
}

type UpdateFileInformation struct {
	Name    string     `json:"name,omitempty"`
	ModTime *time.Time `json:"modTime,omitempty"`
}

type FileCopy struct {
	ParentId       string `json:"parentId,omitempty"`
	Name           string `json:"name,omitempty"`
	ConflictPolicy string `json:"conflictPolicy,omitempty"`
}

type UserProfile struct {
	UserId      int64     `json:"userId"`
	DisplayName string    `json:"displayName,omitempty"`
	Username    string    `json:"username,omitempty"`
	Premium     bool      `json:"premium"`
	CreatedAt   time.Time `json:"createdAt"`
}

type UploadCreateRequest struct {
	ParentId          string    `json:"parentId,omitempty"`
	Name              string    `json:"name"`
	Size              int64     `json:"size"`
	MimeType          string    `json:"mimeType,omitempty"`
	ModTime           time.Time `json:"modTime"`
	Hash              *FileHash `json:"hash,omitempty"`
	Encryption        bool      `json:"encryption,omitempty"`
	ConflictPolicy    string    `json:"conflictPolicy,omitempty"`
	PreferredPartSize int64     `json:"preferredPartSize,omitempty"`
}

type UploadSession struct {
	ID             string     `json:"id"`
	ParentId       string     `json:"parentId,omitempty"`
	Name           string     `json:"name"`
	ExpectedSize   int64      `json:"expectedSize"`
	ExpectedHash   *FileHash  `json:"expectedHash,omitempty"`
	MimeType       string     `json:"mimeType,omitempty"`
	ModTime        time.Time  `json:"modTime"`
	Encryption     bool       `json:"encryption"`
	ConflictPolicy string     `json:"conflictPolicy"`
	PartSize       int64      `json:"partSize"`
	State          string     `json:"state"`
	ExpiresAt      time.Time  `json:"expiresAt"`
	CreatedAt      time.Time  `json:"createdAt"`
	CompletedAt    *time.Time `json:"completedAt,omitempty"`
	FileId         string     `json:"fileId,omitempty"`
}

type UploadSessionPage struct {
	Items      []UploadSession `json:"items"`
	NextCursor string          `json:"nextCursor,omitempty"`
}

type UploadPart struct {
	UploadId  string `json:"uploadId"`
	PartNo    int    `json:"partNo"`
	State     string `json:"state"`
	PlainSize int64  `json:"plainSize"`
	Checksum  string `json:"checksum,omitempty"`
}

type UploadPartPage struct {
	Items      []UploadPart `json:"items"`
	NextCursor string       `json:"nextCursor,omitempty"`
}

type FileShare struct {
	ID        string     `json:"id"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
	RevokedAt *time.Time `json:"revokedAt,omitempty"`
}

type FileSharePage struct {
	Items      []FileShare `json:"items"`
	NextCursor string      `json:"nextCursor,omitempty"`
}

type FileShareCreated struct {
	ID                string     `json:"id"`
	FileID            string     `json:"fileId"`
	Token             string     `json:"token"`
	PublicURL         string     `json:"publicUrl"`
	PasswordProtected bool       `json:"passwordProtected"`
	ExpiresAt         *time.Time `json:"expiresAt,omitempty"`
}

type FileShareCreate struct {
	Password  string     `json:"password,omitempty"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
}

type CategorySize struct {
	Category   string `json:"category"`
	TotalFiles int64  `json:"totalFiles"`
	TotalSize  int64  `json:"totalSize"`
}

type EventPayload struct {
	ParentId string `json:"parentId,omitempty"`
	Name     string `json:"name,omitempty"`
	Kind     string `json:"kind,omitempty"`
	Status   string `json:"status,omitempty"`
	State    string `json:"state,omitempty"`
	FileId   string `json:"fileId,omitempty"`
}

type EventEnvelope struct {
	Version      int          `json:"version"`
	OccurredAt   time.Time    `json:"occurredAt"`
	ResourceType string       `json:"resourceType"`
	ResourceID   string       `json:"resourceId,omitempty"`
	Generation   *int64       `json:"generation,omitempty"`
	Payload      EventPayload `json:"payload"`
}
