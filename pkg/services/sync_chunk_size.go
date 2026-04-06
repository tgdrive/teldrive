package services

const (
	maxSyncChunkSize     int64 = 2000 * 1024 * 1024
	defaultSyncChunkSize int64 = 512 * 1024 * 1024
	minSyncChunkSize     int64 = 64 * 1024 * 1024
	syncChunkBlockSize   int64 = 16 * 1024 * 1024
)

func normalizeSyncPartSize(size int64) int64 {
	if size <= 0 {
		return defaultSyncChunkSize
	}

	clamped := min(max(size, minSyncChunkSize), maxSyncChunkSize)
	aligned := ((clamped + syncChunkBlockSize/2) / syncChunkBlockSize) * syncChunkBlockSize
	return min(aligned, maxSyncChunkSize)
}
