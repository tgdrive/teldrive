package mapper

import (
	"github.com/divyam234/teldrive/pkg/models"
	"github.com/divyam234/teldrive/pkg/schemas"
)

func ToFileOut(file *models.File) schemas.FileOut {
	return schemas.FileOut{
		ID:        file.ID,
		Name:      file.Name,
		Type:      file.Type,
		MimeType:  file.MimeType,
		Path:      file.Path,
		Size:      *file.Size,
		Starred:   file.Starred,
		ParentID:  file.ParentID,
		UpdatedAt: file.UpdatedAt,
	}
}

func ToFileOutFull(file *models.File) *schemas.FileOutFull {
	parts := []schemas.Part{}
	for _, part := range *file.Parts {
		parts = append(parts, schemas.Part{
			ID: part.ID,
		})
	}

	return &schemas.FileOutFull{
		FileOut:   ToFileOut(file),
		Parts:     parts,
		ChannelID: *file.ChannelID,
	}
}

func ToUploadOut(in *models.Upload) *schemas.UploadPartOut {
	out := &schemas.UploadPartOut{
		ID:        in.ID,
		Name:      in.Name,
		PartId:    in.PartId,
		ChannelID: in.ChannelID,
		PartNo:    in.PartNo,
		Size:      in.Size,
	}
	return out
}
