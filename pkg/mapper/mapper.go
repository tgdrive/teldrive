package mapper

import (
	"github.com/tgdrive/teldrive/internal/api"
	"github.com/tgdrive/teldrive/pkg/models"
)

func ToFileOut(file models.File, extended bool) *api.File {
	res := &api.File{
		ID:        api.NewOptString(file.Id),
		Name:      file.Name,
		Type:      api.FileType(file.Type),
		MimeType:  api.NewOptString(file.MimeType),
		Encrypted: api.NewOptBool(file.Encrypted),
		ParentId:  api.NewOptString(file.ParentID.String),
		UpdatedAt: api.NewOptDateTime(file.UpdatedAt),
	}
	if file.Size != nil {
		res.Size = api.NewOptInt64(*file.Size)
	}
	if file.Category != "" {
		res.Category = api.NewOptFileCategory(api.FileCategory(file.Category))
	}
	if extended {
		res.Parts = file.Parts
		if file.ChannelID != nil {
			res.ChannelId = api.NewOptInt64(*file.ChannelID)
		}
	}
	return res
}

func ToUploadOut(parts []models.Upload) []api.UploadPart {
	res := []api.UploadPart{}
	for _, part := range parts {
		res = append(res, api.UploadPart{
			Name:      part.Name,
			PartId:    part.PartId,
			ChannelId: part.ChannelID,
			PartNo:    part.PartNo,
			Size:      part.Size,
			Encrypted: part.Encrypted,
			Salt:      api.NewOptString(part.Salt),
		})

	}
	return res
}
