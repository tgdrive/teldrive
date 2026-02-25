package events

import (
	"fmt"
	"strings"

	"github.com/tgdrive/teldrive/internal/api"
)

var validEventTypes = map[EventType]struct{}{
	api.EventTypeFilesCreated:    {},
	api.EventTypeFilesUpdated:    {},
	api.EventTypeFilesDeleted:    {},
	api.EventTypeFilesMoved:      {},
	api.EventTypeFilesCopied:     {},
	api.EventTypeUploadsProgress: {},
	api.EventTypeJobsProgress:    {},
}

var eventTypeGroups = map[string][]EventType{
	"files.*": {
		api.EventTypeFilesCreated,
		api.EventTypeFilesUpdated,
		api.EventTypeFilesDeleted,
		api.EventTypeFilesMoved,
		api.EventTypeFilesCopied,
	},
	"uploads.*": {
		api.EventTypeUploadsProgress,
	},
	"jobs.*": {
		api.EventTypeJobsProgress,
	},
}

func ParseEventTypes(filters []string) ([]EventType, error) {
	if len(filters) == 0 {
		return nil, nil
	}

	set := make(map[EventType]struct{})
	out := make([]EventType, 0, len(filters))

	for _, raw := range filters {
		for _, token := range strings.Split(raw, ",") {
			name := strings.TrimSpace(token)
			if name == "" {
				continue
			}

			if group, ok := eventTypeGroups[name]; ok {
				for _, eventType := range group {
					if _, exists := set[eventType]; exists {
						continue
					}
					set[eventType] = struct{}{}
					out = append(out, eventType)
				}
				continue
			}

			eventType := EventType(name)
			if _, ok := validEventTypes[eventType]; !ok {
				return nil, fmt.Errorf("invalid event type: %s", name)
			}

			if _, exists := set[eventType]; exists {
				continue
			}

			set[eventType] = struct{}{}
			out = append(out, eventType)
		}
	}

	if len(out) == 0 {
		return nil, nil
	}

	return out, nil
}
