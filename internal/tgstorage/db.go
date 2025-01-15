package tgstorage

import (
	"os"
	"path/filepath"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/go-faster/errors"
	"github.com/tgdrive/teldrive/internal/utils"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func NewDatabase(storageFile string) (*gorm.DB, error) {
	if storageFile == "" {
		dir, err := os.UserHomeDir()
		if err != nil {
			dir = utils.ExecutableDir()
		} else {
			dir = filepath.Join(dir, ".teldrive")
			err := os.Mkdir(dir, 0755)
			if err != nil && !os.IsExist(err) {
				dir = utils.ExecutableDir()
			}
		}
		storageFile = filepath.Join(dir, "storage.db")
	}

	db, err := gorm.Open(sqlite.Open(storageFile), &gorm.Config{NowFunc: func() time.Time {
		return time.Now().UTC()
	}, Logger: logger.Default.LogMode(logger.Silent)})

	if err != nil {
		return nil, err
	}
	return db, nil

}

func MigrateDB(db *gorm.DB) error {
	if err := db.AutoMigrate(&KeyValue{}); err != nil {
		return errors.Wrap(err, "auto migrate")
	}
	return nil
}
