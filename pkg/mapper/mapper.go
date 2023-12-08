package mapper

import (
	"github.com/divyam234/teldrive/pkg/models"
	"github.com/divyam234/teldrive/pkg/schemas"
)

func ToFileOut(file models.File) schemas.FileOut {
	var size int64
	if file.Size != nil {
		size = *file.Size
	}
	return schemas.FileOut{
		ID:        file.ID,
		Name:      file.Name,
		Type:      file.Type,
		MimeType:  file.MimeType,
		Path:      file.Path,
		Size:      size,
		Starred:   file.Starred,
		ParentID:  file.ParentID,
		UpdatedAt: file.UpdatedAt,
	}
}

func ToFileOutFull(file models.File) *schemas.FileOutFull {
	parts := []schemas.Part{}
	for _, part := range *file.Parts {
		parts = append(parts, schemas.Part{
			ID:   part.ID,
			Salt: part.Salt,
		})
	}

	return &schemas.FileOutFull{
		FileOut:   ToFileOut(file),
		Parts:     parts,
		ChannelID: *file.ChannelID,
		Encrypted: file.Encrypted,
	}
}

func ToUploadOut(in *models.Upload) *schemas.UploadPartOut {
	out := &schemas.UploadPartOut{
		Name:      in.Name,
		PartId:    in.PartId,
		ChannelID: in.ChannelID,
		PartNo:    in.PartNo,
		Size:      in.Size,
		Encrypted: in.Encrypted,
		Salt:      in.Salt,
	}
	return out
}
