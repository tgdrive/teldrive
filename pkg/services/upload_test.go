package services

import (
	"testing"

	"github.com/divyam234/teldrive/internal/database"

	"github.com/divyam234/teldrive/pkg/models"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"
)

type UploadServiceSuite struct {
	suite.Suite
	db  *gorm.DB
	srv *UploadService
}

func (s *UploadServiceSuite) SetupSuite() {
	s.db = database.NewTestDatabase(s.T(), false)
	s.srv = NewUploadService(s.db, nil, nil, nil, nil)
}

func (s *UploadServiceSuite) SetupTest() {
	s.srv.db.Where("id is not NULL").Delete(&models.Upload{})
}

func TestUploadSuite(t *testing.T) {
	suite.Run(t, new(UploadServiceSuite))
}
