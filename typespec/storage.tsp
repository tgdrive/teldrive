using TypeSpec.Http;
using TypeSpec.OpenAPI;

namespace TelDrive;

model StorageSummary {
  logicalBytes: int64;
  activeFiles: int64;
  activeFolders: int64;
  trashedFiles: int64;
  trashBytes: int64;
}

model StorageGrowthPoint {
  day: plainDate;
  addedBytes: int64;
  logicalBytes: int64;
}

model StorageChannelStatistic {
  channelId: int64;
  name: string;
  selected: boolean;
  health: string;
  lastCheckedAt?: utcDateTime;
  partCount: int64;
  storedBytes: int64;
}

model StorageCleanupStatistics {
  trashBytes: int64;
  staleUploadBytes: int64;
  staleUploads: int64;
  totalReclaimableBytes: int64;
}

model StorageActivity {
  id: int64;
  type: string;
  resourceType: string;
  resourceId?: string;
  label: string;
  occurredAt: utcDateTime;
}

model StorageDashboard {
  summary: StorageSummary;
  growth: StorageGrowthPoint[];
  categories: FileCategoryStatistics[];
  channels: StorageChannelStatistic[];
  cleanup: StorageCleanupStatistics;
  activity: StorageActivity[];
}

@tag("Storage")
@route("/v1/storage")
@useAuth(TelDriveAuth)
namespace Storage {
  /** Return the authenticated user's storage statistics. */
  @get
  @route("/stats")
  @operationId("getStorageStats")
  op stats(): JsonResponse<200, StorageDashboard> | ErrorResponse<401>;
}
