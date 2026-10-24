import type { FileEntry, UserProfile } from "@/api/types";
import { currentUserQueryOptions } from "@/auth/queries";
import { getDocumentSettings, updateDocumentSettings } from "@/lib/document-settings";
import { getQueryClient } from "@/lib/queryClient";

const databaseName = "teldrive-documents-v1";
export const DOCUMENT_CACHE_CHANGED = "teldrive:document-cache";
const MiB = 1024 * 1024;
type Metadata = { key: string; etag: string; size: number; usedAt: number };
let opening: Promise<IDBDatabase> | undefined;

function database() {
  if (!opening) {
    opening = new Promise<IDBDatabase>((resolve, reject) => {
      let blocked = false;
      const request = indexedDB.open(databaseName, 1);
      request.onupgradeneeded = () => {
        request.result.createObjectStore("metadata", { keyPath: "key" });
        request.result.createObjectStore("content");
        request.result.createObjectStore("control");
      };
      request.onsuccess = () => {
        const db = request.result;
        if (blocked) {
          db.close();
          return;
        }
        db.onversionchange = () => {
          db.close();
          opening = undefined;
        };
        resolve(db);
      };
      request.onerror = () => reject(request.error);
      request.onblocked = () => {
        blocked = true;
        reject(new Error("Document cache is busy in another tab"));
      };
    }).catch((error) => {
      opening = undefined;
      throw error;
    });
  }
  return opening;
}

function result<T>(request: IDBRequest<T>) {
  return new Promise<T>((resolve, reject) => {
    request.onsuccess = () => resolve(request.result);
    request.onerror = () => reject(request.error);
  });
}

async function transaction<T>(
  mode: IDBTransactionMode,
  action: (tx: IDBTransaction) => Promise<T>,
) {
  const db = await database();
  const tx = db.transaction(["metadata", "content", "control"], mode);
  const done = new Promise<void>((resolve, reject) => {
    tx.oncomplete = () => resolve();
    tx.onabort = tx.onerror = () =>
      reject(tx.error ?? new Error("Document cache transaction failed"));
  });
  // Attach rejection handling before requests start, including quota/abort errors.
  void done.catch(() => {});
  try {
    const value = await action(tx);
    await done;
    return value;
  } catch (error) {
    try {
      tx.abort();
    } catch {
      /* The transaction may already have finished. */
    }
    await done.catch(() => {});
    throw error;
  }
}

async function cached(key: string) {
  return transaction("readonly", async (tx) => {
    const [metadata, content, epoch] = await Promise.all([
      result<Metadata | undefined>(tx.objectStore("metadata").get(key)),
      result<Blob | undefined>(tx.objectStore("content").get(key)),
      result<number | undefined>(tx.objectStore("control").get("epoch")),
    ]);
    return { metadata, content, epoch: epoch ?? 0 };
  });
}

async function remove(key: string) {
  await transaction("readwrite", async (tx) => {
    tx.objectStore("metadata").delete(key);
    tx.objectStore("content").delete(key);
  });
  notify();
}

function notify() {
  window.dispatchEvent(new Event(DOCUMENT_CACHE_CHANGED));
}

/** Clear the bytes and invalidate in-flight writes in every tab, atomically. */
export async function clearDocumentCache() {
  await transaction("readwrite", async (tx) => {
    const control = tx.objectStore("control");
    const epoch = await result<number | undefined>(control.get("epoch"));
    control.put((epoch ?? 0) + 1, "epoch");
    tx.objectStore("metadata").clear();
    tx.objectStore("content").clear();
  });
  notify();
}

export async function documentCacheUsage() {
  return transaction("readonly", async (tx) => {
    // Never read all the document blobs just to display storage usage.
    const entries = await result<Metadata[]>(tx.objectStore("metadata").getAll());
    return {
      files: entries.length,
      bytes: entries.reduce((total, entry) => total + entry.size, 0),
    };
  });
}

async function trim(tx: IDBTransaction, entries: Metadata[], limit: number) {
  let size = entries.reduce((total, entry) => total + entry.size, 0);
  for (const entry of entries.sort((a, b) => a.usedAt - b.usedAt)) {
    if (size <= limit) break;
    tx.objectStore("metadata").delete(entry.key);
    tx.objectStore("content").delete(entry.key);
    size -= entry.size;
  }
}

export async function configureDocumentCache(patch: {
  cacheEnabled?: boolean;
  cacheLimitMiB?: number;
}) {
  updateDocumentSettings(patch);
  if (!getDocumentSettings().cacheEnabled) return clearDocumentCache();
  await transaction("readwrite", async (tx) => {
    const entries = await result<Metadata[]>(tx.objectStore("metadata").getAll());
    await trim(tx, entries, getDocumentSettings().cacheLimitMiB * MiB);
  });
  notify();
}

async function store(key: string, etag: string, content: Blob, expectedEpoch: number) {
  await transaction("readwrite", async (tx) => {
    const [epoch, entries] = await Promise.all([
      result<number | undefined>(tx.objectStore("control").get("epoch")),
      result<Metadata[]>(tx.objectStore("metadata").getAll()),
    ]);
    const settings = getDocumentSettings();
    if ((epoch ?? 0) !== expectedEpoch || !settings.cacheEnabled) return;
    const limit = settings.cacheLimitMiB * MiB;
    if (content.size > limit) {
      tx.objectStore("metadata").delete(key);
      tx.objectStore("content").delete(key);
      return;
    }
    const metadata: Metadata = { key, etag, size: content.size, usedAt: Date.now() };
    tx.objectStore("content").put(content, key);
    tx.objectStore("metadata").put(metadata);
    await trim(tx, [...entries.filter((entry) => entry.key !== key), metadata], limit);
  });
  notify();
}

function accountId() {
  return getQueryClient().getQueryData<UserProfile>(currentUserQueryOptions().queryKey)?.userId;
}

/**
 * PDF/ebook file bytes, not rendered pages. Always revalidate with the authenticated
 * endpoint before returning cached content; offline/errors never serve stale bytes.
 * Public shares and external URLs deliberately bypass persistent storage.
 */
export async function loadDocumentContent(
  file: FileEntry,
  url: string,
  signal?: AbortSignal,
): Promise<Blob> {
  const account = accountId();
  const target = new URL(url, location.origin);
  const ownFilePath = `/api/v1/files/${encodeURIComponent(file.id)}/content`;
  const eligible =
    account !== undefined &&
    target.origin === location.origin &&
    (target.pathname === ownFilePath || target.pathname.startsWith(`${ownFilePath}/`));
  // A rename changes the content URL's filename, not the document's identity.
  // The validator, rather than a filename or a stale listing generation, owns freshness.
  const key = JSON.stringify([account, file.id]);
  const enabled = eligible && getDocumentSettings().cacheEnabled;
  const previous = enabled ? await cached(key).catch(() => undefined) : undefined;
  const content = previous?.content instanceof Blob ? previous.content : undefined;
  const metadata = typeof previous?.metadata?.etag === "string" ? previous.metadata : undefined;
  const headers = new Headers();
  if (content && metadata && content.size === metadata.size)
    headers.set("If-None-Match", metadata.etag);
  const response = await fetch(target, { signal, headers, cache: "no-store" });
  signal?.throwIfAborted();
  if (eligible && account !== accountId())
    throw new Error("The account changed while opening this document");
  if (
    response.status === 304 &&
    headers.has("If-None-Match") &&
    content &&
    metadata &&
    (!response.headers.has("ETag") || response.headers.get("ETag") === metadata.etag)
  ) {
    // Touch only if the entry still exists: clear/disable must never resurrect it.
    await transaction("readwrite", async (tx) => {
      const entry = await result<Metadata | undefined>(tx.objectStore("metadata").get(key));
      if (entry?.etag === metadata.etag)
        tx.objectStore("metadata").put({ ...entry, usedAt: Date.now() });
    }).catch(() => {});
    signal?.throwIfAborted();
    if (account !== accountId()) throw new Error("The account changed while opening this document");
    return content;
  }
  if (!response.ok || response.status !== 200) {
    if (enabled && [401, 403, 404, 410].includes(response.status))
      await remove(key).catch(() => {});
    throw new Error(`HTTP ${response.status}`);
  }
  const bytes = await response.blob();
  signal?.throwIfAborted();
  if (eligible && account !== accountId())
    throw new Error("The account changed while opening this document");
  const etag = response.headers.get("ETag");
  if (enabled && previous) {
    // This is the user's explicit local document storage, independent of the
    // HTTP cache and its Cache-Control directives. ETags still gate every reuse.
    if (etag && !response.headers.has("Content-Range")) {
      await store(key, etag, bytes, previous.epoch).catch(() => {});
    } else await remove(key).catch(() => {});
  }
  signal?.throwIfAborted();
  if (eligible && account !== accountId())
    throw new Error("The account changed while opening this document");
  return bytes;
}
