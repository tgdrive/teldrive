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
      message: "El servidor devolvió una respuesta incompleta.",
    });
  }
  return data as T;
}

export function isUnauthorized(error: unknown) {
  return error instanceof ApiError && error.status === 401;
}

export function invalidResponse(
  message = "La respuesta del servidor no coincide con el contrato actual de la API.",
) {
  return new ApiError({ status: 200, code: "invalid_response", message });
}

export function userMessage(error: unknown): string {
  const normalized = normalizeApiError(error);
  switch (normalized.status) {
    case 0:
      return "No se pudo conectar con el servidor. Comprueba la conexión y vuelve a intentarlo.";
    case 400:
      return normalized.message || "La solicitud no era válida.";
    case 401:
      return "Tu sesión ha vencido. Inicia sesión de nuevo para continuar.";
    case 403:
      return "No tienes permiso para realizar esta acción.";
    case 404:
      return "El elemento solicitado ya no existe.";
    case 409:
      return normalized.message || "El cambio entra en conflicto con un elemento existente.";
    case 410:
      return "Esta subida o enlace compartido ha vencido.";
    case 412:
      return "Este elemento cambió en otro dispositivo. Actualiza antes de reintentarlo.";
    case 413:
      return "El archivo seleccionado supera el tamaño permitido por el servidor.";
    case 416:
      return "El intervalo solicitado del archivo no está disponible.";
    case 422:
      return normalized.message || "Revisa los valores señalados.";
    case 429:
      return "Teldrive está recibiendo demasiadas solicitudes. Reintenta en unos momentos.";
    default:
      return normalized.status >= 500
        ? "El servidor devolvió un error. Tus datos no se modificaron."
        : normalized.message;
  }
}
