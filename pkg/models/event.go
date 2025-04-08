package models

import (
	"time"

	"gorm.io/datatypes"
)

type Event struct {
	ID        string                         `gorm:"type:uuid;primaryKey;default:gen_random_uuid()()"`
	Type      string                         `gorm:"type:text;not null"`
	UserID    int64                          `gorm:"type:bigint"`
	Data      datatypes.JSONType[*EventData] `gorm:"type:jsonb"`
	CreatedAt time.Time                      `gorm:"default:timezone('utc'::text, now())"`
}

type EventData struct {
	FileID      string `json:"id,omitempty"`
	FolderID    string `json:"folderId,omitempty"`
	OldFolderID string `json:"oldFolderId,omitempty"`
}
