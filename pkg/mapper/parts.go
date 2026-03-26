package mapper

import (
	"github.com/tgdrive/teldrive/internal/api"
	dbtypes "github.com/tgdrive/teldrive/internal/database/types"
)

func ToAPIParts(parts *dbtypes.JSONB[dbtypes.Parts]) []api.Part {
	if parts == nil || len(parts.Data) == 0 {
		return nil
	}

	out := make([]api.Part, 0, len(parts.Data))
	for _, part := range parts.Data {
		item := api.Part{ID: part.ID}
		if part.Salt != "" {
			item.Salt = api.NewOptString(part.Salt)
		}
		out = append(out, item)
	}

	return out
}

func ToDBParts(parts []api.Part) dbtypes.Parts {
	if len(parts) == 0 {
		return nil
	}

	out := make(dbtypes.Parts, 0, len(parts))
	for _, part := range parts {
		out = append(out, dbtypes.Part{ID: part.ID, Salt: part.Salt.Or("")})
	}

	return out
}

func ToDBPartsJSONB(parts []api.Part) *dbtypes.JSONB[dbtypes.Parts] {
	if len(parts) == 0 {
		return nil
	}

	jsonb := dbtypes.NewJSONB(ToDBParts(parts))
	return &jsonb
}
