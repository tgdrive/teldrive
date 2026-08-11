import { emitFile, resolvePath } from "@typespec/compiler";
import { getOpenAPI3 } from "@typespec/openapi3";
import { stringify } from "yaml";

const rawResponseMedia = new Map([
  ["downloadFile", [["200", "application/octet-stream"], ["206", "application/octet-stream"]]],
  ["downloadPublicShare", [["200", "application/octet-stream"], ["206", "application/octet-stream"]]],
  ["streamEvents", [["200", "text/event-stream"]]],
]);

export async function $onEmit(context) {
  const services = await getOpenAPI3(context.program, context.options);

  for (const service of services) {
    if (service.versioned) {
      for (const version of service.versions) {
        context.program.reportDiagnostics(version.diagnostics);
      }
    } else {
      context.program.reportDiagnostics(service.diagnostics);
    }
  }

  if (context.program.compilerOptions.dryRun || context.program.hasError()) {
    return;
  }

  for (const service of services) {
    if (service.versioned) {
      for (const version of service.versions) {
        await emitDocument(context, version.document);
      }
    } else {
      await emitDocument(context, service.document);
    }
  }
}

async function emitDocument(context, document) {
  markOgenRawResponses(document);
  await emitFile(context.program, {
    path: resolvePath(context.emitterOutputDir, context.options["output-file"] ?? "openapi.yaml"),
    content: stringify(document, { aliasDuplicateObjects: false }),
    newLine: context.options["new-line"],
  });
}

function markOgenRawResponses(document) {
  for (const pathItem of Object.values(document.paths ?? {})) {
    for (const operation of Object.values(pathItem ?? {})) {
      const mediaTypes = rawResponseMedia.get(operation?.operationId);
      if (!mediaTypes) {
        continue;
      }
      for (const [status, contentType] of mediaTypes) {
        const media = operation.responses?.[status]?.content?.[contentType];
        if (media) {
          media["x-ogen-raw-response"] = true;
        }
      }
    }
  }
}
