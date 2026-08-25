using TypeSpec.Http;
using TypeSpec.OpenAPI;

namespace TelDrive;

enum FileKind {
  file: "file",
  folder: "folder",
}

enum FileStatus {
  active: "active",
  trashed: "trashed",
  deletionPending: "deletion_pending",
}

enum NameConflictPolicy {
  fail: "fail",
  replace: "replace",
  rename: "rename",
}

enum FileCategory {
  archive: "archive",
  audio: "audio",
  document: "document",
  image: "image",
  video: "video",
  other: "other",
}

enum FileSearchType {
  text: "text",
  regex: "regex",
}

enum FileSort {
  name: "name",
  updatedAt: "updatedAt",
  size: "size",
  id: "id",
}

enum SortOrder {
  ascending: "asc",
  descending: "desc",
}

model FileEntry {
  id: Uuid;
  parentId?: Uuid;
  name: string;
  kind: FileKind;
  mimeType?: string;
  size?: int64;
  hash?: FileHash;
  encryption: boolean;
  status: FileStatus;
  modTime: utcDateTime;
  generation: int64;
  createdAt: utcDateTime;
  updatedAt: utcDateTime;
}

enum ViewerKind {
  image: "image",
  video: "video",
  audio: "audio",
  pdf: "pdf",
  ebook: "ebook",
  text: "text",
}

model FileViewState {
  fileId: Uuid;
  kind: ViewerKind;
  position: Record<unknown>;
  preferences: Record<unknown>;
  bookmarks: Record<unknown>[];
  updatedAt: utcDateTime;
}

model FileViewStateUpdate {
  kind: ViewerKind;
  position?: Record<unknown>;
  preferences?: Record<unknown>;

  @maxItems(500)
  bookmarks?: Record<unknown>[];
}

model FileListQuery {
  @query
  parentId?: Uuid;

  /** Resolve the listing root relative to the user's drive root. */
  @query
  @maxLength(4096)
  path?: string;

  @query
  kind?: FileKind;

  @query
  status?: FileStatus = FileStatus.active;

  /** File-name search. Text search is case-insensitive; regex uses PostgreSQL-compatible regular expressions. */
  @query
  @maxLength(512)
  search?: string;

  @query
  searchType?: FileSearchType = FileSearchType.text;

  @query
  category?: FileCategory[];

  @query
  updatedAfter?: utcDateTime;

  @query
  updatedBefore?: utcDateTime;

  @query
  sort?: FileSort = FileSort.name;

  @query
  order?: SortOrder = SortOrder.ascending;
}

model FileBulkTrashRequest {
  @minItems(1)
  @maxItems(500)
  fileIds: Uuid[];
}

model FileBulkMoveRequest {
  @minItems(1)
  @maxItems(500)
  fileIds: Uuid[];

  parentId?: Uuid;
  conflictPolicy?: NameConflictPolicy = NameConflictPolicy.fail;
}

model FileBulkResult {
  items: FileEntry[];
}

model FileCategoryStatistics {
  category: FileCategory;
  totalFiles: int64;
  totalSize: int64;
}

model DriveStatistics {
  totalFiles: int64;
  totalFolders: int64;
  totalBytes: int64;
  trashedFiles: int64;
  activeShares: int64;
  openUploads: int64;
}

model FolderCreateRequest {
  parentId?: Uuid;

  @minLength(1)
  @maxLength(255)
  name: string;

  conflictPolicy?: NameConflictPolicy = NameConflictPolicy.fail;
  modTime?: utcDateTime;
}

model FileUpdateRequest {
  @minLength(1)
  @maxLength(255)
  name?: string;

  modTime?: utcDateTime;
}

model FileMoveRequest {
  parentId?: Uuid;
  conflictPolicy?: NameConflictPolicy = NameConflictPolicy.fail;
}

model FileCopyRequest {
  parentId?: Uuid;

  @minLength(1)
  @maxLength(255)
  name?: string;

  conflictPolicy?: NameConflictPolicy = NameConflictPolicy.fail;
}

model OptionalIfMatchHeader {
  @header("If-Match")
  ifMatch?: ETag;
}

model FileMutationResponse {
  @statusCode
  statusCode: 200;

  @header("Etag")
  ETag: ETag;

  @body
  body: FileEntry;
}

model FileCreatedResponse {
  @statusCode
  statusCode: 201;

  @header("Etag")
  ETag: ETag;

  @header("Location")
  location: Uri;

  @body
  body: FileEntry;
}

model FileContentRequestHeaders {
  @header("Range")
  range?: string;

  @header("If-None-Match")
  ifNoneMatch?: ETag;
}

model FileContentResponse {
  @statusCode
  statusCode: 200;

  @header("Content-Length")
  contentLength: int64;

  @header("Accept-Ranges")
  acceptRanges: "bytes";

  @header("Etag")
  ETag: ETag;

  @header("Last-Modified")
  lastModified: utcDateTime;

  @header("Content-Disposition")
  contentDisposition: string;

  @body
  body: bytes;
}

model FilePartialContentResponse {
  @statusCode
  statusCode: 206;

  @header("Content-Length")
  contentLength: int64;

  @header("Content-Range")
  contentRange: string;

  @header("Accept-Ranges")
  acceptRanges: "bytes";

  @header("Etag")
  ETag: ETag;

  @header("Last-Modified")
  lastModified: utcDateTime;

  @header("Content-Disposition")
  contentDisposition: string;

  @body
  body: bytes;
}

model FileHeadResponse {
  @statusCode
  statusCode: 200;

  @header("Content-Length")
  contentLength: int64;

  @header("Accept-Ranges")
  acceptRanges: "bytes";

  @header("Etag")
  ETag: ETag;

  @header("Last-Modified")
  lastModified: utcDateTime;

  @header("Content-Disposition")
  contentDisposition: string;
}

@tag("Files")
@route("/v1/files")
@useAuth(TelDriveAuth)
namespace Files {
  /** List one folder or search within the authenticated user's drive. */
  @get
  @operationId("listFiles")
  op list(...CursorQuery, ...FileListQuery):
    | JsonResponse<200, CursorPage<FileEntry>>
    | ErrorResponse<401>
    | ErrorResponse<404>
    | ErrorResponse<422>;

  /** Return file or folder metadata. */
  @get
  @route("/{fileId}")
  @operationId("getFile")
  op get(@path fileId: Uuid):
    | FileMutationResponse
    | ErrorResponse<401>
    | ErrorResponse<404>;

  /** Rename a file/folder or update its modification time. */
  @patch
  @route("/{fileId}")
  @operationId("updateFile")
  op update(
    ...OptionalIfMatchHeader,
    @path fileId: Uuid,
    @body body: FileUpdateRequest,
  ):
    | FileMutationResponse
    | ErrorResponse<401>
    | ErrorResponse<404>
    | ErrorResponse<409>
    | ErrorResponse<412>
    | ErrorResponse<422>;

  /** Move a file or folder using metadata only. */
  @post
  @route("/{fileId}/move")
  @operationId("moveFile")
  op move(
    ...IdempotencyHeader,
    ...OptionalIfMatchHeader,
    @path fileId: Uuid,
    @body body: FileMoveRequest,
  ):
    | FileMutationResponse
    | ErrorResponse<401>
    | ErrorResponse<404>
    | ErrorResponse<409>
    | ErrorResponse<412>
    | ErrorResponse<422>;

  /** Copy a file or folder to another destination. */
  @post
  @route("/{fileId}/copy")
  @operationId("copyFile")
  op copy(
    ...IdempotencyHeader,
    @path fileId: Uuid,
    @body body: FileCopyRequest,
  ):
    | FileCreatedResponse
    | ErrorResponse<401>
    | ErrorResponse<404>
    | ErrorResponse<409>
    | ErrorResponse<422>;

  /** Move a file or folder to trash. */
  @delete
  @route("/{fileId}")
  @operationId("trashFile")
  op trash(@path fileId: Uuid):
    | EmptyResponse<204>
    | ErrorResponse<401>
    | ErrorResponse<404>
    | ErrorResponse<409>;

  /** Restore a trashed file or folder. */
  @post
  @route("/{fileId}/restore")
  @operationId("restoreFile")
  op restore(...IdempotencyHeader, @path fileId: Uuid):
    | FileMutationResponse
    | ErrorResponse<401>
    | ErrorResponse<404>
    | ErrorResponse<409>;

  /** Permanently delete a trashed file or folder from the catalog. */
  @delete
  @route("/{fileId}/purge")
  @operationId("purgeFile")
  op purge(@path fileId: Uuid):
    | EmptyResponse<204>
    | ErrorResponse<401>
    | ErrorResponse<404>
    | ErrorResponse<409>;


  /** Move every trashed file and folder owned by the authenticated user to deletion pending. */
  @delete
  @route("/trash")
  @operationId("cleanTrash")
  op cleanTrash():
    | EmptyResponse<204>
    | ErrorResponse<401>;

  /** Stream complete or partial file content. */
  @get
  @route("/{fileId}/content")
  @operationId("downloadFile")
  op download(...FileContentRequestHeaders, @path fileId: Uuid):
    | FileContentResponse
    | FilePartialContentResponse
    | EmptyResponse<304>
    | ErrorResponse<401>
    | ErrorResponse<404>
    | ErrorResponse<416>
    | ErrorResponse<429>
    | ErrorResponse<503>;

  /** Return content metadata without streaming the file. */
  @head
  @route("/{fileId}/content")
  @operationId("headFile")
  op head(@path fileId: Uuid):
    | FileHeadResponse
    | ErrorResponse<401>
    | ErrorResponse<404>
    | ErrorResponse<429>
    | ErrorResponse<503>;

  /** Return the authenticated user's synchronized viewer state for a file. */
  @get
  @route("/{fileId}/view-state")
  @operationId("getFileViewState")
  op getViewState(@path fileId: Uuid):
    | JsonResponse<200, FileViewState>
    | EmptyResponse<204>
    | ErrorResponse<401>
    | ErrorResponse<404>;

  /** Create or replace the authenticated user's synchronized viewer state. */
  @put
  @route("/{fileId}/view-state")
  @operationId("putFileViewState")
  op putViewState(@path fileId: Uuid, @body body: FileViewStateUpdate):
    | JsonResponse<200, FileViewState>
    | ErrorResponse<401>
    | ErrorResponse<404>
    | ErrorResponse<422>;

  /** Clear the authenticated user's synchronized viewer state. */
  @delete
  @route("/{fileId}/view-state")
  @operationId("deleteFileViewState")
  op deleteViewState(@path fileId: Uuid):
    | EmptyResponse<204>
    | ErrorResponse<401>;

  /** Transactionally move multiple files or folders. */
  @post
  @route("/bulk/move")
  @operationId("bulkMoveFiles")
  op bulkMove(...IdempotencyHeader, @body body: FileBulkMoveRequest):
    | JsonResponse<200, FileBulkResult>
    | ErrorResponse<401>
    | ErrorResponse<404>
    | ErrorResponse<409>
    | ErrorResponse<422>;

  /** Transactionally move multiple files or folders to trash. */
  @post
  @route("/bulk/trash")
  @operationId("bulkTrashFiles")
  op bulkTrash(...IdempotencyHeader, @body body: FileBulkTrashRequest):
    | JsonResponse<200, FileBulkResult>
    | ErrorResponse<401>
    | ErrorResponse<404>
    | ErrorResponse<409>
    | ErrorResponse<422>;

  /** Aggregate active file counts and bytes by category. */
  @get
  @route("/statistics/categories")
  @operationId("getFileCategoryStatistics")
  op categoryStatistics():
    | JsonResponse<200, FileCategoryStatistics[]>
    | ErrorResponse<401>;

  /** Aggregate drive-level counters for the authenticated user. */
  @get
  @route("/statistics/drive")
  @operationId("getDriveStatistics")
  op driveStatistics(): JsonResponse<200, DriveStatistics> | ErrorResponse<401>;
}

@tag("Folders")
@route("/v1/folders")
@useAuth(TelDriveAuth)
namespace Folders {
  @post
  @operationId("createFolder")
  op create(...IdempotencyHeader, @body body: FolderCreateRequest):
    | FileCreatedResponse
    | ErrorResponse<401>
    | ErrorResponse<404>
    | ErrorResponse<409>
    | ErrorResponse<422>;
}
