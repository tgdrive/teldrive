package models

import (
	"time"

	"gorm.io/datatypes"
)

type Event struct {
	ID        string                      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()()"`
	Type      string                      `gorm:"type:text;not null"`
	UserID    int64                       `gorm:"type:bigint"`
	Source    datatypes.JSONType[*Source] `gorm:"type:jsonb"`
	CreatedAt time.Time                   `gorm:"default:timezone('utc'::text, now())"`
}

type Source struct {
	ID           string `json:"id,omitempty"`
	Type         string `json:"type,omitempty"`
	Name         string `json:"name,omitempty"`
	ParentID     string `json:"parentId,omitempty"`
	DestParentID string `json:"destParentId,omitempty"`
}
