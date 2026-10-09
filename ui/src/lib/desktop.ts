type NativeReply = { id: string; result?: unknown; error?: string };
type NativeView = {
  postMessage: (message: unknown) => void;
  addEventListener: (name: string, listener: (event: MessageEvent<NativeReply>) => void) => void;
  removeEventListener: (name: string, listener: (event: MessageEvent<NativeReply>) => void) => void;
};
function bridge(): NativeView | undefined {
  return (window as Window & { chrome?: { webview?: NativeView } }).chrome?.webview;
}
export function isDesktop() {
  return Boolean(bridge());
}
export function desktopRequest<T>(method: string, params: unknown = {}): Promise<T> {
  const view = bridge();
  if (!view)
    return Promise.reject(new Error("Esta función requiere Teldrive Desktop para Windows."));
  return new Promise((resolve, reject) => {
    const id = crypto.randomUUID();
    const handler = (event: MessageEvent<NativeReply>) => {
      if (event.data.id !== id) return;
      clearTimeout(timeout);
      view.removeEventListener("message", handler);
      if (event.data.error) reject(new Error(event.data.error));
      else resolve(event.data.result as T);
    };
    const timeout = setTimeout(() => {
      view.removeEventListener("message", handler);
      reject(new Error("El programa no respondió. Consulta Actividad en la ventana de Windows."));
    }, 120_000);
    view.addEventListener("message", handler);
    view.postMessage({ id, method, params });
  });
}
