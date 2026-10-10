import { expect, test } from "bun:test"
import { serverConfig, rcloneConfig, psQuote } from "../src/utils/integrations"
import { storageUsage } from "../src/utils/storage"
import schema from "../src/config/cli-schema.json"
import { cliLabels } from "../src/config/cli-labels"

test("all CLI options are covered and export a parseable TOML", () => {
  expect(schema).toHaveLength(84)
  expect(cliLabels).toHaveLength(schema.length)
  const values = Object.fromEntries(schema.map((field) => [field.path, field.default]))
  const config = Bun.TOML.parse(serverConfig(values)) as any
  expect(config.server.port).toBe(8080)
  expect(config.tg.uploads.threads).toBe(8)
  expect(config["cron-jobs"].enable).toBe(true)
  expect(config.log.http["skip-paths"]).toEqual(["/health", "/metrics"])
  expect(config.jwt["allowed-users"]).toEqual([])
})
test("TOML quoting preserves secrets without injecting new sections", () => {
  const secret = 'a"\n[jwt]\nsecret="overwritten'
  const parsed = Bun.TOML.parse(serverConfig({ "jwt.secret": secret })) as any
  expect(parsed.jwt.secret).toBe(secret)
  expect(() => serverConfig({ "server.port": "8080\n[evil]" })).toThrow()
})
test("rclone config matches the bundled fork and prevents INI injection", () => {
  const input = { name: "teldrive", host: "http://localhost:8080", token: "test-token", chunk: "512Mi", concurrency: "2", channel: "", encrypt: false }
  expect(rcloneConfig(input)).toContain("api_host = http://localhost:8080")
  expect(rcloneConfig(input)).toContain("access_token = test-token")
  expect(rcloneConfig(input)).toContain("hash_enabled = true")
  expect(() => rcloneConfig({ ...input, token: "token\ntype = other" })).toThrow()
  expect(() => rcloneConfig({ ...input, host: "http://localhost:8080/api" })).toThrow()
  expect(() => rcloneConfig({ ...input, name: "x]\n[evil" })).toThrow()
})
test("PowerShell paths are literal even with apostrophes and substitution", () => {
  expect(psQuote("D:\\O'Brien\\$(echo x)")).toBe("'D:\\O''Brien\\$(echo x)'")
})
test("storage includes retained files, excludes folder totals, handles quota overflow", () => {
  const data = [{ category: "image", totalSize: 3000000000 }, { category: "folder", totalSize: 3000000000 }]
  expect(storageUsage(data, 10)).toEqual({ used: 3000000000, total: 10000000000, available: 7000000000, percent: 30, exceeded: false })
  expect(storageUsage(data, 2).percent).toBe(100)
  expect(storageUsage(data, 2).available).toBe(0)
  expect(storageUsage(data, 0).total).toBe(0)
  expect(storageUsage(data, Number.NaN).total).toBe(0)
})
