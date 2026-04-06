package mapper

import (
	"github.com/google/uuid"
	"github.com/tgdrive/teldrive/internal/api"
	jetmodel "github.com/tgdrive/teldrive/internal/database/jet/gen/model"
	"github.com/tgdrive/teldrive/internal/utils"
)

func ToJetFileOut(file jetmodel.Files) *api.File {
	res := &api.File{
		ID:        api.NewOptUUID(api.UUID(file.ID)),
		Name:      file.Name,
		Type:      api.FileType(file.Type),
		Parts:     ToAPIParts(file.Parts),
		MimeType:  api.NewOptString(file.MimeType),
		UpdatedAt: api.NewOptDateTime(file.UpdatedAt),
		Encrypted: api.NewOptBool(file.Encrypted),
	}
	if file.ParentID != nil {
		res.ParentId = api.NewOptUUID(api.UUID(*file.ParentID))
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

func UUIDFromString(id string) api.UUID {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return api.UUID{}
	}

	return api.UUID(parsed)
}

func OptUUIDFromString(id string) api.OptUUID {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return api.OptUUID{}
	}

	return api.NewOptUUID(api.UUID(parsed))
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
