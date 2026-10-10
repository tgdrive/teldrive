export type ApiErrorDetails = Record<string, unknown>;

export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly details?: ApiErrorDetails;
  readonly requestId?: string;
  readonly retryAfterSeconds?: number;

  constructor({
    status,
    code,
    message,
    details,
    requestId,
    retryAfterSeconds,
  }: {
    status: number;
    code: string;
    message: string;
    details?: ApiErrorDetails;
    requestId?: string;
    retryAfterSeconds?: number;
  }) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
    this.details = details;
    this.requestId = requestId;
    this.retryAfterSeconds = retryAfterSeconds;
  }

  get retryable() {
    return this.status === 408 || this.status === 425 || this.status === 429 || this.status >= 500;
  }
}

function envelope(
  error: unknown,
): { code?: unknown; message?: unknown; details?: unknown } | undefined {
  if (!error || typeof error !== "object") return undefined;
  const candidate = error as { error?: unknown };
  if (!candidate.error || typeof candidate.error !== "object") return undefined;
  return candidate.error as { code?: unknown; message?: unknown; details?: unknown };
}

export function normalizeApiError(error: unknown, response?: Response): ApiError {
  if (error instanceof ApiError) return error;
  const parsed = envelope(error);
  const status = response?.status ?? 0;
  const requestId = response?.headers.get("X-Request-ID") ?? undefined;
  const retryAfter = Number(response?.headers.get("Retry-After"));
  const fallback =
    error instanceof Error
      ? error.message
      : status
        ? `Request failed with status ${status}`
        : "Network request failed";
  return new ApiError({
    status,
    code:
      typeof parsed?.code === "string" ? parsed.code : status ? `http_${status}` : "network_error",
    message: typeof parsed?.message === "string" ? parsed.message : fallback,
    details:
      parsed?.details && typeof parsed.details === "object"
        ? (parsed.details as ApiErrorDetails)
        : undefined,
    requestId,
    retryAfterSeconds: Number.isFinite(retryAfter) ? retryAfter : undefined,
  });
}

type ApiResult<T> = { data?: T; error?: unknown; response: Response };

export async function unwrap<T>(result: ApiResult<T> | Promise<ApiResult<T>>): Promise<T> {
  const { data, error, response } = await result;
  if (error !== undefined || !response.ok) throw normalizeApiError(error, response);
  if (data === undefined && response.status !== 204) {
    throw new ApiError({
      status: response.status,
      code: "invalid_response",
      message: "The server returned an incomplete response.",
    });
  }
  return data as T;
}

export function isUnauthorized(error: unknown) {
  return error instanceof ApiError && error.status === 401;
}

export function invalidResponse(
  message = "The server returned data that does not match the current OpenAPI contract.",
) {
  return new ApiError({ status: 200, code: "invalid_response", message });
}

export function userMessage(error: unknown): string {
  const normalized = normalizeApiError(error);
  switch (normalized.status) {
    case 0:
      return "Teldrive could not reach the server. Check your connection and try again.";
    case 400:
      return normalized.message || "The request was not valid.";
    case 401:
      return "Your sign-in has expired. Sign in again to continue.";
    case 403:
      return "You do not have permission to perform this action.";
    case 404:
      return "The requested item no longer exists.";
    case 409:
      return normalized.message || "That change conflicts with an existing item.";
    case 410:
      return "This upload or share has expired.";
    case 412:
      return "This item changed on another device. Refresh before trying again.";
    case 413:
      return "The selected file is larger than the server allows.";
    case 416:
      return "The requested file range is not available.";
    case 422:
      return normalized.message || "Some values need to be corrected.";
    case 429:
      return "Teldrive is receiving too many requests. Try again shortly.";
    default:
      return normalized.status >= 500
        ? "Teldrive encountered a server error. Your data was not changed."
        : normalized.message;
  }
}
