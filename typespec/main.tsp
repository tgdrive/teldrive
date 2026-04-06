import "@typespec/http";
import "@typespec/openapi";

using TypeSpec.Http;
@server(
  "{url}/api",
  "Teldrive Server URL",
  {
    url: string,
  }
)
@service(#{ title: "Teldrive" })
namespace Teldrive;

@format("uuid")
scalar UUID extends string;

model AccessTokenCookieAuth {
  type: AuthType.apiKey;
  in: ApiKeyLocation.cookie;
  name: "access_token";
}

model XApiKeyHeaderAuth {
  type: AuthType.apiKey;
  in: ApiKeyLocation.header;
  name: "X-Api-Key";
}

model SessionHashAuth {
  type: AuthType.apiKey;
  in: ApiKeyLocation.query;
  name: "shash";
}

alias ApiAuth = BearerAuth | AccessTokenCookieAuth | XApiKeyHeaderAuth | SessionHashAuth;

@doc("File streaming response")
model FileStream {
  @doc("The file content as binary data")
  @body
  content: bytes;

  @statusCode
  statusCode: 200 | 206;

  @header("Accept-Ranges")
  @doc("Indicates server supports range requests")
  @example("bytes")
  acceptRanges: "bytes";

  @header("Content-Length")
  @doc("Size of the response body in bytes")
  @example("1048576")
  contentLength: string;

  @header("Content-Disposition")
  @doc("File attachment information")
  @example("attachment; filename=\"example.pdf\"")
  contentDisposition: string;

  @header("Content-Range")
  @doc("Range of bytes being sent")
  @example("bytes 0-1048575/2097152")
  contentRange?: string;

  @header("Etag")
  @doc("Entity tag for cache validation")
  @example("5e8d1a2b-3c4d-5e6f-7a8b-9c0d1e2f3a4b")
  eTag: string;

  @header("Last-Modified")
  @doc("Last modification timestamp")
  lastModified: utcDateTime;
}

@error
@doc("Standard error response")
model Error {
  @doc("Error code")
  @example(500)
  code: integer;

  @doc("Error message")
  @example("Internal server error occurred")
  message: string;
}

@doc("User session information containing authentication and profile details")
model Session {
  @doc("User's display name or full name")
  @example("John Doe")
  name: string;

  @doc("Telegram username without @ symbol")
  @example("johndoe")
  userName: string;

  @doc("Telegram user ID (unique identifier)")
  @example(123456789)
  @minValue(1)
  userId: int64;

  @doc("Premium subscription status")
  @example(true)
  isPremium: boolean;

  @doc("Session ID for verification and security")
  @example("123e4567-e89b-12d3-a456-426614174000")
  sessionId: UUID;

  @doc("Compatibility alias for session ID")
  hash?: string;

  @doc("Session expiration date")
  expires: utcDateTime;

  @doc("Optional session token for client identification")
  @visibility(Lifecycle.Create)
  session: string;
}

model AuthAttempt {
  id: string;
  authType: "qr" | "phone";
  state: "created" | "qr_pending" | "code_sent" | "password_required" | "authenticated" | "failed" | "expired";
  token?: string;
  phoneCodeHash?: string;
  session?: AuthAttemptSession;
  message?: string;
}

model AuthAttemptSession {
  name: string;
  userName: string;
  userId: int64;
  isPremium: boolean;
  session: string;
}

model AuthAttemptCreate {
  authType: "qr" | "phone";
  phoneNo?: string;
}

model AuthAttemptSendCode {
  phoneNo: string;
}

model AuthAttemptSignIn {
  phoneNo: string;
  phoneCode: string;
  phoneCodeHash: string;
}

model AuthAttemptPassword {
  password: string;
}

model AuthAttemptEvent {
  type: "token" | "phone_code" | "password_required" | "success" | "error";
  token?: string;
  phoneCodeHash?: string;
  session?: AuthAttemptSession;
  message?: string;
}

@route("/auth")
@tag("Auth")
interface Auth {
  @route("/session")
  @summary("Get session information")
  @get
  @useAuth(ApiAuth)
  session(): (Session & {
    @header("Set-Cookie") setCookie?: string;
  }) | NoContentResponse | Error;

  @route("/login")
  @summary("Login")
  @post
  login(@body body: AuthAttemptSession): {
    ...NoContentResponse;
    @header("Set-Cookie") setCookie: string;
  } | Error;

  @route("/refresh")
  @summary("Refresh access token")
  @post
  refresh(@cookie(#{ name: "refresh_token" }) refreshToken: string): {
    ...NoContentResponse;
    @header("Set-Cookie") setCookie: string;
  } | Error;

  @route("/logout")
  @summary("Logout")
  @useAuth(ApiAuth)
  @post
  logout(): {
    ...NoContentResponse;
    @header("Set-Cookie") setCookie: string;
  } | Error;

  @route("/attempts")
  @summary("Create auth attempt")
  @post
  createAttempt(@body body: AuthAttemptCreate): {
    ...AuthAttempt;
    @statusCode _: 201;
  } | Error;

  @route("/attempts/{id}")
  @summary("Get auth attempt")
  @get
  getAttempt(@path id: string): AuthAttempt | Error;

  @route("/attempts/{id}")
  @summary("Delete auth attempt")
  @delete
  deleteAttempt(@path id: string): NoContentResponse | Error;

  @route("/attempts/{id}/phone/send-code")
  @summary("Send phone login code")
  @post
  sendCode(@path id: string, @body body: AuthAttemptSendCode): NoContentResponse | Error;

  @route("/attempts/{id}/phone/sign-in")
  @summary("Sign in with phone code")
  @post
  signIn(@path id: string, @body body: AuthAttemptSignIn): NoContentResponse | Error;

  @route("/attempts/{id}/password")
  @summary("Complete password auth")
  @post
  password(@path id: string, @body body: AuthAttemptPassword): NoContentResponse | Error;
}

@doc("Supported file categories")
enum Category {
  @doc("Archive files (.zip, .rar, etc)")
  archive,

  @doc("Audio files (.mp3, .wav, etc)")
  audio,

  @doc("Document files (.pdf, .doc, etc)")
  document,

  @doc("Image files (.jpg, .png, etc)")
  image,

  @doc("Other file types")
  other,

  @doc("Video files (.mp4, .mov, etc)")
  video,
}

@doc("Hash algorithm types for integrity checking")
enum HashAlgorithm {
  @doc("MD5 hash algorithm")
  md5,

  @doc("SHA-256 hash algorithm")
  sha256,

  @doc("XXHash64 algorithm")
  xxhash,

  @doc("BLAKE3 hash algorithm")
  blake3,
}

@doc("File query parameters")
model FileQuery {
  @query
  @doc("File name filter")
  @example("video.mp4")
  name?: string;

  @query
  @doc("Search query")
  @example("report 2023")
  query?: string;

  @query
  @doc("Search type")
  @example("text")
  searchType?: "text" | "regex" = "text";

  @query
  @doc("File type")
  @example("file")
  type?: "folder" | "file";

  @query
  @doc("File path")
  @example("/documents/2023/")
  path?: string;

  @query
  @doc("Operation")
  @example("list")
  operation?: "list" | "find" = "list";

  @query
  @doc("File Status")
  @example("active")
  status?: "active" = "active";

  @query
  @doc("Enable deep search")
  @example(false)
  deepSearch?: boolean = false;

  @query
  @doc("Show shared files")
  @example(false)
  shared?: boolean;

  @query
  @doc("Parent folder ID")
  @example("123e4567-e89b-12d3-a456-426614174000")
  parentId?: UUID;

  @query
  @doc("File category")
  @example(#[Category.document, Category.video])
  category?: Array<Category>;

  @query
  @doc("UpdatedAt Filter supports operator eq, gt, lt, gte, lte")
  updatedAt?: string;

  @query
  @doc("Sort field")
  @example("name")
  sort?: "name" | "updatedAt" | "size" | "id" = "name";

  @query
  @doc("Sort order")
  @example("asc")
  order?: "asc" | "desc" = "asc";

  @query
  @doc("Items per page")
  @example(50)
  @maxValue(1000)
  @minValue(1)
  limit?: integer = 500;

  @query
  @doc("Pagination cursor")
  cursor?: string;

  @query
  @doc("Page number")
  @example(1)
  @minValue(1)
  page?: integer = 1;
}

@doc("File part information")
model Part {
  @doc("Part ID")
  @example(1)
  id: integer;

  @doc("Encryption salt")
  @example("abc123")
  salt?: string;
}
@doc("File metadata")
model File {
  @doc("File ID")
  @example("123e4567-e89b-12d3-a456-426614174000")
  @visibility(Lifecycle.Read)
  id?: UUID;

  @doc("Upload session ID (for multi-part uploads)")
  @example("upload-abc-123")
  uploadId?: string;

  @doc("File name")
  @example("document.pdf")
  name: string;

  @doc("File type")
  @example("file")
  type: "folder" | "file";

  @doc("File parts")
  parts?: Part[];

  @doc("MIME type")
  @example("application/pdf")
  mimeType?: string;

  @doc("File category")
  @example(Category.document)
  @visibility(Lifecycle.Read)
  category?: Category;

  @doc("Channel ID")
  @example(123456789)
  channelId?: int64;

  @doc("File or Folder path")
  @example("/documents/2023/")
  path?: string;

  @doc("Parent folder ID")
  @example("123e4567-e89b-12d3-a456-426614174000")
  parentId?: UUID;

  @doc("File size in bytes")
  @example(1048576)
  size?: int64;

  @doc("Encryption status")
  @example(false)
  encrypted?: boolean;

  @doc("BLAKE3 tree hash for integrity checking")
  @example("d41d8cd98f00b204e9800998ecf8427e")
  hash?: string;

  @doc("Last update time")
  @visibility(Lifecycle.Read)
  updatedAt?: utcDateTime;

}

@doc("Pagination metadata for cursor-based listing")
model Meta {
  @doc("Next cursor for pagination")
  nextCursor?: string;
}

@doc("File share creation request")
model FileShareCreate {
  @doc("Share password")
  @example("securepass123")
  password?: string;

  @doc("Share expiration date")
  expiresAt?: utcDateTime;
}

@doc("Lightweight file sharing information")
model FileShare {
  @doc("Share ID")
  @example("123e4567-e89b-12d3-a456-426614174000")
  id: UUID;

  @doc("Indicates if the shared file requires password protection")
  @example(true)
  protected: boolean;

  @doc("Expiration date and time of the share link")
  expiresAt?: utcDateTime;
}

@doc("Detailed file sharing information")
model FileShareDetail {
  @doc("Share ID")
  @example("123e4567-e89b-12d3-a456-426614174000")
  id: UUID;

  @doc("File ID")
  @example("123e4567-e89b-12d3-a456-426614174000")
  fileId: UUID;

  @doc("ID of the user who created the share")
  @example(123456789)
  userId: int64;

  @doc("File type")
  @example("file")
  type: "folder" | "file";

  @doc("Name of the shared file or folder")
  name: string;

  @doc("Indicates if the shared file requires password protection")
  @example(true)
  protected: boolean;

  @doc("Expiration date and time of the share link")
  expiresAt?: utcDateTime;
}

@doc("Paginated file listing response with metadata")
model FileList {
  @doc("Array of file entries in the current page")
  @minItems(0)
  @maxItems(1000)
  items: File[];

  @doc("Pagination metadata")
  meta: Meta;
}

@doc("File update request")
model FileUpdate {
  @doc("File name")
  @example("document.pdf")
  name?: string;

  @doc("Parent folder ID")
  @example("123e4567-e89b-12d3-a456-426614174000")
  parentId?: UUID;

  @doc("Channel ID")
  @example(123456)
  channelId?: int64;

  @doc("Upload ID for hash calculation")
  uploadId?: string;

  @doc("File parts")
  parts?: Part[];

  @doc("Indicates if the file is encrypted")
  encrypted?: boolean;

  @doc("File size in bytes")
  @example(1048576)
  size?: int64;

  @doc("Last update time")
  updatedAt?: utcDateTime;

}

@doc("River job state")
enum JobState {
  available,
  running,
  scheduled,
  retryable,
  completed,
  cancelled,
  discarded,
}

@doc("Job error details")
model JobError {
  @doc("Error message")
  message: string;

  @doc("Error context or stack trace")
  detail?: string;
}

@doc("Job progress summary")
model JobProgress {
  @doc("Total work items")
  total?: integer;

  @doc("Completed work items")
  done?: integer;

  @doc("Progress percentage")
  percent?: integer;
}

@doc("Job output payload")
model JobOutput {
  @doc("Progress details")
  progress?: JobProgress;

  @doc("Additional output payload")
  data?: unknown;
}

@doc("Job insertion request")
model JobInsertRequest {
  @doc("Job kind")
  kind: string;

  @doc("Job arguments")
  args: unknown;

  @doc("Queue name")
  queue?: string;

  @doc("Schedule time for delayed execution")
  scheduledAt?: utcDateTime;

  @doc("Maximum attempts before discard")
  maxAttempts?: integer;
}

@doc("Job status response")
model JobStatus {
  @doc("Job ID")
  id: int64;

  @doc("Job kind")
  kind: string;

  @doc("Queue name")
  queue: string;

  @doc("Current job state")
  state: JobState;

  @doc("Number of attempts so far")
  attempt: integer;

  @doc("Maximum allowed attempts")
  maxAttempts: integer;

  @doc("Job creation time")
  createdAt: utcDateTime;

  @doc("Scheduled execution time")
  scheduledAt?: utcDateTime;

  @doc("Start time of the current attempt")
  startedAt?: utcDateTime;

  @doc("Completion time")
  finalizedAt?: utcDateTime;

  @doc("Job output payload")
  output?: JobOutput;

  @doc("Error history")
  errors?: JobError[];
}

@doc("Query parameters for listing jobs")
model JobListQuery {
  @query
  @doc("Filter by job state")
  state?: JobState;

  @query
  @doc("Maximum number of jobs to return")
  @minValue(1)
  @maxValue(200)
  limit?: integer = 50;

  @query
  @doc("Pagination cursor")
  cursor?: string;
}

@doc("Job counts grouped by state")
model JobStateCounts {
  @doc("Total number of jobs")
  all: int64;

  @doc("Available jobs")
  available: int64;

  @doc("Running jobs")
  running: int64;

  @doc("Scheduled jobs")
  scheduled: int64;

  @doc("Retryable jobs")
  retryable: int64;

  @doc("Completed jobs")
  completed: int64;

  @doc("Cancelled jobs")
  cancelled: int64;

  @doc("Discarded jobs")
  discarded: int64;
}

@doc("Paginated job listing response")
model JobList {
  @doc("Array of jobs")
  @minItems(0)
  @maxItems(200)
  items: JobStatus[];

  @doc("Pagination metadata")
  meta: Meta;

  @doc("Job counts grouped by state")
  counts: JobStateCounts;
}

model SyncFilters {
  include?: string[];
  exclude?: string[];
  minSize?: int64;
  maxSize?: int64;
  excludeIfPresent?: string[];
}

model SyncOptions {
  partSize?: int64;
  encrypted?: boolean;
  sync?: boolean;
}

model SyncArgs {
  source: string;
  destinationDir: string;
  headers?: Record<string>;
  proxy?: string;
  filters?: SyncFilters;
  options?: SyncOptions;
}

model SyncArgsUpdate {
  source?: string;
  destinationDir?: string;
  headers?: Record<string>;
  proxy?: string;
  filters?: SyncFilters;
  options?: SyncOptions;
}

enum PeriodicJobKind {
  SyncRun: "sync.run",
  CleanOldEvents: "clean.old_events",
  CleanStaleUploads: "clean.stale_uploads",
  CleanPendingFiles: "clean.pending_files",
  RefreshFolderSizes: "refresh.folder_sizes",
}

model CleanOldEventsArgs {
  retention?: string;
}

model CleanStaleUploadsArgs {
  retention?: string;
}

model CleanPendingFilesArgs {}

model PeriodicJobSummary {
  id: UUID;
  name: string;
  kind: PeriodicJobKind;
  enabled: boolean;
  cronExpression: string;
  createdAt: utcDateTime;
  updatedAt: utcDateTime;
}

model PeriodicJobDetail {
  id: UUID;
  name: string;
  kind: PeriodicJobKind;
  enabled: boolean;
  cronExpression: string;
  createdAt: utcDateTime;
  updatedAt: utcDateTime;
  args?: Record<unknown>;
}

model PeriodicJobCreate {
  name: string;
  enabled?: boolean = true;
  cronExpression: string;
  args: SyncArgs;
}

model PeriodicJobUpdate {
  name?: string;
  enabled?: boolean;
  cronExpression?: string;
  args?: Record<unknown>;
}

@route("/periodic-jobs")
@tag("PeriodicJobs")
@useAuth(ApiAuth)
interface PeriodicJobs {
  @route("")
  @get
  @summary("List periodic jobs")
  list(): PeriodicJobSummary[] | Error;

  @route("")
  @post
  @summary("Create periodic sync job")
  create(@body body: PeriodicJobCreate): (PeriodicJobDetail & {
    @statusCode _: 201;
  }) | Error;

  @route("/{id}")
  @get
  @summary("Get periodic job")
  get(@path id: UUID): PeriodicJobDetail | Error;

  @route("/{id}")
  @patch(#{ implicitOptionality: true })
  @summary("Update periodic job")
  update(@path id: UUID, @body body: PeriodicJobUpdate): PeriodicJobDetail | Error;

  @route("/{id}")
  @delete
  @summary("Delete periodic job")
  delete(@path id: UUID): NoContentResponse | Error;

  @route("/{id}/enable")
  @post
  @summary("Enable periodic job")
  enable(@path id: UUID): NoContentResponse | Error;

  @route("/{id}/disable")
  @post
  @summary("Disable periodic job")
  disable(@path id: UUID): NoContentResponse | Error;

  @route("/{id}/run")
  @post
  @summary("Run periodic job now")
  run(@path id: UUID): {
    ...JobStatus;
    @statusCode _: 202;
  } | Error;

  @route("/test-connection")
  @post
  @summary("Test sync source connection")
  testConnection(@body body: SyncArgs): NoContentResponse | Error;
}

@doc("Bulk file delete request")
model FileDelete {
  @doc("List of file IDs")
  ids: UUID[];
}

@doc("Bulk file move request")
model FileMove {
  @doc("File IDs to move")
  ids: UUID[];

  @doc("Destination parent path or folder ID")
  destinationParent: string;

  @doc("Optional destination name for single-file move")
  destinationName?: string;
}

@doc("Copy file request")
model FileCopy {
  @doc("Destination parent path or folder ID")
  destination: string;

  @doc("Optional new file name")
  newName?: string;

  @doc("Last update time")
  updatedAt?: utcDateTime;
}

@doc("Statistics for files by category")
@example(#{
  totalFiles: 1250,
  totalSize: 104857600,
  category: Category.document,
})
model CategoryStats {
  @doc("Total number of files")
  totalFiles: int64;

  @doc("Total size of files in bytes")
  totalSize: int64;

  @doc("Category name")
  category: Category;
}

@route("/files")
@tag("Files")
@useAuth(ApiAuth)
interface Files {
  @route("")
  @get
  @summary("List all files")
  list(...FileQuery): FileList | Error;

  @route("")
  @post
  @summary("Create a new file")
  create(...File): {
    ...File;
    @statusCode _: 201;
  } | Error;

  @route("/{id}")
  @get
  @summary("Get file by ID")
  getById(@path id: UUID): File | Error;

  @route("/{id}")
  @patch(#{ implicitOptionality: true })
  @summary("Update file")
  update(@path id: UUID, @body body: FileUpdate): File | Error;

  @route("/{id}")
  @delete
  @summary("Mark file or folder for deletion")
  deleteById(@path id: UUID): NoContentResponse | Error;

  @route("/{id}/copy")
  @post
  @summary("Copy file")
  copy(@path id: UUID, @body body: FileCopy): File | Error;

  @route("/delete")
  @post
  @summary("Mark files or folders for deletion")
  delete(@body body: FileDelete): NoContentResponse | Error;

  @route("/move")
  @post
  @summary("Bulk move files or folders")
  move(@body body: FileMove): NoContentResponse | Error;

  @route("/{id}/shares")
  @get
  @summary("List shares by file ID")
  listShares(@path id: UUID): FileShare[] | Error;

  @route("/{id}/shares/{shareId}")
  @patch(#{ implicitOptionality: true })
  @summary("Update share")
  editShare(
    @path id: UUID,
    @path shareId: UUID,
    @bodyRoot body: FileShareCreate,
  ): NoContentResponse | Error;

  @route("/{id}/shares")
  @post
  @summary("Create share for file")
  createShare(
    @path id: UUID,
    @body body: FileShareCreate,
  ): CreatedResponse | Error;

  @route("/{id}/shares/{shareId}")
  @delete
  @summary("Delete share")
  deleteShare(
    @path id: UUID,
    @path shareId: UUID,
  ): NoContentResponse | Error;

  @route("/{id}/content")
  @get
  @summary("Stream or Download file")
  stream(
    @path id: UUID,
    @query download?: "0" | "1" = "0",
    @header("Range") range?: string,
  ): FileStream | Error;

  @route("/{id}/content")
  @head
  @summary("Get file content headers")
  streamHead(@path id: UUID): FileStream | Error;

  @route("/{id}/children")
  @get
  @summary("List folder children")
  children(@path id: UUID, ...FileQuery): FileList | Error;

  @route("/categories")
  @get
  @summary("Get category stats")
  categoryStats(): CategoryStats[] | Error;
}

@route("/jobs")
@tag("Jobs")
@useAuth(ApiAuth)
interface Jobs {
  @route("")
  @get
  @summary("List jobs")
  list(...JobListQuery): JobList | Error;

  @route("")
  @post
  @summary("Insert a new background job")
  insert(@body body: JobInsertRequest): {
    ...JobStatus;
    @statusCode _: 202;
  } | Error;

  @route("/{id}")
  @get
  @summary("Get job status")
  get(@path id: int64): JobStatus | Error;

  @route("/{id}/cancel")
  @post
  @summary("Cancel a running or queued job")
  cancel(@path id: int64): NoContentResponse | Error;

  @route("/{id}/delete")
  @delete
  @summary("Delete a completed, cancelled, or discarded job")
  delete(@path id: int64): NoContentResponse | Error;
}

@doc("Query parameters for file upload")
@example(#{
  fileName: "document.pdf",
  partNo: 1,
  channelId: 123456789,
  encrypted: false,
})
model UploadQuery {
  @doc("Original file name")
  @query
  fileName: string;

  @doc("Part number in sequence")
  @query
  partNo: integer;

  @doc("Optional channel identifier for upload")
  @query
  channelId?: int64;

  @doc("Whether the upload content is encrypted")
  @query
  encrypted?: boolean = false;

  @doc("Enable BLAKE3 hashing for integrity checking")
  @query
  hashing?: boolean = true;
}

@doc("Details of an uploaded part")
model UploadPart {
  @doc("Name identifier of the part")
  name: string;

  @doc("Telegram Message ID")
  partId: integer;

  @doc("Sequential number of the part")
  partNo: integer;

  @doc("Channel identifier associated with the upload")
  channelId: int64;

  @doc("Size of the part in bytes")
  size: int64;

  @doc("Indicates if the part is encrypted")
  encrypted: boolean;

  @doc("Salt value used for encryption, required if encrypted is true")
  salt?: string;
}

@doc("Statistics about the upload")
model UploadStats {
  @doc("Date and time when the upload occurred")
  uploadDate: utcDateTime;

  @doc("Total number of bytes uploaded")
  totalUploaded: int64;
}

@route("/uploads")
@tag("Uploads")
@useAuth(ApiAuth)
interface Uploads {
  @route("/{id}")
  @post
  @summary("Upload file")
  upload(
    @path id: string,
    @header("Content-Type") contentType?: string,
    @header("Content-Length") contentLength: int64,
    @bodyRoot body: bytes,
    ...UploadQuery,
  ): UploadPart | Error;

  @route("/{id}")
  @get
  @summary("Get uploaded parts by ID")
  partsById(@path id: string): UploadPart[] | Error;

  @route("/{id}")
  @delete
  @summary("Delete uploaded file")
  delete(@path id: string): NoContentResponse | Error;

  @route("/stats")
  @get
  @summary("Get upload stats")
  stats(@query days: integer): UploadStats[] | Error;
}

@doc("User session information")
@example(#{
  sessionId: "123e4567-e89b-12d3-a456-426614174000",
  createdAt: "2023-10-21T07:28:00Z",
  location: "New York, USA",
  officialApp: true,
  appName: "Telegram",
  valid: true,
  current: true,
})
model UserSession {
  @doc("Unique identifier for the session")
  sessionId: UUID;

  @doc("Compatibility alias for session ID")
  hash?: string;

  @doc("Session creation timestamp")
  @format("date-time")
  createdAt: string;

  @doc("Geographic location where session was created")
  location?: string;

  @doc("Indicates if session is from an official Telegram app")
  officialApp?: boolean;

  @doc("Name of the application used for session")
  appName?: string;

  @doc("Indicates if the session is still valid")
  valid: boolean;

  @doc("Indicates if this is the currently active session")
  current: boolean;
}

@doc("Telegram channel information")
@example(#{ channelName: "Channel Name", channelId: 123456789 })
model Channel {
  @doc("Name of the Telegram channel")
  channelName: string;

  @doc("Unique numeric identifier of the channel")
  channelId?: int64;
}

model AddBots {
  @doc("List of bot tokens")
  bots: string[];
}

@doc("Create API key request")
model UserApiKeyCreate {
  @doc("Display name for the API key")
  @example("CI deploy key")
  name: string;

  @doc("Optional expiration date. Omit for no expiry")
  expiresAt?: utcDateTime;
}

@doc("API key metadata")
model UserApiKey {
  @doc("API key ID")
  @example("123e4567-e89b-12d3-a456-426614174000")
  id: UUID;

  @doc("Display name for the API key")
  @example("CI deploy key")
  name: string;

  @doc("Creation date and time")
  createdAt: utcDateTime;

  @doc("Optional expiration date and time")
  expiresAt?: utcDateTime;

  @doc("Last used date and time")
  lastUsedAt?: utcDateTime;
}

@doc("API key creation response")
model UserApiKeyCreateResult {
  ...UserApiKey;

  @doc("Plain API key value (returned only once)")
  @example("tdk_abc123")
  key: string;
}

@doc("User configuration for channel and bot settings")
@example(#{ channelId: 123456789, bots: #["bot1", "bot2"] })
model UserConfig {
  @doc("Channel identifier associated with the user")
  channelId: int64;

  @doc("List of bot tokens")
  bots: string[];
}

@route("/users")
@tag("Users")
@useAuth(ApiAuth)
interface Users {
  @route("/profile")
  @get
  @summary("Get user profile photo")
  profileImage(): {
    @body image: bytes;
    @statusCode statusCode: 200;
    @header("Content-Type") contentType: "image/jpeg";
    @header("Etag") eTag: string;
    @header("Cache-Control") cacheControl: string;
    @header("Content-Length") contentLength: int64;
    @header("Content-Disposition") contentDisposition: string;
  } | Error;

  @route("/config")
  @get
  @summary("Get user config")
  stats(): UserConfig | Error;

  @route("/channels")
  @get
  @summary("List user channels")
  listChannels(): Channel[] | Error;

  @route("/channels")
  @post
  @summary("Create user channel")
  createChannel(@body body: Channel): CreatedResponse | Error;

  @route("/channels")
  @patch(#{ implicitOptionality: true })
  @summary("Update user channel")
  updateChannel(@body body: Channel): NoContentResponse | Error;

  @route("/channels/{id}")
  @delete
  @summary("Delete user channel")
  deleteChannel(@path id: string): NoContentResponse | Error;

  @route("/channels/sync")
  @patch(#{ implicitOptionality: true })
  @summary("Sync user channels")
  syncChannels(): NoContentResponse | Error;

  @route("/sessions")
  @get
  @summary("List user sessions")
  listSessions(): UserSession[] | Error;

  @route("/bots")
  @post
  @summary("Add bots to user account")
  addBots(@body body: AddBots): NoContentResponse | Error;

  @route("/bots")
  @delete
  @summary("Remove bots from user account")
  removeBots(): NoContentResponse | Error;

  @route("/api-keys")
  @get
  @summary("List API keys")
  listApiKeys(): UserApiKey[] | Error;

  @route("/api-keys")
  @post
  @summary("Create API key")
  createApiKey(@body body: UserApiKeyCreate): {
    ...UserApiKeyCreateResult;
    @statusCode _: 201;
  } | Error;

  @route("/api-keys/{id}")
  @delete
  @summary("Revoke API key")
  removeApiKey(@path id: UUID): NoContentResponse | Error;

  @route("/sessions/{id}")
  @delete
  @summary("Remove user session")
  removeSession(@path id: UUID): NoContentResponse | Error;
}

model FileShareInfo {
  @doc("File name")
  @example("document.pdf")
  name: string;

  @doc("File type")
  @example("file")
  type: "folder" | "file";

  @doc("Share expiration date")
  expiresAt?: utcDateTime;

  @doc("Share User ID")
  userId: int64;

  @doc("Share Protection Status")
  @example(false)
  protected: boolean;
}
model ShareUnlock {
  @doc("Share password")
  @example("securepass123")
  password: string;
}

@doc("Share query parameters")
model ShareQuery {
  @query
  @doc("Folder path")
  @example("/documents/2023/")
  path?: string;

  @query
  @doc("Sort field")
  @example("name")
  sort?: "name" | "updatedAt" | "size" | "id" = "name";

  @query
  @doc("Sort order")
  @example("asc")
  order?: "asc" | "desc" = "asc";

  @query
  @doc("Items per page")
  @example(50)
  @maxValue(1000)
  @minValue(1)
  limit?: integer = 500;

  @query
  @doc("Pagination cursor")
  cursor?: string;

  @query
  @doc("Page number")
  @example(1)
  @minValue(1)
  page?: integer = 1;
}

@route("/shares")
@tag("Shares")
interface Shares {
  @route("/{id}")
  @get
  @summary("Get share by ID")
  getById(@path id: UUID): FileShareInfo | Error;

  @route("/{id}/unlock")
  @post
  @summary("Unlock share")
  unlock(@path id: UUID, @body body: ShareUnlock): {
    ...NoContentResponse;
    @header("Set-Cookie") setCookie: string;
  } | Error;

  @route("/{id}/files")
  @get
  @summary("List files in share")
  listFiles(@path id: UUID, @cookie(#{ name: "share_token" }) shareToken?: string, ...ShareQuery): FileList | Error;

  @route("/{id}/files/{fileId}/content")
  @get
  @summary("Stream or Download shared file")
  stream(
    @path id: UUID,
    @path fileId: UUID,
    @cookie(#{ name: "share_token" }) shareToken?: string,
    @query download?: "0" | "1" = "0",
  ): FileStream | Error;
}

model Source {
  @doc("File ID")
  @example("123e4567-e89b-12d3-a456-426614174000")
  id: UUID;

  @doc("File name")
  @example("document.pdf")
  name: string;

  @doc("File type")
  @example("file")
  type: "folder" | "file";

  @doc("Parent ID")
  @example("123e4567-e89b-12d3-a456-426614174000")
  parentId: UUID;

  @doc("Destination Parent ID")
  @example("123e4567-e89b-12d3-a456-426614174000")
  destParentId?: UUID;

  @doc("Full path of the file/folder (e.g., 'documents/projects/file.txt')")
  @example("documents/2023/report.pdf")
  path?: string;
}

@doc("Event information")
enum EventType {
  "files.created",
  "files.updated",
  "files.deleted",
  "files.moved",
  "files.copied",
  "uploads.progress",
  "jobs.progress",
}

@doc("Event information")
model Event {
  @doc("Event ID")
  @example("123e4567-e89b-12d3-a456-426614174000")
  id: UUID;

  @doc("Event type")
  type: EventType;

  @doc("Event timestamp")
  createdAt: utcDateTime;

  @doc("Event Source")
  source: Source;
}

@route("/events")
@tag("Events")
interface Events {
  @route("")
  @get
  @summary("Get events")
  @useAuth(ApiAuth)
  @doc("Get events (polling)")
  getEvents(): Array<Event> | Error;

  @route("/stream")
  @get
  @summary("Server-Sent Events stream")
  @useAuth(ApiAuth)
  @doc("Real-time event stream using Server-Sent Events (SSE). Events are filtered by authenticated user. Optional interval parameter for heartbeat configuration.")
  eventsStream(
    @doc("Event types filter. Supports repeated query params and comma-separated values. Wildcards supported: files.*, uploads.*, jobs.*")
    @query
    types?: string[],

    @doc("Heartbeat interval in milliseconds (default: 30000)")
    @query
    interval?: int64,
  ): {
    @statusCode statusCode: 200;
    @header("Content-Type") contentType: "text/event-stream";
    @header("Cache-Control") cacheControl: "no-cache";
    @header("Connection") connection: "keep-alive";
    @body body: string;
  } | Error;
}

model ApiVersion {
  @doc("API version")
  @example("1.0.0")
  version: string;

  @doc("Git commit SHA")
  @example("a1b2c3d4e5f6g7h8i9j0")
  commitSHA: string;

  @doc("Go version")
  @example("go1.17.1")
  goVersion: string;

  @doc("Operating system")
  @example("linux")
  os: string;

  @doc("Architecture")
  @example("amd64")
  arch: string;
}

@route("/version")
@tag("Version")
@useAuth(ApiAuth)
interface Version {
  @route("")
  @get
  @summary("Get API version")
  version(): ApiVersion | Error;
}
