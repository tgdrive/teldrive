package services

import (
	"testing"

	"github.com/divyam234/teldrive/internal/database"
	"github.com/gin-gonic/gin"

	"github.com/divyam234/teldrive/internal/utils"
	"github.com/divyam234/teldrive/pkg/models"
	"github.com/divyam234/teldrive/pkg/schemas"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"
)

type FileServiceSuite struct {
	suite.Suite
	db  *gorm.DB
	srv *FileService
}

func (s *FileServiceSuite) SetupSuite() {
	s.db = database.NewTestDatabase(s.T(), false)
	s.srv = NewFileService(s.db, nil, nil)
}

func (s *FileServiceSuite) SetupTest() {
	s.srv.db.Where("id is not NULL").Delete(&models.File{})
	s.srv.db.Create(&models.File{
		Name:     "root",
		Type:     "folder",
		MimeType: "drive/folder",
		Path:     "/",
		Depth:    utils.IntPointer(0),
		UserID:   123456,
		Status:   "active",
		ParentID: "root",
	})
}

func (s *FileServiceSuite) entry(name string) *schemas.FileIn {
	return &schemas.FileIn{
		Name:      name,
		Type:      "file",
		Parts:     []schemas.Part{},
		MimeType:  "image/jpeg",
		Path:      "/",
		ChannelID: 123456,
		Size:      121531,
		Encrypted: false,
	}
}

func TestSuite(t *testing.T) {
	suite.Run(t, new(FileServiceSuite))
}

func (s *FileServiceSuite) TestSave() {
	res, err := s.srv.CreateFile(&gin.Context{}, 123456, s.entry("file.jpeg"))
	s.NoError(err.Error)
	find, err := s.srv.GetFileByID(res.Id)
	s.NoError(err.Error)
	s.Equal(find.Id, res.Id)
	s.Equal(find.MimeType, res.MimeType)
}

func (s *FileServiceSuite) TestSave_Duplicate() {
	c := &gin.Context{}
	_, err := s.srv.CreateFile(c, 123456, s.entry("file1.jpeg"))
	s.NoError(err.Error)

	_, err = s.srv.CreateFile(c, 123456, s.entry("file1.jpeg"))
	s.Error(err.Error)
	s.Equal(database.ErrKeyConflict, err)
}

func (s *FileServiceSuite) Test_Update() {

	res, err := s.srv.CreateFile(&gin.Context{}, 123456, s.entry("file2.jpeg"))
	s.NoError(err.Error)
	data := &schemas.FileUpdate{
		Name: "file3.jpeg",
		Path: "/dwkd",
		Type: "file",
	}
	r, err := s.srv.UpdateFile(res.Id, 123456, data, nil)
	s.NoError(err.Error)
	s.Equal(r.Name, data.Name)
	s.Equal(r.Path, data.Path)
}

func (s *FileServiceSuite) Test_NoFound() {
	_, err := s.srv.GetFileByID("kj2ei28bdkj")
	s.Error(err.Error)
	s.Equal(err, database.ErrNotFound)
}
