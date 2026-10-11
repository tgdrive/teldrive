import "@typespec/http";
import "@typespec/openapi";
import "@typespec/openapi3";

import "./common.tsp";
import "./system.tsp";
import "./auth.tsp";
import "./channels.tsp";
import "./events.tsp";
import "./files.tsp";
import "./uploads.tsp";
import "./shares.tsp";
import "./jobs.tsp";
import "./storage.tsp";

using TypeSpec.Http;
using TypeSpec.OpenAPI;

/**
 * Backend-first API for rclone and other TelDrive clients. Upload sessions are
 * durable, API instances are stateless, and completed files are immediately readable.
 */
@service(#{ title: "TelDrive API" })
@info(#{ version: "1.0.0", summary: "Telegram-backed cloud drive API" })
@server("/api", "Current TelDrive server")
namespace TelDrive;
