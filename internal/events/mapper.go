package events

import (
	"encoding/json"

	"github.com/google/uuid"
	jetmodel "github.com/tgdrive/teldrive/internal/database/jet/gen/model"
	"github.com/tgdrive/teldrive/pkg/dto"
)

func eventToModel(evt dto.Event) (*jetmodel.Events, error) {
	id, err := uuid.Parse(evt.ID)
	if err != nil {
		return nil, err
	}

	source, err := sourceToJSON(evt.Source)
	if err != nil {
		return nil, err
	}

	return &jetmodel.Events{
		ID:        id,
		Type:      evt.Type,
		UserID:    evt.UserID,
		Source:    source,
		CreatedAt: evt.CreatedAt,
	}, nil
}

func eventFromModel(evt jetmodel.Events) dto.Event {
	return dto.Event{
		ID:        evt.ID.String(),
		Type:      evt.Type,
		UserID:    evt.UserID,
		Source:    sourceFromJSON(evt.Source),
		CreatedAt: evt.CreatedAt,
	}
}

func sourceToJSON(source *dto.Source) (*string, error) {
	if source == nil {
		return nil, nil
	}

	b, err := json.Marshal(source)
	if err != nil {
		return nil, err
	}

	v := string(b)
	return &v, nil
}

func sourceFromJSON(raw *string) *dto.Source {
	if raw == nil {
		return &dto.Source{}
	}

	source := &dto.Source{}
	if err := json.Unmarshal([]byte(*raw), source); err != nil {
		return &dto.Source{}
	}

	return source
}
