import { createServer, type IncomingHttpHeaders } from "node:http";

// Chromium bypasses Playwright route interception for <a download> requests.
// Keep the app and fixture download on one real origin so tests can observe
// successful native downloads and the cookies actually sent by the browser.
export async function startDownloadServer(upstream: string, downloadPath: string) {
  const requests: { url: string; headers: IncomingHttpHeaders }[] = [];
  const server = createServer(async (request, response) => {
    try {
      if (request.url === downloadPath) {
        requests.push({ url: request.url, headers: request.headers });
        response.writeHead(200, {
          "Content-Type": "application/octet-stream",
          "Content-Disposition": "attachment",
        });
        response.end("streamed fixture");
        return;
      }
      const asset = await fetch(new URL(request.url ?? "/", upstream));
      response.writeHead(asset.status, {
        "Content-Type": asset.headers.get("Content-Type") ?? "application/octet-stream",
      });
      response.end(Buffer.from(await asset.arrayBuffer()));
    } catch (error) {
      response.writeHead(502);
      response.end(String(error));
    }
  });
  await new Promise<void>((resolve, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", resolve);
  });
  const address = server.address();
  if (!address || typeof address === "string") throw new Error("Missing download server address");
  return {
    origin: `http://127.0.0.1:${address.port}`,
    requests,
    close: () =>
      new Promise<void>((resolve, reject) => {
        server.close((error) => (error ? reject(error) : resolve()));
        server.closeAllConnections();
      }),
  };
}
