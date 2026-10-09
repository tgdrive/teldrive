using TypeSpec.Http;
using TypeSpec.OpenAPI;
namespace TelDrive;

@tag("Shares")
@route("/v1/shared/spam")
@useAuth(TelDriveAuth)
namespace IncomingSpam {
  @get
  @operationId("listIncomingSpam")
  op list(): JsonResponse<200, FileEntry[]> | ErrorResponse<401>;

  @post
  @route("/{fileId}")
  @operationId("markIncomingSpam")
  op mark(@path fileId: Uuid): EmptyResponse<204> | ErrorResponse<401> | ErrorResponse<404>;

  @delete
  @route("/{fileId}")
  @operationId("restoreIncomingSpam")
  op restore(@path fileId: Uuid): EmptyResponse<204> | ErrorResponse<401>;

  @delete
  @route("/{fileId}/dismiss")
  @operationId("dismissIncomingSpam")
  op dismiss(@path fileId: Uuid): EmptyResponse<204> | ErrorResponse<401>;
}
