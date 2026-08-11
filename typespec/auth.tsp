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

model BrowserRefreshCookie {
  @cookie("teldrive_refresh")
  refreshToken: string;
}

model TokenPair {
  accessToken: string;
  refreshToken: string;
  tokenType: "Bearer";
  expiresIn: int32;
}

model BrowserSession {
  authenticated: true;
  expiresAt: utcDateTime;
}

model BrowserSessionResponse<Status extends int32> {
  @statusCode
  statusCode: Status;

  @header("Set-Cookie")
  setCookie: string[];

  @body
  body: BrowserSession;
}

model BrowserSessionNoContent<Status extends int32> {
  @statusCode
  statusCode: Status;

  @header("Set-Cookie")
  setCookie: string[];
}

model UserProfile {
  userId: int64;
  displayName?: string;
  username?: string;
  premium: boolean;
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

  /** Complete Telegram code verification for the browser UI and establish an HttpOnly cookie session. */
  @post
  @route("/browser/telegram/verify-code")
  @operationId("browserTelegramLoginVerifyCode")
  op browserVerifyCode(
    ...IdempotencyHeader,
    @body body: TelegramCodeVerifyRequest,
  ):
    | BrowserSessionResponse<200>
    | JsonResponse<202, TelegramLoginStartResponse>
    | ErrorResponse<400>
    | ErrorResponse<401>
    | ErrorResponse<410>
    | ErrorResponse<422>
    | ErrorResponse<429>;

  /** Complete Telegram password verification for the browser UI and establish an HttpOnly cookie session. */
  @post
  @route("/browser/telegram/verify-password")
  @operationId("browserTelegramLoginVerifyPassword")
  op browserVerifyPassword(
    ...IdempotencyHeader,
    @body body: TelegramPasswordVerifyRequest,
  ):
    | BrowserSessionResponse<200>
    | ErrorResponse<400>
    | ErrorResponse<401>
    | ErrorResponse<410>
    | ErrorResponse<422>
    | ErrorResponse<429>;

  /** Poll Telegram QR login for the browser UI and establish an HttpOnly cookie session when authorized. */
  @post
  @route("/browser/telegram/qr/poll")
  @operationId("browserTelegramQRLoginPoll")
  op browserQrPoll(
    ...IdempotencyHeader,
    @body body: TelegramQRLoginPollRequest,
  ):
    | BrowserSessionResponse<200>
    | JsonResponse<202, TelegramQRLoginResponse>
    | ErrorResponse<400>
    | ErrorResponse<401>
    | ErrorResponse<410>
    | ErrorResponse<422>
    | ErrorResponse<429>;

  /** Rotate browser session cookies using the HttpOnly refresh cookie. */
  @post
  @route("/browser/refresh")
  @operationId("refreshBrowserSession")
  op browserRefresh(...BrowserRefreshCookie):
    | BrowserSessionResponse<200>
    | ErrorResponse<401>
    | ErrorResponse<429>;

  /** Revoke the browser session and clear its HttpOnly cookies. */
  @post
  @route("/browser/logout")
  @operationId("logoutBrowserSession")
  @useAuth(BrowserCookieAuth)
  op browserLogout(): BrowserSessionNoContent<204> | ErrorResponse<401>;
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
