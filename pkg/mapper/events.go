package mapper

import (
	"encoding/json"
	"time"

	"github.com/tgdrive/teldrive/internal/api"
	jetmodel "github.com/tgdrive/teldrive/internal/database/jet/gen/model"
	"github.com/tgdrive/teldrive/pkg/dto"
)

func ToEventOut(item jetmodel.Events) api.Event {
	src := &dto.Source{}
	if item.Source != nil {
		_ = json.Unmarshal([]byte(*item.Source), src)
	}

	return eventOut(item.ID.String(), item.Type, item.CreatedAt, src)
}

func ToEventOutFromDTO(event dto.Event) api.Event {
	return eventOut(event.ID, event.Type, event.CreatedAt, event.Source)
}

func eventOut(id string, typ string, createdAt time.Time, src *dto.Source) api.Event {
	if src == nil {
		src = &dto.Source{}
	}

	return api.Event{
		ID:        UUIDFromString(id),
		Type:      api.EventType(typ),
		CreatedAt: createdAt,
		Source: api.Source{
			ID:           UUIDFromString(src.ID),
			Type:         api.SourceType(src.Type),
			Name:         src.Name,
			ParentId:     UUIDFromString(src.ParentID),
			DestParentId: OptUUIDFromString(src.DestParentID),
		},
	}
}
