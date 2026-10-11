using TypeSpec.Http;
using TypeSpec.OpenAPI;

namespace TelDrive;

model EventTicketAuth is ApiKeyAuth<ApiKeyLocation.query, "ticket">;
alias EventStreamAuth = BearerAuth | ExternalApiKeyAuth | EventTicketAuth;

model EventStreamTicket {
  /** Short-lived credential suitable for a browser EventSource URL. */
  ticket: string;

  expiresAt: utcDateTime;
}

model EventStreamResponse {
  @statusCode
  statusCode: 200;

  @header("Content-Type")
  contentType: "text/event-stream";

  @header("Cache-Control")
  cacheControl: "no-cache, no-transform";

  @body
  body: bytes;
}

@tag("Events")
@route("/v1/events")
namespace Events {
  /** Create a short-lived browser-compatible event stream credential. */
  @post
  @route("/ticket")
  @operationId("createEventStreamTicket")
  @useAuth(TelDriveAuth)
  op createTicket():
    | JsonResponse<200, EventStreamTicket>
    | ErrorResponse<401>
    | ErrorResponse<503>;

  /** Subscribe to ordered, replayable events for the authenticated user. */
  @get
  @operationId("streamEvents")
  @useAuth(EventStreamAuth)
  op stream(
    /** Resume after the last successfully processed event. */
    @header("Last-Event-Id") lastEventId?: string,

    /** Header-equivalent cursor for clients that cannot set Last-Event-ID. */
    @query after?: int64,

    /** Optional comma-separated event types to include. */
    @query types?: string[],
  ):
    | EventStreamResponse
    | ErrorResponse<401>
    | ErrorResponse<422>
    | ErrorResponse<429>
    | ErrorResponse<503>;
}
