import md5 from "md5";
import pLimit from "p-limit";


import type { components } from "@/lib/api";
import { fetchClient } from "@/utils/api";
import { formatTime, zeroPad } from "@/utils/common";
import type { UploadParams } from "./types";


function generateUUID(): string {
  // Check if crypto.randomUUID is available and call it to generate a UUID
  if (window.crypto && typeof window.crypto.randomUUID === 'function') {
      return crypto.randomUUID();
  }

  // Fallback to Date.now() if crypto.randomUUID is not available
  return Date.now().toString();
}



export const uploadChunk = <T extends {}>(
  url: string,
  body: Blob,
  params: UploadParams,
  signal: AbortSignal,
  onProgress: (progress: number) => void,
) => {
  return new Promise<T>((resolve, reject) => {
    const xhr = new XMLHttpRequest();

    const uploadUrl = new URL(url);

    for (const key of Object.keys(params)) {
      uploadUrl.searchParams.append(key, String(params[key]));
    }

    signal.addEventListener("abort", () => xhr.abort());

    xhr.open("POST", uploadUrl.href, true);
    xhr.setRequestHeader("Content-Type", "application/octet-stream");

    xhr.responseType = "json";

    xhr.upload.onprogress = (event) =>
      event.lengthComputable && onProgress((event.loaded / event.total) * 100);

    xhr.onload = () => {
      onProgress(100);
      resolve(xhr.response as T);
    };

    xhr.onabort = () => {
      reject(new Error("upload aborted"));
    };
    xhr.onerror = () => {
      reject(new Error("upload failed"));
    };
    xhr.send(body);
  });
};

export const uploadFile = async (
  file: File,
  path: string,
  chunkSize: number,
  userId: number,
  concurrency: number,
  retries: number,
  retryDelay: number,
  encyptFile: boolean,
  randomChunking: boolean,
  signal: AbortSignal,
  onProgress: (progress: number) => void,
  onChunksCompleted: (chunks: number) => void,
  onCreate: (payload: components["schemas"]["File"]) => Promise<void>,
  skipCheck = false,
) => {
  const fileName = file.name;

  if (!skipCheck) {
    const res = (
      await fetchClient.GET("/files", {
        params: {
          query: { path, name: fileName, operation: "find" },
        },
      })
    ).data;

    if (res && res.items.length > 0) {
      throw Error("file exists");
    }
  }

  const totalParts = Math.ceil(file.size / chunkSize);

  const limit = pLimit(concurrency);

  const uploadId = md5(
    `${path}/${fileName}${file.size.toString()}${formatTime(file.lastModified)}${userId}`,
  );

  const url = `${window.location.origin}/api/uploads/${uploadId}`;

  const uploadedParts = (
    await fetchClient.GET("/uploads/{id}", {
      params: {
        path: {
          id: uploadId,
        },
      },
    })
  ).data!;

  let channelId = 0;

  if (uploadedParts.length > 0) {
    channelId = uploadedParts[0].channelId;
  }

  const partUploadPromises: Promise<components["schemas"]["UploadPart"]>[] = [];

  const partProgress: number[] = [];

  for (let partIndex = 0; partIndex < totalParts; partIndex++) {
    if (
      uploadedParts?.findIndex((item) => item.partNo === partIndex + 1) > -1
    ) {
      partProgress[partIndex] = 100;
      continue;
    }

    partUploadPromises.push(
      limit(() =>
        (async () => {
          const start = partIndex * chunkSize;

          const end = Math.min(partIndex * chunkSize + chunkSize, file.size);

          const fileBlob = totalParts > 1 ? file.slice(start, end) : file;

          const partName = randomChunking
            ? md5(generateUUID())
            : totalParts > 1
              ? `${fileName}.part.${zeroPad(partIndex + 1, 3)}`
              : fileName;

          const params = {
            partName,
            fileName,
            partNo: partIndex + 1,
            encrypted: encyptFile,
            channelId,
          } as const;

          let retryCount = 0;
          let asset: components["schemas"]["UploadPart"] | null = null;

          while (retryCount <= retries) {
            try {
              asset = await uploadChunk<components["schemas"]["UploadPart"]>(
                url,
                fileBlob,
                params,
                signal,
                (progress) => {
                  partProgress[partIndex] = progress;
                },
              );
              break;
            } catch (error) {
              if (signal.aborted || retryCount === retries) {
                throw error;
              }
              retryCount++;
              partProgress[partIndex] = 0;
              await new Promise((resolve) =>
                setTimeout(resolve, retryDelay * retryCount),
              );
            }
          }

          return asset!;
        })(),
      ),
    );
  }

  const timer = setInterval(() => {
    const totalProgress = partProgress.reduce(
      (sum, progress) => sum + progress,
      0,
    );
    onProgress(totalParts > 0 ? totalProgress / totalParts : 0);

    const completedChunks = partProgress.filter((p) => p === 100).length;
    onChunksCompleted(completedChunks);
  }, 200);

  signal.addEventListener("abort", () => {
    limit.clearQueue();
    clearInterval(timer);
  });

  try {
    const parts = await Promise.all(partUploadPromises);

    const uploadParts = uploadedParts
      .concat(parts)
      .sort((a, b) => a.partNo - b.partNo)
      .map((item) => ({ id: item.partId, salt: item.salt }));

    const payload = {
      name: fileName,
      mimeType: file.type ?? "application/octet-stream",
      type: "file",
      parts: uploadParts,
      size: file.size,
      path: path ? path : "/",
      encrypted: encyptFile,
      channelId,
    } as const;

    await onCreate(payload);
    await fetchClient.DELETE("/uploads/{id}", {
      params: {
        path: {
          id: uploadId,
        },
      },
    });
    clearInterval(timer);
  } catch (error) {
    clearInterval(timer);
    throw error;
  }
};
