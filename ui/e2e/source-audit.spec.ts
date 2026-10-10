import { expect, test } from "@playwright/test";
import { readFileSync, readdirSync, statSync } from "node:fs";
import { join, relative } from "node:path";

const sourceRoot = join(process.cwd(), "src");
const exactCopies = new Set([
  "components/task-launcher.tsx",
  "components/task-status-chip.tsx",
  "routes/tasks.tsx",
  "routes/tasks_.$id.tsx",
  "routes/_settings.settings.periodic-jobs.tsx",
]);

function sourceFiles(directory: string): string[] {
  return readdirSync(directory).flatMap((entry) => {
    const path = join(directory, entry);
    return statSync(path).isDirectory()
      ? sourceFiles(path)
      : /\.(ts|tsx|css)$/.test(path)
        ? [path]
        : [];
  });
}

test("all active API consumers import generated OpenAPI types", () => {
  const apiConsumers = sourceFiles(sourceRoot).filter((path) => {
    if (path.endsWith("api/schema.ts") || path.endsWith("gen/api.d.ts")) return false;
    if (!path.endsWith(".ts") && !path.endsWith(".tsx")) return false;
    const source = readFileSync(path, "utf8");
    return (
      source.includes("$api.") || source.includes("components[") || source.includes("operations[")
    );
  });
  expect(apiConsumers.length).toBeGreaterThan(10);
  for (const path of apiConsumers) {
    const source = readFileSync(path, "utf8");
    const importsGeneratedTypes =
      /from\s+["'](?:@\/api(?:\/client|\/schema)?|[^"']*\/api(?:\/client|\/schema)?|\.\/schema)["']/.test(
        source,
      );
    expect(importsGeneratedTypes, relative(sourceRoot, path)).toBe(true);
  }
});
