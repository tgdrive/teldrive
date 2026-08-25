using TypeSpec.Http;
using TypeSpec.OpenAPI;

namespace TelDrive;

enum SharePermission {
  read: "read",
  edit: "edit",
}

model ShareCreateRequest {
  /** Optional password. The server stores only a password hash. */
  @minLength(1)
  password?: string;

  expiresAt?: utcDateTime;

  @minValue(1)
  maxDownloads?: int64;

  permission?: SharePermission = SharePermission.read;
}

model ShareUpdateRequest {
  @minLength(1)
  password?: string;

  clearPassword?: boolean = false;
  expiresAt?: utcDateTime;
  clearExpiresAt?: boolean = false;

  @minValue(1)
  maxDownloads?: int64;

  clearMaxDownloads?: boolean = false;
  permission?: SharePermission;
}

model PublicShareListQuery {
  /** Folder path relative to the shared folder root. */
  @query
  @maxLength(4096)
  path?: string;

  @query
  @maxLength(512)
  search?: string;
}

model ShareCreated {
  id: Uuid;
  fileId: Uuid;

  /** Opaque public token returned only when the share is created. */
  token: string;

  publicUrl: Uri;
  passwordProtected: boolean;
  expiresAt?: utcDateTime;
  maxDownloads?: int64;
  permission: SharePermission;
  createdAt: utcDateTime;
}

model ShareSummary {
  id: Uuid;
  fileId: Uuid;
  passwordProtected: boolean;
  expiresAt?: utcDateTime;
  maxDownloads?: int64;
  downloadCount: int64;
  permission: SharePermission;
  createdAt: utcDateTime;
  revokedAt?: utcDateTime;
}

model PublicShare {
  id: Uuid;
  file: FileEntry;
  passwordProtected: boolean;
  expiresAt?: utcDateTime;
  permission: SharePermission;
}

model FileAccessGrantCreateRequest {
  granteeUserId: int64;
  permission?: SharePermission = SharePermission.read;
  expiresAt?: utcDateTime;
}

model FileAccessGrantUpdateRequest {
  permission?: SharePermission;
  expiresAt?: utcDateTime;
  clearExpiresAt?: boolean = false;
}

model FileAccessGrantSummary {
  id: Uuid;
  fileId: Uuid;
  ownerId: int64;
  granteeUserId: int64;
  granteeDisplayName?: string;
  granteeUsername?: string;
  permission: SharePermission;
  expiresAt?: utcDateTime;
  createdAt: utcDateTime;
}

model SharedWithMeEntry {
  grantId: Uuid;
  ownerId: int64;
  ownerDisplayName?: string;
  ownerUsername?: string;
  permission: SharePermission;
  expiresAt?: utcDateTime;
  file: FileEntry;
}

model SharePasswordHeader {
  @header("X-Share-Password")
  password?: string;
}

@tag("Shares")
@route("/v1")
@useAuth(TelDriveAuth)
namespace Shares {
  @post
  @route("/files/{fileId}/shares")
  @operationId("createShare")
  op create(
    ...IdempotencyHeader,
    @path fileId: Uuid,
    @body body: ShareCreateRequest,
  ):
    | JsonResponse<201, ShareCreated>
    | ErrorResponse<401>
    | ErrorResponse<404>
    | ErrorResponse<409>
    | ErrorResponse<422>;

  @get
  @route("/files/{fileId}/shares")
  @operationId("listFileShares")
  op listForFile(...CursorQuery, @path fileId: Uuid):
    | JsonResponse<200, CursorPage<ShareSummary>>
    | ErrorResponse<401>
    | ErrorResponse<404>;

  @patch
  @route("/shares/{shareId}")
  @operationId("updateShare")
  op update(@path shareId: Uuid, @body body: ShareUpdateRequest):
    | JsonResponse<200, ShareSummary>
    | ErrorResponse<401>
    | ErrorResponse<404>
    | ErrorResponse<422>;

  @delete
  @route("/shares/{shareId}")
  @operationId("revokeShare")
  op revoke(@path shareId: Uuid):
    | EmptyResponse<204>
    | ErrorResponse<401>
    | ErrorResponse<404>;

  @post
  @route("/files/{fileId}/grants")
  @operationId("createFileAccessGrant")
  op createGrant(@path fileId: Uuid, @body body: FileAccessGrantCreateRequest):
    | JsonResponse<201, FileAccessGrantSummary>
    | ErrorResponse<401>
    | ErrorResponse<404>
    | ErrorResponse<409>
    | ErrorResponse<422>;

  @get
  @route("/files/{fileId}/grants")
  @operationId("listFileAccessGrants")
  op listGrants(@path fileId: Uuid):
    | JsonResponse<200, FileAccessGrantSummary[]>
    | ErrorResponse<401>
    | ErrorResponse<404>;

  @patch
  @route("/grants/{grantId}")
  @operationId("updateFileAccessGrant")
  op updateGrant(
    @path grantId: Uuid,
    @body body: FileAccessGrantUpdateRequest,
  ):
    | JsonResponse<200, FileAccessGrantSummary>
    | ErrorResponse<401>
    | ErrorResponse<404>
    | ErrorResponse<422>;

  @delete
  @route("/grants/{grantId}")
  @operationId("revokeFileAccessGrant")
  op revokeGrant(@path grantId: Uuid):
    | EmptyResponse<204>
    | ErrorResponse<401>
    | ErrorResponse<404>;

  @get
  @route("/shared-with-me")
  @operationId("listSharedWithMe")
  op sharedWithMe():
    | JsonResponse<200, SharedWithMeEntry[]>
    | ErrorResponse<401>;
}

@tag("Public shares")
@route("/v1/public/shares")
@useAuth(NoAuth)
namespace PublicShares {
  @get
  @route("/{token}")
  @operationId("getPublicShare")
  op get(...SharePasswordHeader, @path token: string):
    | JsonResponse<200, PublicShare>
    | ErrorResponse<401>
    | ErrorResponse<404>
    | ErrorResponse<410>
    | ErrorResponse<429>;

  /** List children within a publicly shared folder. */
  @get
  @route("/{token}/files")
  @operationId("listPublicShareFiles")
  op listFiles(
    ...SharePasswordHeader,
    ...CursorQuery,
    ...PublicShareListQuery,
    @path token: string,
  ):
    | JsonResponse<200, CursorPage<FileEntry>>
    | ErrorResponse<401>
    | ErrorResponse<404>
    | ErrorResponse<410>
    | ErrorResponse<422>
    | ErrorResponse<429>;

  @get
  @route("/{token}/files/{fileId}/content")
  @operationId("downloadPublicShareFile")
  op downloadFile(
    ...SharePasswordHeader,
    ...FileContentRequestHeaders,
    @path token: string,
    @path fileId: Uuid,
  ):
    | FileContentResponse
    | FilePartialContentResponse
    | EmptyResponse<304>
    | ErrorResponse<401>
    | ErrorResponse<404>
    | ErrorResponse<410>
    | ErrorResponse<416>
    | ErrorResponse<429>
    | ErrorResponse<503>;

  @head
  @route("/{token}/files/{fileId}/content")
  @operationId("headPublicShareFile")
  op headFile(...SharePasswordHeader, @path token: string, @path fileId: Uuid):
    | FileHeadResponse
    | ErrorResponse<401>
    | ErrorResponse<404>
    | ErrorResponse<410>
    | ErrorResponse<429>
    | ErrorResponse<503>;

  @get
  @route("/{token}/content")
  @operationId("downloadPublicShare")
  op download(
    ...SharePasswordHeader,
    ...FileContentRequestHeaders,
    @path token: string,
  ):
    | FileContentResponse
    | FilePartialContentResponse
    | EmptyResponse<304>
    | ErrorResponse<401>
    | ErrorResponse<404>
    | ErrorResponse<410>
    | ErrorResponse<416>
    | ErrorResponse<429>
    | ErrorResponse<503>;

  @head
  @route("/{token}/content")
  @operationId("headPublicShare")
  op head(...SharePasswordHeader, @path token: string):
    | FileHeadResponse
    | ErrorResponse<401>
    | ErrorResponse<404>
    | ErrorResponse<410>
    | ErrorResponse<429>
    | ErrorResponse<503>;

  @post
  @route("/{token}/folders")
  @operationId("createPublicShareFolder")
  op createFolder(
    ...SharePasswordHeader,
    @path token: string,
    @body body: FolderCreateRequest,
  ):
    | JsonResponse<201, FileEntry>
    | ErrorResponse<401>
    | ErrorResponse<403>
    | ErrorResponse<404>
    | ErrorResponse<410>
    | ErrorResponse<422>;

  @patch
  @route("/{token}/files/{fileId}")
  @operationId("updatePublicShareFile")
  op updateFile(
    ...SharePasswordHeader,
    @path token: string,
    @path fileId: Uuid,
    @header("If-Match") ifMatch: ETag,
    @body body: FileUpdateRequest,
  ):
    | JsonResponse<200, FileEntry>
    | ErrorResponse<401>
    | ErrorResponse<403>
    | ErrorResponse<404>
    | ErrorResponse<410>
    | ErrorResponse<412>
    | ErrorResponse<422>;

  @delete
  @route("/{token}/files/{fileId}")
  @operationId("trashPublicShareFile")
  op trashFile(
    ...SharePasswordHeader,
    @path token: string,
    @path fileId: Uuid,
  ):
    | EmptyResponse<204>
    | ErrorResponse<401>
    | ErrorResponse<403>
    | ErrorResponse<404>
    | ErrorResponse<410>;

  @post
  @route("/{token}/uploads")
  @operationId("createPublicShareUpload")
  op createUpload(
    ...SharePasswordHeader,
    ...IdempotencyHeader,
    @path token: string,
    @body body: UploadCreateRequest,
  ):
    | JsonResponse<201, UploadSession>
    | ErrorResponse<401>
    | ErrorResponse<403>
    | ErrorResponse<404>
    | ErrorResponse<409>
    | ErrorResponse<410>
    | ErrorResponse<422>;

  @put
  @route("/{token}/uploads/{uploadId}/parts/{partNo}")
  @operationId("putPublicShareUploadPart")
  op putUploadPart(
    ...SharePasswordHeader,
    ...UploadPartHeaders,
    @path token: string,
    @path uploadId: Uuid,
    @path @minValue(1) partNo: int32,
    @body body: bytes,
  ):
    | UploadPartStoredResponse
    | ErrorResponse<400>
    | ErrorResponse<401>
    | ErrorResponse<403>
    | ErrorResponse<404>
    | ErrorResponse<409>
    | ErrorResponse<410>
    | ErrorResponse<422>
    | ErrorResponse<503>;

  @post
  @route("/{token}/uploads/{uploadId}/complete")
  @operationId("completePublicShareUpload")
  op completeUpload(
    ...SharePasswordHeader,
    ...IdempotencyHeader,
    @path token: string,
    @path uploadId: Uuid,
  ):
    | UploadCompletedResponse
    | ErrorResponse<401>
    | ErrorResponse<403>
    | ErrorResponse<404>
    | ErrorResponse<409>
    | ErrorResponse<410>
    | ErrorResponse<422>;

  @delete
  @route("/{token}/uploads/{uploadId}")
  @operationId("abortPublicShareUpload")
  op abortUpload(
    ...SharePasswordHeader,
    @path token: string,
    @path uploadId: Uuid,
  ):
    | EmptyResponse<204>
    | ErrorResponse<401>
    | ErrorResponse<403>
    | ErrorResponse<404>
    | ErrorResponse<410>;
}
