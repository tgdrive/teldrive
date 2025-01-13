package models

import (
	"time"

	"github.com/tgdrive/teldrive/internal/api"
	"gorm.io/datatypes"
)

type File struct {
	ID        string                        `gorm:"type:uuid;primaryKey;default:uuid7()"`
	Name      string                        `gorm:"type:text;not null"`
	Type      string                        `gorm:"type:text;not null"`
	MimeType  string                        `gorm:"type:text;not null"`
	Size      *int64                        `gorm:"type:bigint"`
	Category  string                        `gorm:"type:text"`
	Encrypted bool                          `gorm:"default:false"`
	UserId    int64                         `gorm:"type:bigint;not null"`
	Status    string                        `gorm:"type:text"`
	ParentId  *string                       `gorm:"type:uuid;index"`
	Parts     datatypes.JSONSlice[api.Part] `gorm:"type:jsonb"`
	ChannelId *int64                        `gorm:"type:bigint"`
	CreatedAt time.Time                     `gorm:"default:timezone('utc'::text, now())"`
	UpdatedAt time.Time                     `gorm:"autoUpdateTime:false"`
}
