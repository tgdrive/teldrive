package schemas

import (
	"time"

	"gorm.io/datatypes"
)

type Part struct {
	ID   int64  `json:"id"`
	Salt string `json:"salt,omitempty"`
}

type FileQuery struct {
	Name       string `form:"name"`
	Query      string `form:"query"`
	SearchType string `form:"searchType"`
	Type       string `form:"type"`
	Path       string `form:"path"`
	Op         string `form:"op"`
	DeepSearch bool   `form:"deepSearch"`
	Shared     *bool  `form:"shared"`
	ParentID   string `form:"parentId"`
	Category   string `form:"category"`
	UpdatedAt  string `form:"updatedAt"`
	Sort       string `form:"sort"`
	Order      string `form:"order"`
	Limit      int    `form:"limit"`
	Page       int    `form:"page"`
}

type FileIn struct {
	Name      string `json:"name" binding:"required"`
	Type      string `json:"type" binding:"required"`
	Parts     []Part `json:"parts,omitempty"`
	MimeType  string `json:"mimeType"`
	ChannelID int64  `json:"channelId"`
	Path      string `json:"path" binding:"required"`
	Size      int64  `json:"size"`
	ParentID  string `json:"parentId"`
	Encrypted bool   `json:"encrypted"`
}

type FileOut struct {
	Id         string    `json:"id"`
	Name       string    `json:"name"`
	Type       string    `json:"type"`
	MimeType   string    `json:"mimeType"`
	Category   string    `json:"category,omitempty"`
	Encrypted  bool      `json:"encrypted"`
	Size       int64     `json:"size,omitempty"`
	ParentID   string    `json:"parentId,omitempty"`
	ParentPath string    `json:"parentPath,omitempty"`
	UpdatedAt  time.Time `json:"updatedAt,omitempty"`
	Total      int       `json:"total,omitempty"`
}

type FileOutFull struct {
	*FileOut
	Parts     datatypes.JSONSlice[Part] `json:"parts,omitempty"`
	ChannelID *int64                    `json:"channelId,omitempty"`
	Path      string                    `json:"path,omitempty"`
}

type FileUpdate struct {
	Name      string    `json:"name,omitempty"`
	UpdatedAt time.Time `json:"updatedAt,omitempty"`
	Parts     []Part    `json:"parts,omitempty"`
	Size      *int64    `json:"size,omitempty"`
}

type Meta struct {
	Count       int `json:"count,omitempty"`
	TotalPages  int `json:"totalPages,omitempty"`
	CurrentPage int `json:"currentPage,omitempty"`
}
type FileResponse struct {
	Files []FileOut `json:"files"`
	Meta  Meta      `json:"meta"`
}

type FileOperation struct {
	Files       []string `json:"files"  binding:"required"`
	Destination string   `json:"destination,omitempty"`
}
type DeleteOperation struct {
	Files  []string `json:"files,omitempty"`
	Source string   `json:"source,omitempty"`
}
type PartUpdate struct {
	Parts     []Part    `json:"parts"`
	UploadId  string    `json:"uploadId"`
	UpdatedAt time.Time `json:"updatedAt" binding:"required"`
	Size      int64     `json:"size"`
	ChannelID int64     `json:"channelId"`
}

type DirMove struct {
	Source      string `json:"source" binding:"required"`
	Destination string `json:"destination" binding:"required"`
}

type MkDir struct {
	Path string `json:"path" binding:"required"`
}

type Copy struct {
	ID          string `json:"id" binding:"required"`
	Name        string `json:"name" binding:"required"`
	Destination string `json:"destination" binding:"required"`
}

type FileCategoryStats struct {
	TotalFiles int    `json:"totalFiles"`
	TotalSize  int    `json:"totalSize"`
	Category   string `json:"category"`
}

type FileShareIn struct {
	Password  string     `json:"password,omitempty"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
}

type FileShareOut struct {
	ID        string     `json:"id,omitempty"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
	Protected bool       `json:"protected"`
	UserID    int64      `json:"userId,omitempty"`
	Type      string     `json:"type"`
	Name      string     `json:"name"`
}

type FileShare struct {
	Password  *string
	ExpiresAt *time.Time
	Type      string
	FileID    string
	UserID    int64
	Path      string
	Name      string
}
