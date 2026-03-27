package services

import (
	"encoding/json"

	"github.com/tgdrive/teldrive/internal/api"
	"github.com/tgdrive/teldrive/pkg/queue"
)

func syncArgsUpdateToArgs(update api.PeriodicJobUpdateArgs, current api.SyncArgs) (api.SyncArgs, error) {
	currentJSON, err := current.MarshalJSON()
	if err != nil {
		return api.SyncArgs{}, err
	}
	currentMap := map[string]any{}
	if err := json.Unmarshal(currentJSON, &currentMap); err != nil {
		return api.SyncArgs{}, err
	}

	updateJSON, err := json.Marshal(update)
	if err != nil {
		return api.SyncArgs{}, err
	}
	updateMap := map[string]any{}
	if err := json.Unmarshal(updateJSON, &updateMap); err != nil {
		return api.SyncArgs{}, err
	}

	merged := mergeObjectMaps(currentMap, updateMap)
	mergedJSON, err := json.Marshal(merged)
	if err != nil {
		return api.SyncArgs{}, err
	}
	var out api.SyncArgs
	if err := out.UnmarshalJSON(mergedJSON); err != nil {
		return api.SyncArgs{}, err
	}
	return out, nil
}

func mergeObjectMaps(base, update map[string]any) map[string]any {
	merged := make(map[string]any, len(base)+len(update))
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range update {
		updateMap, updateIsMap := value.(map[string]any)
		baseMap, baseIsMap := merged[key].(map[string]any)
		if updateIsMap && baseIsMap {
			merged[key] = mergeObjectMaps(baseMap, updateMap)
			continue
		}
		merged[key] = value
	}
	return merged
}

func toQueueFilters(v api.OptSyncFilters) queue.SyncFilters {
	if !v.IsSet() {
		return queue.SyncFilters{}
	}
	out := queue.SyncFilters{Include: v.Value.Include, Exclude: v.Value.Exclude, ExcludeIfPresent: v.Value.ExcludeIfPresent}
	if v.Value.MinSize.IsSet() {
		out.MinSize = v.Value.MinSize.Value
	}
	if v.Value.MaxSize.IsSet() {
		out.MaxSize = v.Value.MaxSize.Value
	}
	return out
}

func toQueueOptions(v api.OptSyncOptions) queue.SyncOptions {
	out := queue.SyncOptions{PartSize: 100 * 1024 * 1024, Sync: true}
	if !v.IsSet() {
		return out
	}
	if v.Value.PartSize.IsSet() && v.Value.PartSize.Value > 0 {
		out.PartSize = v.Value.PartSize.Value
	}
	if v.Value.Sync.IsSet() {
		out.Sync = v.Value.Sync.Value
	}
	return out
}
