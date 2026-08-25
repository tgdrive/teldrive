using TypeSpec.Http;
using TypeSpec.OpenAPI;

namespace TelDrive;

model TelegramLoginStartRequest {
  /** Phone number in E.164 format. */
  @pattern("^\\+[1-9][0-9]{7,14}$")
  phoneNumber: string;
}

model TelegramLoginStartResponse {
  flowId: Uuid;
  expiresAt: utcDateTime;
  passwordRequired: boolean;
}

enum TelegramQRLoginState {
  pending: "pending",
  passwordRequired: "password_required",
}

model TelegramQRLoginResponse {
  flowId: Uuid;
  expiresAt: utcDateTime;
  state: TelegramQRLoginState;
  qrUrl?: string;
  qrExpiresAt?: utcDateTime;
}

model TelegramQRLoginPollRequest {
  flowId: Uuid;
}
model TelegramCodeVerifyRequest {
  flowId: Uuid;

  @minLength(1)
  @maxLength(16)
  code: string;
}

model TelegramPasswordVerifyRequest {
  flowId: Uuid;

  @minLength(1)
  password: string;
}

model RefreshTokenRequest {
  refreshToken: string;
}

model RefreshCookie {
  @cookie("teldrive_refresh")
  refreshToken: string;
}

model TokenPair {
  accessToken: string;
  refreshToken: string;
  tokenType: "Bearer";
  expiresIn: int32;
}

model CookieSession {
  authenticated: true;
  expiresAt: utcDateTime;
}

model CookieSessionResponse<Status extends int32> {
  @statusCode
  statusCode: Status;

  @header("Set-Cookie")
  setCookie: string[];

  @body
  body: CookieSession;
}

model CookieSessionNoContent<Status extends int32> {
  @statusCode
  statusCode: Status;

  @header("Set-Cookie")
  setCookie: string[];
}

enum UserRole {
  owner: "owner",
  admin: "admin",
  user: "user",
}

model UserAdminUpdateRequest {
  role?: UserRole;
  disabled?: boolean;
}

model AdminUserSummary {
  userId: int64;
  displayName?: string;
  username?: string;
  premium: boolean;
  role: UserRole;
  disabled: boolean;
  createdAt: utcDateTime;
  updatedAt: utcDateTime;
}

model UserSearchResult {
  userId: int64;
  displayName?: string;
  username?: string;
}

model UserProfile {
  userId: int64;
  displayName?: string;
  username?: string;
  premium: boolean;
  role: UserRole;
  capabilities: string[];
  createdAt: utcDateTime;
}

model ProfilePhotoResponse {
  @statusCode
  statusCode: 200;

  @header("Content-Length")
  contentLength: int64;

  @header("Cache-Control")
  cacheControl: string;

  @header("Etag")
  ETag: ETag;

  @header("Content-Disposition")
  contentDisposition: string;

  @body
  body: bytes;
}
model ApiKeyCreateRequest {
  @minLength(1)
  @maxLength(120)
  name: string;

  expiresAt?: utcDateTime;
}

model ApiKeyCreated {
  id: Uuid;
  name: string;

  /** Returned once. The server stores only a hash. */
  secret: string;

  createdAt: utcDateTime;
  expiresAt?: utcDateTime;
}

model ApiKeySummary {
  id: Uuid;
  name: string;
  createdAt: utcDateTime;
  lastUsedAt?: utcDateTime;
  expiresAt?: utcDateTime;
  revokedAt?: utcDateTime;
}

model SessionSummary {
  id: Uuid;
  current: boolean;
  createdAt: utcDateTime;
  lastUsedAt?: utcDateTime;
  expiresAt: utcDateTime;
  revokedAt?: utcDateTime;
}

@tag("Authentication")
@route("/v1/auth")
@useAuth(NoAuth)
namespace Authentication {
  /** Begin Telegram authentication and send a login code. */
  @post
  @route("/telegram/start")
  @operationId("telegramLoginStart")
  op telegramStart(
    ...IdempotencyHeader,
    @body body: TelegramLoginStartRequest,
  ):
    | JsonResponse<201, TelegramLoginStartResponse>
    | ErrorResponse<400>
    | ErrorResponse<409>
    | ErrorResponse<422>
    | ErrorResponse<429>;

  /** Begin a resumable Telegram QR login flow. */
  @post
  @route("/telegram/qr/start")
  @operationId("telegramQRLoginStart")
  op qrStart(...IdempotencyHeader):
    | JsonResponse<201, TelegramQRLoginResponse>
    | ErrorResponse<400>
    | ErrorResponse<409>
    | ErrorResponse<422>
    | ErrorResponse<429>;

  /** Poll a Telegram QR login flow. Any API replica can resume the encrypted flow state. */
  @post
  @route("/telegram/qr/poll")
  @operationId("telegramQRLoginPoll")
  op qrPoll(...IdempotencyHeader, @body body: TelegramQRLoginPollRequest):
    | JsonResponse<200, TokenPair>
    | JsonResponse<202, TelegramQRLoginResponse>
    | ErrorResponse<400>
    | ErrorResponse<401>
    | ErrorResponse<410>
    | ErrorResponse<422>
    | ErrorResponse<429>;
  /** Verify the Telegram login code. */
  @post
  @route("/telegram/verify-code")
  @operationId("telegramLoginVerifyCode")
  op verifyCode(...IdempotencyHeader, @body body: TelegramCodeVerifyRequest):
    | JsonResponse<200, TokenPair>
    | JsonResponse<202, TelegramLoginStartResponse>
    | ErrorResponse<400>
    | ErrorResponse<401>
    | ErrorResponse<410>
    | ErrorResponse<422>
    | ErrorResponse<429>;

  /** Complete Telegram two-step verification. */
  @post
  @route("/telegram/verify-password")
  @operationId("telegramLoginVerifyPassword")
  op verifyPassword(
    ...IdempotencyHeader,
    @body body: TelegramPasswordVerifyRequest,
  ):
    | JsonResponse<200, TokenPair>
    | ErrorResponse<400>
    | ErrorResponse<401>
    | ErrorResponse<410>
    | ErrorResponse<422>
    | ErrorResponse<429>;

  /** Rotate an access/refresh token pair. */
  @post
  @route("/refresh")
  @operationId("refreshSession")
  op refresh(@body body: RefreshTokenRequest):
    | JsonResponse<200, TokenPair>
    | ErrorResponse<401>
    | ErrorResponse<429>;

  /** Complete Telegram code verification and establish an HttpOnly cookie session. */
  @post
  @route("/cookie/telegram/verify-code")
  @operationId("cookieTelegramLoginVerifyCode")
  op cookieVerifyCode(
    ...IdempotencyHeader,
    @body body: TelegramCodeVerifyRequest,
  ):
    | CookieSessionResponse<200>
    | JsonResponse<202, TelegramLoginStartResponse>
    | ErrorResponse<400>
    | ErrorResponse<401>
    | ErrorResponse<410>
    | ErrorResponse<422>
    | ErrorResponse<429>;

  /** Complete Telegram password verification and establish an HttpOnly cookie session. */
  @post
  @route("/cookie/telegram/verify-password")
  @operationId("cookieTelegramLoginVerifyPassword")
  op cookieVerifyPassword(
    ...IdempotencyHeader,
    @body body: TelegramPasswordVerifyRequest,
  ):
    | CookieSessionResponse<200>
    | ErrorResponse<400>
    | ErrorResponse<401>
    | ErrorResponse<410>
    | ErrorResponse<422>
    | ErrorResponse<429>;

  /** Poll Telegram QR login and establish an HttpOnly cookie session when authorized. */
  @post
  @route("/cookie/telegram/qr/poll")
  @operationId("cookieTelegramQRLoginPoll")
  op cookieQrPoll(
    ...IdempotencyHeader,
    @body body: TelegramQRLoginPollRequest,
  ):
    | CookieSessionResponse<200>
    | JsonResponse<202, TelegramQRLoginResponse>
    | ErrorResponse<400>
    | ErrorResponse<401>
    | ErrorResponse<410>
    | ErrorResponse<422>
    | ErrorResponse<429>;

  /** Rotate session cookies using the HttpOnly refresh cookie. */
  @post
  @route("/cookie/refresh")
  @operationId("refreshCookieSession")
  op cookieRefresh(...RefreshCookie):
    | CookieSessionResponse<200>
    | ErrorResponse<401>
    | ErrorResponse<429>;

  /** Revoke the cookie session and clear its HttpOnly cookies. */
  @post
  @route("/cookie/logout")
  @operationId("logoutCookieSession")
  @useAuth(CookieAuth)
  op cookieLogout(): CookieSessionNoContent<204> | ErrorResponse<401>;
}

@tag("Account")
@route("/v1")
@useAuth(TelDriveAuth)
namespace Account {
  /** Return the authenticated user profile. */
  @get
  @route("/me")
  @operationId("getCurrentUser")
  op getCurrentUser(): JsonResponse<200, UserProfile> | ErrorResponse<401>;

  /** Return the authenticated Telegram user's small profile photo. */
  @get
  @route("/me/photo")
  @operationId("getProfilePhoto")
  op getProfilePhoto():
    | ProfilePhotoResponse
    | EmptyResponse<204>
    | ErrorResponse<401>
    | ErrorResponse<429>;
  /** Revoke the current session. */
  @post
  @route("/auth/logout")
  @operationId("logoutSession")
  op logout(): EmptyResponse<204> | ErrorResponse<401>;
}

@tag("Users")
@route("/v1/users")
@useAuth(TelDriveAuth)
namespace Users {
  @get
  @route("/search")
  @operationId("searchUsers")
  op search(
    @query
    @minLength(1)
    @maxLength(128)
    search: string,
  ):
    | JsonResponse<200, UserSearchResult[]>
    | ErrorResponse<401>
    | ErrorResponse<422>;
}

@tag("Administration")
@route("/v1/admin/users")
@useAuth(TelDriveAuth)
namespace Administration {
  @get
  @operationId("listAdminUsers")
  op list(@query @maxLength(128) search?: string):
    | JsonResponse<200, AdminUserSummary[]>
    | ErrorResponse<401>
    | ErrorResponse<403>;

  @patch
  @route("/{userId}")
  @operationId("updateAdminUser")
  op update(@path userId: int64, @body body: UserAdminUpdateRequest):
    | JsonResponse<200, AdminUserSummary>
    | ErrorResponse<401>
    | ErrorResponse<403>
    | ErrorResponse<404>
    | ErrorResponse<422>;

  @post
  @route("/{userId}/revoke-access")
  @operationId("revokeAdminUserAccess")
  op revokeAccess(@path userId: int64):
    | EmptyResponse<204>
    | ErrorResponse<401>
    | ErrorResponse<403>
    | ErrorResponse<404>;
}

@tag("API keys")
@route("/v1/api-keys")
@useAuth(BearerOrCookieAuth)
namespace ApiKeys {
  @get
  @operationId("listApiKeys")
  op list(...CursorQuery):
    | JsonResponse<200, CursorPage<ApiKeySummary>>
    | ErrorResponse<401>;

  @post
  @operationId("createApiKey")
  op create(...IdempotencyHeader, @body body: ApiKeyCreateRequest):
    | JsonResponse<201, ApiKeyCreated>
    | ErrorResponse<401>
    | ErrorResponse<409>
    | ErrorResponse<422>;

  @delete
  @route("/{apiKeyId}")
  @operationId("revokeApiKey")
  op revoke(@path apiKeyId: Uuid):
    | EmptyResponse<204>
    | ErrorResponse<401>
    | ErrorResponse<404>;
}

@tag("Sessions")
@route("/v1/sessions")
@useAuth(BearerOrCookieAuth)
namespace Sessions {
  /** List all TelDrive sessions for the authenticated Telegram account. */
  @get
  @operationId("listSessions")
  op list(...CursorQuery):
    | JsonResponse<200, CursorPage<SessionSummary>>
    | ErrorResponse<401>;

  /** Revoke one TelDrive session. Revoking the current session invalidates its bearer token immediately. */
  @delete
  @route("/{sessionId}")
  @operationId("revokeSession")
  op revoke(@path sessionId: Uuid):
    | EmptyResponse<204>
    | ErrorResponse<401>
    | ErrorResponse<404>;
}
