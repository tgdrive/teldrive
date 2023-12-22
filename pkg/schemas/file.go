package schemas

import (
	"time"
)

type PaginationQuery struct {
	PerPage       int    `form:"perPage"`
	NextPageToken string `form:"nextPageToken"`
}

type SortingQuery struct {
	Sort  string `form:"sort"`
	Order string `form:"order"`
}

type Part struct {
	ID   int64  `json:"id"`
	Salt string `json:"salt"`
}

type FileQuery struct {
	Name      string     `form:"name" mapstructure:"name,omitempty"`
	Search    string     `form:"search" mapstructure:"search,omitempty"`
	Type      string     `form:"type" mapstructure:"type,omitempty"`
	Path      string     `form:"path" mapstructure:"path,omitempty"`
	Op        string     `form:"op" mapstructure:"op,omitempty"`
	Starred   *bool      `form:"starred" mapstructure:"starred,omitempty"`
	MimeType  string     `form:"mimeType" mapstructure:"mime_type,omitempty"`
	ParentID  string     `form:"parentId" mapstructure:"parent_id,omitempty"`
	UpdatedAt *time.Time `form:"updatedAt" mapstructure:"updated_at,omitempty"`
	Status    string     `mapstructure:"status"`
	UserID    int64      `mapstructure:"user_id"`
}

type CreateFile struct {
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
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Type       string    `json:"type"`
	MimeType   string    `json:"mimeType"`
	Path       string    `json:"path,omitempty"`
	Size       int64     `json:"size,omitempty"`
	Starred    bool      `json:"starred"`
	ParentID   string    `json:"parentId,omitempty"`
	ParentPath string    `json:"parentPath,omitempty"`
	UpdatedAt  time.Time `json:"updatedAt,omitempty"`
}

type FileOutFull struct {
	FileOut
	Parts     []Part `json:"parts,omitempty"`
	ChannelID int64  `json:"channelId"`
	Encrypted bool   `json:"encrypted"`
}

type UpdateFile struct {
	Name      string    `json:"name,omitempty"`
	Type      string    `json:"type,omitempty"`
	Path      string    `json:"path,omitempty"`
	Starred   *bool     `json:"starred,omitempty"`
	ParentID  string    `json:"parentId,omitempty"`
	UpdatedAt time.Time `json:"updatedAt,omitempty"`
}

type FileResponse struct {
	Results       []FileOut `json:"results"`
	NextPageToken string    `json:"nextPageToken,omitempty"`
}

type FileOperation struct {
	Files       []string `json:"files"  binding:"required"`
	Destination string   `json:"destination,omitempty"`
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
