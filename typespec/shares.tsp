using TypeSpec.Http;
using TypeSpec.OpenAPI;

namespace TelDrive;

model ShareCreateRequest {
  /** Optional password. The server stores only a password hash. */
  @minLength(1)
  password?: string;

  expiresAt?: utcDateTime;

  @minValue(1)
  maxDownloads?: int64;
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
  createdAt: utcDateTime;
}

model ShareSummary {
  id: Uuid;
  fileId: Uuid;
  passwordProtected: boolean;
  expiresAt?: utcDateTime;
  maxDownloads?: int64;
  downloadCount: int64;
  createdAt: utcDateTime;
  revokedAt?: utcDateTime;
}

model PublicShare {
  id: Uuid;
  file: FileEntry;
  passwordProtected: boolean;
  expiresAt?: utcDateTime;
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
}
