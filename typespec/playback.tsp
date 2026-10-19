using TypeSpec.Http;
using TypeSpec.OpenAPI;
namespace TelDrive;

model PlaybackSession {
  ticket: string;
  expiresAt: utcDateTime;
  conversionAvailable: boolean;
}

@tag("Files")
@route("/v1/files/{fileId}/playback")
@useAuth(TelDriveAuth)
@post
@operationId("createPlayback")
op createPlayback(@path fileId: Uuid): JsonResponse<201, PlaybackSession> | ErrorResponse<401> | ErrorResponse<404> | ErrorResponse<429>;

@tag("Public shares")
@route("/v1/public/shares/{token}/files/{fileId}/playback")
@post
@operationId("createPublicPlayback")
op createPublicPlayback(@path token: string, @path fileId: Uuid, ...SharePasswordHeader): JsonResponse<201, PlaybackSession> | ErrorResponse<401> | ErrorResponse<404> | ErrorResponse<429>;

@tag("Files")
@route("/v1/playback")
@get
@operationId("streamPlayback")
op streamPlayback(
  @query @minLength(32) @maxLength(128) ticket: string,
  @query mode?: "original" | "audio" | "video" = "original",
  @query @minValue(0) @maxValue(86400) start?: float64 = 0,
  @query @minValue(0) @maxValue(1024) audio?: int32,
  @query @minValue(0) @maxValue(1024) subtitle?: int32,
  ...FileContentRequestOptions,
): FileContentResponse | FilePartialContentResponse | EmptyResponse<304> | ErrorResponse<401> | ErrorResponse<404> | ErrorResponse<416> | ErrorResponse<422> | ErrorResponse<429> | ErrorResponse<503>;

model MediaTrack {
  index: int32;
  kind: "video" | "audio" | "subtitle";
  codec: string;
  language: string;
  title: string;
  default: boolean;
  bitmap: boolean;
}
model MediaTracks { available: boolean; tracks: MediaTrack[]; }

model StreamAccountStatistics {
  id: int64;
  name: string;
  `alias`: string;
  kind: "bot" | "user";
  bytes: int64;
  bytesPerSecond: float64;
  requests: int64;
  errors: int64;
  activeRequests: int32;
  latencyMs: float64;
  channelId: int64;
  messageId: int64;
  offset: int64;
  lastSeen?: utcDateTime;
}
model PlaybackStatistics {
  scope: "file";
  fileId: Uuid;
  name: string;
  mimeType: string;
  size: int64;
  encrypted: boolean;
  sampledAt: utcDateTime;
  startedAt?: utcDateTime;
  activeStreams: int32;
  bytes: int64;
  bytesPerSecond: float64;
  accounts: StreamAccountStatistics[];
}
@tag("Files") @useAuth(TelDriveAuth) @route("/v1/files/{fileId}/playback/statistics") @get @operationId("getFilePlaybackStatistics")
op getFilePlaybackStatistics(@path fileId: Uuid): JsonResponse<200, PlaybackStatistics> | ErrorResponse<401> | ErrorResponse<403> | ErrorResponse<404>;

@tag("Files") @route("/v1/playback/tracks") @get @operationId("getPlaybackTracks")
op getPlaybackTracks(@query @minLength(32) @maxLength(128) ticket: string): JsonResponse<200, MediaTracks> | ErrorResponse<401> | ErrorResponse<404> | ErrorResponse<422> | ErrorResponse<503>;

@tag("Files") @route("/v1/playback/subtitles") @get @operationId("getPlaybackSubtitles")
op getPlaybackSubtitles(@query @minLength(32) @maxLength(128) ticket: string, @query @minValue(0) @maxValue(1024) track: int32, @query @minValue(0) @maxValue(86400) start?: float64 = 0): FileContentResponse | ErrorResponse<401> | ErrorResponse<404> | ErrorResponse<422> | ErrorResponse<503>;

model CastingDevice { id: string; name: string; protocol: "dlna"; }
model CastingDevices { devices: CastingDevice[]; }
model CastingRelay { addresses: string[]; expiresAt: utcDateTime; }
model CastingStart {
  ticket: string;
  serverUrl: string;
  mode: "original" | "video";
  @minValue(0) @maxValue(86400) start?: float64 = 0;
  @minValue(0) @maxValue(1024) audio?: int32;
  @minValue(0) @maxValue(1024) subtitle?: int32;
}
@tag("Files") @useAuth(TelDriveAuth) @route("/v1/casting/relay") @post @operationId("createCastingRelay")
op createCastingRelay(@body body: { @minLength(32) @maxLength(128) ticket: string; }): JsonResponse<201, CastingRelay> | ErrorResponse<401> | ErrorResponse<403> | ErrorResponse<404> | ErrorResponse<503>;
@tag("Files") @useAuth(TelDriveAuth) @route("/v1/casting/devices") @get @operationId("listCastingDevices")
op listCastingDevices(): JsonResponse<200, CastingDevices> | ErrorResponse<401> | ErrorResponse<403> | ErrorResponse<503>;
@tag("Files") @useAuth(TelDriveAuth) @route("/v1/casting/devices/{deviceId}/play") @post @operationId("playOnCastingDevice")
op playOnCastingDevice(@path deviceId: string, @body body: CastingStart): EmptyResponse<204> | ErrorResponse<400> | ErrorResponse<401> | ErrorResponse<403> | ErrorResponse<404> | ErrorResponse<422> | ErrorResponse<503>;
@tag("Files") @useAuth(TelDriveAuth) @route("/v1/casting/devices/{deviceId}/control") @post @operationId("controlCastingDevice")
op controlCastingDevice(@path deviceId: string, @body body: { command: "play" | "pause" | "stop"; }): EmptyResponse<204> | ErrorResponse<401> | ErrorResponse<403> | ErrorResponse<404> | ErrorResponse<503>;
