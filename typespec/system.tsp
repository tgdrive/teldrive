using TypeSpec.Http;
using TypeSpec.OpenAPI;

namespace TelDrive;

model HealthStatus {
  status: "ok" | "degraded";
  version: string;
}

@tag("System")
@route("/health")
@useAuth(NoAuth)
namespace System {
  /** Process liveness check. */
  @get
  @route("/live")
  @operationId("healthLive")
  op live(): JsonResponse<200, HealthStatus>;

  /** Readiness check for PostgreSQL, migrations, and required runtime state. */
  @get
  @route("/ready")
  @operationId("healthReady")
  op ready(): JsonResponse<200, HealthStatus> | ErrorResponse<503>;
}
