package mapper

import (
	"github.com/tgdrive/teldrive/internal/api"
	"github.com/tgdrive/teldrive/internal/utils"
	"github.com/tgdrive/teldrive/pkg/models"
)

func ToFileOut(file models.File) *api.File {
	res := &api.File{
		ID:        api.NewOptString(file.ID),
		Name:      file.Name,
		Type:      api.FileType(file.Type),
		MimeType:  api.NewOptString(file.MimeType),
		Encrypted: api.NewOptBool(file.Encrypted),
		UpdatedAt: api.NewOptDateTime(file.UpdatedAt),
	}
	if file.ParentId != nil {
		res.ParentId = api.NewOptString(*file.ParentId)
	}
	if file.Size != nil {
		res.Size = api.NewOptInt64(*file.Size)
	}
	if file.Category != "" {
		res.Category = api.NewOptFileCategory(api.FileCategory(file.Category))
	}
	return res
}

func ToUploadOut(parts []models.Upload) []api.UploadPart {
	return utils.Map(parts, func(part models.Upload) api.UploadPart {
		return api.UploadPart{
			Name:      part.Name,
			PartId:    part.PartId,
			ChannelId: part.ChannelId,
			PartNo:    part.PartNo,
			Size:      part.Size,
			Encrypted: part.Encrypted,
			Salt:      api.NewOptString(part.Salt),
		}
	})
}
