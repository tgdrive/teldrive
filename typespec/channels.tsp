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
model ChannelSyncRequest {
  /** Only these discovered broadcast channels will be registered. Existing channels are retained. */
  channelIds: int64[];
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
  /** Telegram display name, independently of the public username. */
  name?: string;
  username?: string;
  enabled: boolean;
  createdAt: utcDateTime;
}

model BotCreateResponse {
  bots: BotSummary[];
  failedIndexes: int32[];

  /** First provisioning task, retained for older clients. Use jobIds for all tasks. */
  jobId?: string;

  /** Each saved bot has an independent provisioning task. */
  jobIds: string[];
}

model BotProvisionResponse {
  jobId: string;
}

model BotBulkProvisionResponse {
  jobIds: string[];
}

model ManagedBotCreateRequest {
  @minValue(1) managerBotId: int64;
  @minLength(1) @maxLength(64) name: string;
  @minLength(5) @maxLength(32) username: string;
}
model ManagedBotCreateResponse {
  bot: BotSummary;

  /** True if an interrupted creation was recovered using the same username. */
  recovered: boolean;

  jobId?: string;
}
model BotManagerStatus {
  id: int64;
  username: string;
  canManageBots: boolean;
}
model BotUsernameRequest {
  @minLength(5) @maxLength(32) username: string;
}
model BotUsernameAvailability {
  available: boolean;
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

  /** List broadcast channels where the authenticated user can add administrators. Groups are excluded. */
  @get
  @route("/discover")
  @operationId("discoverChannels")
  op discover():
    | JsonResponse<200, DiscoveredChannel[]>
    | ErrorResponse<401>
    | ErrorResponse<429>;

  /** Register only chosen manageable broadcast channels, without deleting existing channels. */
  @post
  @route("/sync")
  @operationId("syncChannels")
  op sync(...IdempotencyHeader, @body body: ChannelSyncRequest):
    | JsonResponse<200, ChannelSummary[]>
    | ErrorResponse<401>
    | ErrorResponse<404>
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
  @route("/{botId}/manager")
  @operationId("getBotManager")
  op manager(@path botId: int64):
    | JsonResponse<200, BotManagerStatus>
    | ErrorResponse<401>
    | ErrorResponse<404>
    | ErrorResponse<422>
    | ErrorResponse<429>
    | ErrorResponse<503>;

  @post
  @route("/managed/check-username")
  @operationId("checkManagedBotUsername")
  op checkUsername(@body body: BotUsernameRequest):
    | JsonResponse<200, BotUsernameAvailability>
    | ErrorResponse<401>
    | ErrorResponse<409>
    | ErrorResponse<422>
    | ErrorResponse<429>
    | ErrorResponse<503>;

  @post
  @route("/managed")
  @operationId("createManagedBot")
  op createManaged(...IdempotencyHeader, @body body: ManagedBotCreateRequest):
    | JsonResponse<200, ManagedBotCreateResponse>
    | ErrorResponse<401>
    | ErrorResponse<404>
    | ErrorResponse<409>
    | ErrorResponse<422>
    | ErrorResponse<429>
    | ErrorResponse<503>;

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
  /** Re-verify all saved bots, with one independent task per bot. */
  @post
  @route("/provision")
  @operationId("provisionBots")
  op provisionAll(...IdempotencyHeader):
    | JsonResponse<202, BotBulkProvisionResponse>
    | ErrorResponse<401>
    | ErrorResponse<422>
    | ErrorResponse<429>;

  /** Re-verify a saved bot and repair its access to the user's registered channels. */
  @post
  @route("/{botId}/provision")
  @operationId("provisionBot")
  op provision(@path botId: int64, ...IdempotencyHeader):
    | JsonResponse<202, BotProvisionResponse>
    | ErrorResponse<401>
    | ErrorResponse<404>
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
