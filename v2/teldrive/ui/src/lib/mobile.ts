type MobileRequest = { id: string; method: "download"; url: string; filename: string; headers: Record<string, string> };
type AndroidBridge = { postMessage: (message: string) => void; onmessage?: (event: {data: string}) => void };
const pending = new Map<string, {resolve: () => void; reject: (error: Error) => void; timeout: ReturnType<typeof setTimeout>}>();

/** Returns false in a normal browser; native apps stream directly to device storage. */
export async function nativeFileDownload(url: string, filename: string, headers: HeadersInit = {}): Promise<boolean> {
  const mobile = window as Window & { teldriveMobile?: AndroidBridge; webkit?: {messageHandlers?: {teldriveMobile?: {postMessage: (request: MobileRequest) => Promise<unknown>}}} };
  const request: MobileRequest = {id:crypto.randomUUID(), method:"download", url, filename, headers:Object.fromEntries(new Headers(headers))};
  const ios = mobile.webkit?.messageHandlers?.teldriveMobile;
  if (ios) { await ios.postMessage(request); return true; }
  const android = mobile.teldriveMobile;
  if (!android) return false;
  android.onmessage = event => {
    try {
      const reply = JSON.parse(event.data) as {id: string; error?: string};
      const operation = pending.get(reply.id); if (!operation) return;
      clearTimeout(operation.timeout); pending.delete(reply.id);
      if (reply.error) operation.reject(new Error(reply.error)); else operation.resolve();
    } catch { /* Ignore unrelated messages. */ }
  };
  await new Promise<void>((resolve, reject) => {
    const timeout = setTimeout(() => {pending.delete(request.id); reject(new Error("La app no pudo iniciar la descarga."));}, 15_000);
    pending.set(request.id, {resolve, reject, timeout});
    android.postMessage(JSON.stringify(request));
  });
  return true;
}
