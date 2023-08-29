package schemas

import (
	"time"

	"github.com/divyam234/teldrive/models"
)

type PaginationQuery struct {
	PerPage       int    `form:"perPage"`
	NextPageToken string `form:"nextPageToken"`
}

type SortingQuery struct {
	Sort  string `form:"sort"`
	Order string `form:"order"`
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

type FileIn struct {
	Name      string        `json:"name" mapstructure:"name,omitempty"`
	Type      string        `json:"type" mapstructure:"type,omitempty"`
	Parts     *models.Parts `json:"parts,omitempty" mapstructure:"parts,omitempty"`
	MimeType  string        `json:"mimeType" mapstructure:"mime_type,omitempty"`
	ChannelID *int64        `json:"channelId,omitempty" mapstructure:"channel_id,omitempty"`
	Path      string        `json:"path" mapstructure:"path,omitempty"`
	Size      int64         `json:"size" mapstructure:"size,omitempty"`
	Starred   *bool         `json:"starred" mapstructure:"starred,omitempty"`
	Depth     *int          `json:"depth,omitempty" mapstructure:"depth,omitempty"`
	Status    string        `mapstructure:"status,omitempty"`
	UserID    int64         `json:"userId" mapstructure:"user_id,omitempty"`
	ParentID  string        `json:"parentId" mapstructure:"parent_id,omitempty"`
}

type FileOut struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Type      string    `json:"type"`
	MimeType  string    `json:"mimeType" mapstructure:"mime_type"`
	Path      string    `json:"path,omitempty" mapstructure:"path,omitempty"`
	Size      int64     `json:"size,omitempty" mapstructure:"size,omitempty"`
	Starred   *bool     `json:"starred"`
	ParentID  string    `json:"parentId,omitempty" mapstructure:"parent_id"`
	UpdatedAt time.Time `json:"updatedAt,omitempty" mapstructure:"updated_at"`
}

type FileResponse struct {
	Results       []FileOut `json:"results"`
	NextPageToken string    `json:"nextPageToken,omitempty"`
}

type FileOutFull struct {
	FileOut
	Parts     *models.Parts `json:"parts,omitempty"`
	ChannelID *int64        `json:"channelId"`
}

type FileOperation struct {
	Files       []string `json:"files"`
	Destination string   `json:"destination,omitempty"`
}

type MkDir struct {
	Path string `json:"path"`
}

type FileShare struct {
	ID string `json:"id"`
}
