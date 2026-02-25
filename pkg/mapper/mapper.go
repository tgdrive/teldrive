package mapper

import (
	"github.com/tgdrive/teldrive/internal/api"
	jetmodel "github.com/tgdrive/teldrive/internal/database/jetgen/teldrive_jet/teldrive/model"
	"github.com/tgdrive/teldrive/internal/utils"
)

func ToJetFileOut(file jetmodel.Files) *api.File {
	res := &api.File{
		ID:        api.NewOptString(file.ID.String()),
		Name:      file.Name,
		Type:      api.FileType(file.Type),
		MimeType:  api.NewOptString(file.MimeType),
		UpdatedAt: api.NewOptDateTime(file.UpdatedAt),
		Encrypted: api.NewOptBool(file.Encrypted),
	}
	if file.ParentID != nil {
		res.ParentId = api.NewOptString(file.ParentID.String())
	}
	if file.Size != nil {
		res.Size = api.NewOptInt64(*file.Size)
	}
	if file.Category != nil && *file.Category != "" {
		res.Category = api.NewOptCategory(api.Category(*file.Category))
	}
	if file.Hash != nil && *file.Hash != "" {
		res.Hash = api.NewOptString(*file.Hash)
	}
	if file.ChannelID != nil {
		res.ChannelId = api.NewOptInt64(*file.ChannelID)
	}

	return res
}

func ToUploadOut(parts []jetmodel.Uploads) []api.UploadPart {
	return utils.Map(parts, func(part jetmodel.Uploads) api.UploadPart {
		res := api.UploadPart{
			Name:      part.Name,
			PartId:    int(part.PartID),
			ChannelId: part.ChannelID,
			PartNo:    int(part.PartNo),
			Size:      part.Size,
			Encrypted: part.Encrypted,
		}
		if part.Salt != nil {
			res.Salt = api.NewOptString(*part.Salt)
		}
		// Note: BlockHashes are internal, not exposed in API response
		return res
	})
}
