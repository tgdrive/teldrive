package models

import (
	"database/sql/driver"
	"encoding/json"
	"time"
)

type File struct {
	ID        string    `gorm:"type:text;primary_key;default:generate_uid(16)"`
	Name      string    `gorm:"type:text"`
	Type      string    `gorm:"type:text"`
	MimeType  string    `gorm:"type:text"`
	Path      string    `gorm:"index;type:text"`
	Size      int64     `gorm:"type:bigint"`
	Starred   *bool     `gorm:"default:false"`
	Depth     *int      `gorm:"type:integer"`
	UserID    int       `gorm:"type:bigint"`
	ParentID  string    `gorm:"index;type:text"`
	FilePart  FilePart  `gorm:"foreignKey:FileId;references:id;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
	CreatedAt time.Time `gorm:"default:timezone('utc'::text, now())"`
	UpdatedAt time.Time `gorm:"default:timezone('utc'::text, now())"`
}

type FilePart struct {
	ID        int    `gorm:"type:serial4;primary_key"`
	FileId    string `gorm:"type:text"`
	Parts     *Parts `gorm:"type:jsonb"`
	ChannelID int64  `gorm:"type:bigint"`
}

type Parts []Part
type Part struct {
	ID int64 `json:"id"`
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
