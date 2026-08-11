using TypeSpec.Http;
using TypeSpec.OpenAPI;

namespace TelDrive;

enum UploadState {
  open: "open",
  completing: "completing",
  completed: "completed",
  aborted: "aborted",
  expired: "expired",
}

enum UploadPartState {
  reserved: "reserved",
  uploading: "uploading",
  stored: "stored",
  failed: "failed",
}

model UploadCreateRequest {
  parentId?: Uuid;

  @minLength(1)
  @maxLength(255)
  name: string;

  @minValue(0)
  size: int64;

  mimeType?: string;
  modTime: utcDateTime;
  hash?: FileHash;
  encryption?: boolean = false;
  conflictPolicy?: NameConflictPolicy = NameConflictPolicy.fail;

  /** Optional preferred part size. The server may choose a different value. */
  @minValue(1048576)
  preferredPartSize?: int64;
}

model UploadSession {
  id: Uuid;
  parentId?: Uuid;
  name: string;
  expectedSize: int64;
  expectedHash?: FileHash;
  mimeType?: string;
  modTime: utcDateTime;
  encryption: boolean;
  conflictPolicy: NameConflictPolicy;
  partSize: int64;
  state: UploadState;
  expiresAt: utcDateTime;
  createdAt: utcDateTime;
  completedAt?: utcDateTime;
  fileId?: Uuid;
}

model UploadPart {
  uploadId: Uuid;
  partNo: int32;
  state: UploadPartState;
  plainSize: int64;
  storedSize?: int64;
  checksum?: Checksum;
  createdAt: utcDateTime;
  updatedAt: utcDateTime;
}

model UploadDailyStatistics {
  date: plainDate;
  uploadedBytes: int64;
  completedFiles: int64;
}

enum UploadImportSourceType {
  local: "local",
  http: "http",
}

model UploadImportSource {
  type: UploadImportSourceType;
  path?: string;
  url?: url;
  headers?: Record<string>;
  destinationPath?: string;
  exclude?: string[];
}

model UploadImportRequest {
  /** Destination folder UUID or absolute drive path from root. */
  destination: string;
  sources: UploadImportSource[];
  headers?: Record<string>;
  exclude?: string[];
  minSize?: string;
  maxSize?: string;

  @minValue(1)
  @maxValue(16)
  partConcurrency?: int32 = 4;

  /** Preferred upload part size. Values are aligned to the nearest 16 MiB. */
  @minValue(67108864)
  @maxValue(2097152000)
  chunkSize?: int64 = 536870912;

  encryption?: boolean = false;
}

model UploadPartHeaders {
  @header("Content-Type")
  contentType: "application/octet-stream";

  @header("Content-Length")
  @minValue(0)
  contentLength: int64;

  /** BLAKE3 checksum of the plaintext part. */
  @header("X-Part-Checksum")
  checksum?: Checksum;
}

model UploadPartStoredResponse {
  @statusCode
  statusCode: 200 | 201;

  @body
  body: UploadPart;
}

model UploadCompletedResponse {
  @statusCode
  statusCode: 200 | 201;

  @header("Etag")
  ETag: ETag;

  @header("Location")
  location: Uri;

  @body
  body: FileEntry;
}

@tag("Uploads")
@route("/v1/uploads")
@useAuth(TelDriveAuth)
namespace Uploads {
  /** List resumable upload sessions owned by the authenticated user. */
  @get
  @operationId("listUploads")
  op list(...CursorQuery, @query state?: UploadState):
    | JsonResponse<200, CursorPage<UploadSession>>
    | ErrorResponse<401>
    | ErrorResponse<422>;

  /** Return completed upload volume grouped by UTC day. */
  @get
  @route("/statistics")
  @operationId("getUploadStatistics")
  op statistics(
    @query
    @minValue(1)
    @maxValue(366)
    days?: int32 = 30,
  ):
    | JsonResponse<200, UploadDailyStatistics[]>
    | ErrorResponse<401>
    | ErrorResponse<422>;

  /** Queue local paths, recursive folders, and HTTP URLs for server-side upload. */
  @post
  @route("/imports")
  @operationId("createUploadImport")
  op createImport(@body body: UploadImportRequest):
    | JsonResponse<202, Job>
    | ErrorResponse<400>
    | ErrorResponse<401>
    | ErrorResponse<422>
    | ErrorResponse<503>;

  /** Create a durable upload session. */
  @post
  @operationId("createUpload")
  op create(...IdempotencyHeader, @body body: UploadCreateRequest):
    | JsonResponse<201, UploadSession>
    | ErrorResponse<401>
    | ErrorResponse<404>
    | ErrorResponse<409>
    | ErrorResponse<413>
    | ErrorResponse<422>
    | ErrorResponse<429>;

  /** Inspect an upload for resume and conflict handling. */
  @get
  @route("/{uploadId}")
  @operationId("getUpload")
  op get(@path uploadId: Uuid):
    | JsonResponse<200, UploadSession>
    | ErrorResponse<401>
    | ErrorResponse<404>
    | ErrorResponse<410>;

  /** List parts already known to the upload session. */
  @get
  @route("/{uploadId}/parts")
  @operationId("listUploadParts")
  op listParts(...CursorQuery, @path uploadId: Uuid):
    | JsonResponse<200, CursorPage<UploadPart>>
    | ErrorResponse<401>
    | ErrorResponse<404>
    | ErrorResponse<410>;

  /**
   * Upload one part. The `(uploadId, partNo)` pair is the idempotency key.
   * An exact retry returns the existing stored part; conflicting size/checksum returns 409.
   */
  @put
  @route("/{uploadId}/parts/{partNo}")
  @operationId("putUploadPart")
  op putPart(
    ...UploadPartHeaders,
    @path uploadId: Uuid,
    @path @minValue(1) partNo: int32,
    @body body: bytes,
  ):
    | UploadPartStoredResponse
    | ErrorResponse<400>
    | ErrorResponse<401>
    | ErrorResponse<404>
    | ErrorResponse<409>
    | ErrorResponse<410>
    | ErrorResponse<413>
    | ErrorResponse<422>
    | ErrorResponse<429>
    | ErrorResponse<503>;

  /**
   * Validate all parts and transactionally publish the file. A successful response
   * guarantees immediate read-after-write visibility.
   */
  @post
  @route("/{uploadId}/complete")
  @operationId("completeUpload")
  op complete(...IdempotencyHeader, @path uploadId: Uuid):
    | UploadCompletedResponse
    | ErrorResponse<401>
    | ErrorResponse<404>
    | ErrorResponse<409>
    | ErrorResponse<410>
    | ErrorResponse<422>;

  /** Abort an upload and schedule physical Telegram cleanup. */
  @delete
  @route("/{uploadId}")
  @operationId("abortUpload")
  op abort(@path uploadId: Uuid):
    | EmptyResponse<204>
    | ErrorResponse<401>
    | ErrorResponse<404>
    | ErrorResponse<409>;
}
