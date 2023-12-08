package models

import (
	"database/sql/driver"
	"encoding/json"
	"time"
)

type File struct {
	ID        string    `gorm:"type:text;primaryKey;default:generate_uid(16)"`
	Name      string    `gorm:"type:text;not null"`
	Type      string    `gorm:"type:text;not null"`
	MimeType  string    `gorm:"type:text;not null"`
	Path      string    `gorm:"type:text;index"`
	Size      *int64    `gorm:"type:bigint"`
	Starred   bool      `gorm:"default:false"`
	Depth     *int      `gorm:"type:integer"`
	Encrypted bool      `gorm:"default:false"`
	UserID    int64     `gorm:"type:bigint;not null"`
	Status    string    `gorm:"type:text"`
	ParentID  string    `gorm:"type:text;index"`
	Parts     *Parts    `gorm:"type:jsonb"`
	ChannelID *int64    `gorm:"type:bigint"`
	CreatedAt time.Time `gorm:"default:timezone('utc'::text, now())"`
	UpdatedAt time.Time `gorm:"default:timezone('utc'::text, now())"`
}

type Parts []Part
type Part struct {
	ID   int64  `json:"id"`
	Salt string `json:"salt,omitempty"`
}

func (a Parts) Value() (driver.Value, error) {
	return json.Marshal(a)
}

func (a *Parts) Scan(value interface{}) error {
	if err := json.Unmarshal(value.([]byte), &a); err != nil {
		return err
	}
	return nil
}
