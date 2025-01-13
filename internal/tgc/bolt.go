package tgc

import (
	"os"
	"path/filepath"
	"time"

	"github.com/tgdrive/teldrive/internal/utils"
	"go.etcd.io/bbolt"
)

func NewBoltDB(sessionFile string) (*bbolt.DB, error) {
	if sessionFile == "" {
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
		sessionFile = filepath.Join(dir, "session.db")
	}
	db, err := bbolt.Open(sessionFile, 0666, &bbolt.Options{
		Timeout:    time.Second,
		NoGrowSync: false,
	})
	if err != nil {
		return nil, err
	}
	return db, nil

}
