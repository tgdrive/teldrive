using TypeSpec.Http;
using TypeSpec.OpenAPI;

namespace TelDrive;

enum ChannelHealth {
  unknownState: "unknown",
  healthy: "healthy",
  degraded: "degraded",
  unavailable: "unavailable",
}

model ChannelSummary {
  id: int64;
  name: string;
  selected: boolean;
  health: ChannelHealth;
  createdAt: utcDateTime;
}

model DiscoveredChannel {
  id: int64;
  name: string;
}
model ChannelCreateRequest {
  /** Optional display name. Telegram may choose the final title. */
  @maxLength(120)
  name?: string;

  selected?: boolean = true;
}

model BotCreateRequest {
  /** Telegram bot tokens. Tokens are never returned by the API. */
  @minItems(1)
  @maxItems(200)
  tokens: string[];
}

model BotSummary {
  id: int64;
  username?: string;
  enabled: boolean;
  createdAt: utcDateTime;
}

model BotCreateResponse {
  bots: BotSummary[];
  failedIndexes: int32[];
  /** Present only when existing channels need background bot provisioning. */
  jobId?: string;
}

@tag("Channels")
@route("/v1/channels")
@useAuth(TelDriveAuth)
namespace Channels {
  @get
  @operationId("listChannels")
  op list(...CursorQuery):
    | JsonResponse<200, CursorPage<ChannelSummary>>
    | ErrorResponse<401>;

  /** List Telegram channels where the authenticated user can add administrators. */
  @get
  @route("/discover")
  @operationId("discoverChannels")
  op discover():
    | JsonResponse<200, DiscoveredChannel[]>
    | ErrorResponse<401>
    | ErrorResponse<429>;

  /** Discover and upsert manageable Telegram channels without deleting missing rows. */
  @post
  @route("/sync")
  @operationId("syncChannels")
  op sync(...IdempotencyHeader):
    | JsonResponse<200, ChannelSummary[]>
    | ErrorResponse<401>
    | ErrorResponse<422>
    | ErrorResponse<429>;
  @post
  @operationId("createChannel")
  op create(...IdempotencyHeader, @body body: ChannelCreateRequest):
    | JsonResponse<201, ChannelSummary>
    | ErrorResponse<401>
    | ErrorResponse<409>
    | ErrorResponse<422>
    | ErrorResponse<429>;

  @post
  @route("/{channelId}/select")
  @operationId("selectChannel")
  op select(@path channelId: int64):
    | JsonResponse<200, ChannelSummary>
    | ErrorResponse<401>
    | ErrorResponse<404>
    | ErrorResponse<409>;

  @delete
  @route("/{channelId}")
  @operationId("deleteChannel")
  op delete(@path channelId: int64):
    | EmptyResponse<204>
    | ErrorResponse<401>
    | ErrorResponse<404>
    | ErrorResponse<409>;
}

@tag("Bots")
@route("/v1/bots")
@useAuth(BearerOrCookieAuth)
namespace Bots {
  @get
  @operationId("listBots")
  op list(...CursorQuery):
    | JsonResponse<200, CursorPage<BotSummary>>
    | ErrorResponse<401>;

  @post
  @operationId("createBots")
  op create(...IdempotencyHeader, @body body: BotCreateRequest):
    | JsonResponse<200, BotCreateResponse>
    | ErrorResponse<400>
    | ErrorResponse<401>
    | ErrorResponse<422>
    | ErrorResponse<429>;
  @delete
  @route("/{botId}")
  @operationId("deleteBot")
  op delete(@path botId: int64):
    | EmptyResponse<204>
    | ErrorResponse<401>
    | ErrorResponse<404>
    | ErrorResponse<409>;
}
