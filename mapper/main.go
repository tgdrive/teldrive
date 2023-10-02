package mapper

import (
	"github.com/divyam234/teldrive/models"
	"github.com/divyam234/teldrive/schemas"
)

func MapFileToFileOut(file models.File) schemas.FileOut {
	return schemas.FileOut{
		ID:        file.ID,
		Name:      file.Name,
		Type:      file.Type,
		MimeType:  file.MimeType,
		Path:      file.Path,
		Size:      file.Size,
		Starred:   file.Starred,
		ParentID:  file.ParentID,
		UpdatedAt: file.UpdatedAt,
	}
}

func MapFileInToFile(file schemas.FileIn) models.File {
	return models.File{
		Name:      file.Name,
		Type:      file.Type,
		MimeType:  file.MimeType,
		Path:      file.Path,
		Size:      file.Size,
		Starred:   file.Starred,
		Depth:     file.Depth,
		UserID:    file.UserID,
		ParentID:  file.ParentID,
		Parts:     file.Parts,
		ChannelID: &file.ChannelID,
		Status:    file.Status,
	}
}

func MapFileToFileOutFull(file models.File) *schemas.FileOutFull {
	return &schemas.FileOutFull{
		FileOut: MapFileToFileOut(file),
		Parts:   file.Parts, ChannelID: file.ChannelID,
	}
}

func MapUploadSchema(in *models.Upload) *schemas.UploadPartOut {
	out := &schemas.UploadPartOut{
		ID:         in.ID,
		Name:       in.Name,
		PartId:     in.PartId,
		ChannelID:  in.ChannelID,
		PartNo:     in.PartNo,
		TotalParts: in.TotalParts,
		Size:       in.Size,
	}
	return out
}
