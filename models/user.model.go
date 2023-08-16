package models

import (
	"time"

	"github.com/divyam234/teldrive/utils"
	"gorm.io/gorm"
)

type User struct {
	UserId    int       `gorm:"type:int;primaryKey"`
	Name      string    `gorm:"type:text"`
	UserName  string    `gorm:"type:text"`
	IsPremium bool      `gorm:"type:bool"`
	TgSession string    `gorm:"type:text"`
	UpdatedAt time.Time `gorm:"default:timezone('utc'::text, now())"`
	CreatedAt time.Time `gorm:"default:timezone('utc'::text, now())"`
}

func (u *User) AfterCreate(tx *gorm.DB) (err error) {
	//create too folder on first signIn
	if u.UserId != 0 {
		file := File{
			Name:     "root",
			Type:     "folder",
			MimeType: "drive/folder",
			Path:     "/",
			Depth:    utils.IntPointer(0),
			UserID:   u.UserId,
			Status:   "active",
			ParentID: "root",
		}
		tx.Model(&File{}).Create(&file)
	}
	return
}
