import { readdirSync, readFileSync, statSync } from "node:fs";
import { join, relative } from "node:path";
import { expect, test } from "@playwright/test";
import { APP_LOCALES, STRINGS } from "../src/lib/locales";

const sourceRoot = join(process.cwd(), "src");
const placeholders = (value: string) => (value.match(/\{\w+\}/g) ?? []).toSorted().join(",");

test("every locale contains all English keys and matching placeholders", () => {
  const englishKeys = Object.keys(STRINGS.en).toSorted();
  for (const { code } of APP_LOCALES) {
    const strings = STRINGS[code];
    expect(Object.keys(strings).toSorted(), code).toEqual(englishKeys);
    expect(
      englishKeys.filter((key) => !strings[key].trim()),
      `${code}: empty translations`,
    ).toEqual([]);
    expect(
      englishKeys.filter((key) => placeholders(strings[key]) !== placeholders(STRINGS.en[key])),
      `${code}: mismatched placeholders`,
    ).toEqual([]);
  }
});

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
