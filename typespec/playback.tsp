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
  ...FileContentRequestOptions,
): FileContentResponse | FilePartialContentResponse | EmptyResponse<304> | ErrorResponse<401> | ErrorResponse<404> | ErrorResponse<416> | ErrorResponse<422> | ErrorResponse<429> | ErrorResponse<503>;
