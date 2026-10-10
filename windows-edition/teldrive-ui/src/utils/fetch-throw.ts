export class NetworkError extends Error {
  status?: number;
  headers?: Headers;
  data?: Response;

  constructor(message: string) {
    super(message);
    Error.captureStackTrace?.(this, this.constructor);
    this.name = this.constructor.name;
    this.message = message;
  }
}

type FetchInput = Parameters<typeof fetch>[0];
type FetchInit = Parameters<typeof fetch>[1];

function fetchThrow(input: FetchInput, init?: FetchInit): Promise<Response>;
function fetchThrow(
  customFetch: typeof fetch,
): (input: FetchInput, init?: FetchInit) => Promise<Response>;
function fetchThrow(
  inputOrCustomFetch: FetchInput | typeof fetch,
  init?: FetchInit,
): Promise<Response> | ((input: FetchInput, init?: FetchInit) => Promise<Response>) {
  const res = (data: Response): Response => {
    if (data.ok) {
      return data;
    }
    const error = new NetworkError(({ 401: "Sesión no válida", 403: "Acceso denegado", 404: "No se encontró el recurso", 409: "No se pudo completar el cambio", 429: "Demasiadas peticiones. Espera y vuelve a intentarlo" } as Record<number, string>)[data.status] || "No se pudo completar la petición al servidor");
    error.status = data.status;
    error.headers = data.headers;
    error.data = data;
    throw error;
  };

  return window.fetch(inputOrCustomFetch as FetchInput, init).then(res);
}

export default fetchThrow;
