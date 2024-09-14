package models

import (
	"database/sql"
	"time"

	"github.com/tgdrive/teldrive/pkg/schemas"
	"gorm.io/datatypes"
)

type File struct {
	Id        string                            `gorm:"type:uuid;primaryKey;default:uuid7()"`
	Name      string                            `gorm:"type:text;not null"`
	Type      string                            `gorm:"type:text;not null"`
	MimeType  string                            `gorm:"type:text;not null"`
	Size      *int64                            `gorm:"type:bigint"`
	Category  string                            `gorm:"type:text"`
	Encrypted bool                              `gorm:"default:false"`
	UserID    int64                             `gorm:"type:bigint;not null"`
	Status    string                            `gorm:"type:text"`
	ParentID  sql.NullString                    `gorm:"type:uuid;index"`
	Parts     datatypes.JSONSlice[schemas.Part] `gorm:"type:jsonb"`
	ChannelID *int64                            `gorm:"type:bigint"`
	CreatedAt time.Time                         `gorm:"default:timezone('utc'::text, now())"`
	UpdatedAt time.Time                         `gorm:"default:timezone('utc'::text, now())"`
}
