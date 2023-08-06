package database

import (
	"os"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

var DB *gorm.DB

func InitDB() {

	var err error

	DB, err = gorm.Open(postgres.Open(os.Getenv("DATABASE_URL")), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{
			TablePrefix:   "teldrive.",
			SingularTable: false,
		},
		PrepareStmt: false,
		NowFunc: func() time.Time {
			return time.Now().UTC()
		},
	})
	if err != nil {
		panic(err)
	}

	sqlDB, err := DB.DB()
	if err != nil {
		panic(err)
	}
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)

	sqlDB.SetConnMaxLifetime(time.Hour)

	go func() {
		DB.Exec("select 1")
	}()

	// Auto-migrate the File struct to the database
	//db.AutoMigrate(&File{})

}
