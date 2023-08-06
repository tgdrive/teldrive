package models

import (
	"time"

	"github.com/jackc/pgtype"
)

type File struct {
	ID        string       `gorm:"type:text;primary_key;default:generate_uid(16)"`
	Name      string       `gorm:"type:text"`
	Type      string       `gorm:"type:text"`
	Parts     pgtype.JSONB `gorm:"type:jsonb"`
	MimeType  string       `gorm:"type:text"`
	ChannelID int64        `gorm:"type:bigint"`
	Path      string       `gorm:"index;type:text"`
	Size      int64        `gorm:"type:bigint"`
	Starred   bool         `gorm:"default:false"`
	Depth     int          `gorm:"type:integer"`
	UserID    int          `gorm:"type:bigint"`
	ParentID  string       `gorm:"index;type:text"`
	CreatedAt time.Time    `gorm:"default:timezone('utc'::text, now())"`
	UpdatedAt time.Time    `gorm:"default:timezone('utc'::text, now())"`
}
