import type { Session } from "@/types";
import { partial } from "filesize";
import { useSettingsStore } from "./stores/settings";

export const navigateToExternalUrl = (url: string, shouldOpenNewTab = true) => {
  if (shouldOpenNewTab) {
    window.open(url, "_blank");
  } else {
    window.location.href = url;
  }
};

export function downloadFile(url: string, name: string) {
  const link = document.createElement("a");
  link.href = url;
  link.download = name;
  link.rel = "noopener";
  document.body.appendChild(link);
  link.click();
  link.remove();
}

export const chainLinks = (path: string) => {
  let pathsoFar = "/";
  const chains = [["Mi unidad", pathsoFar]] as Array<[string, string]>;
  if (!path || path === "/") {
    return chains;
  }
  const paths = path?.split("/").slice(1);
  for (const path of paths) {
    const decodedPath = decodeURIComponent(path);
    chains.push([decodedPath, pathsoFar + decodedPath]);
    pathsoFar = `${pathsoFar + decodedPath}/`;
  }

  return chains;
};

export const chainSharedLinks = (root: string, path: string) => {
  const paths = path?.split("/").slice(1);
  const chains = [[root, ""]] as Array<[string, string]>;

  if (!path) {
    return chains;
  }

  let pathsoFar = "/";
  for (const path of paths) {
    const decodedPath = decodeURIComponent(path);
    chains.push([decodedPath, pathsoFar + decodedPath]);
    pathsoFar = `${pathsoFar + decodedPath}/`;
  }
  return chains;
};

export const realPath = (parts: string[]) =>
  parts.length > 1
    ? parts.slice(1).reduce((acc: any, val: any) => `${acc}/${val}`, "")
    : "/";

export function getRawExtension(fileName: string | string[]) {
  return fileName.slice(((fileName.lastIndexOf(".") - 1) >>> 0) + 2);
}
export function getExtension(fileName: string) {
  return (getRawExtension(fileName) as string).toLowerCase();
}

export const zeroPad = (num: number | string, places: number) =>
  String(num).padStart(places, "0");

export const copyDataToClipboard = (data: string[]) => {
  return new Promise((resolve, reject) => {
    const textToCopy = data.join("\n");

    navigator.clipboard
      .writeText(textToCopy)
      .then(() => {
        resolve("copy success");
      })
      .catch((err) => {
        const errorMessage = `Unable to copy array to clipboard: ${err}`;
        console.error(errorMessage);
        reject(errorMessage);
      });
  });
};

export function formatDuration(value: number) {
  const minute = Math.floor(value / 60);
  const secondLeft = Math.floor(value - minute * 60);
  return `${minute}:${secondLeft < 10 ? `0${secondLeft}` : secondLeft}`;
}

export function formatTime(epochTime: number): string {
  const milliseconds = epochTime * 1000;

  const date = new Date(milliseconds);

  const formattedDate = date.toISOString();

  return formattedDate;
}

export function encode(str: string) {
  const charMap = {
    "!": "%21",
    "'": "%27",
    "(": "%28",
    ")": "%29",
    "~": "%7E",
    "%20": "+",
    "%00": "\x00",
  } as const;
  return encodeURIComponent(str).replace(
    /[!'()~]|%20|%00/g,
    function replacer(match) {
      return charMap[match as keyof typeof charMap];
    },
  );
}

export const mediaUrl = (
  id: string,
  name: string,
  path: string,
  sessionHash: string,
  download = false,
) => {
  const { settings } = useSettingsStore.getState();
  if (settings.rcloneProxy && path) {
    return `${settings.rcloneProxy}${path === "/" ? "" : path}/${encodeURIComponent(name)}`;
  }
  const url = new URL(window.location.origin);
  url.pathname = `/api/files/${id}/${name}`;
  url.searchParams.set("hash", sessionHash);
  if (download) {
    url.searchParams.set("download", "1");
  }
  return url.toString();
};

export const sharedMediaUrl = (
  shareId: string,
  fileId: string,
  name: string,
  download = false,
) => {
  const url = new URL(window.location.origin);
  url.pathname = `/api/shares/${shareId}/files/${fileId}/${name}`;
  if (download) {
    url.searchParams.set("download", "1");
  }
  return url.toString();
};

export function bytesToGB(bytes: number) {
  const gb = bytes / 1024 ** 3;
  return Math.round(gb * 10) / 10;
}

export const filesize = partial({ standard: "jedec" });

const isMobileDevice = () => {
  const toMatch = [
    /Android/i,
    /webOS/i,
    /iPhone/i,
    /iPad/i,
    /iPod/i,
    /BlackBerry/i,
    /Windows Phone/i,
  ];
  return toMatch.some((toMatchItem) => navigator.userAgent.match(toMatchItem));
};

export const isMobile = isMobileDevice();

export const chunkArray = <T>(arr: T[], size: number): T[][] => {
  return Array.from({ length: Math.ceil(arr.length / size) }, (_, index) =>
    arr.slice(index * size, index * size + size),
  );
};

export const preloadImage = (src: string) => {
  return new Promise<void>((resolve, reject) => {
    const image = new Image();
    image.onload = () => resolve();
    image.onerror = () => reject();
    image.src = src;
  });
};

export function getNextDate(): string {
  const today: Date = new Date();
  today.setDate(today.getDate() + 1);

  const year: number = today.getFullYear();
  const month: string = String(today.getMonth() + 1).padStart(2, "0");
  const day: string = String(today.getDate()).padStart(2, "0");

  return `${year}-${month}-${day}`;
}

export function getCountryCode(): string | null {
  const language: string =
    navigator.language || (navigator as any).userLanguage;
  if (language.includes("-")) {
    const parts: string[] = language.split("-");
    return parts[1] ? parts[1].toUpperCase() : "US";
  }

  return "US";
}

export function formatSpeed(bytesPerSecond: number): string {
  return `${filesize(bytesPerSecond)}/s`;
}

export function formatETA(seconds: number): string {
  if (seconds < 60) {
    return `${Math.round(seconds)}s left`;
  }
  if (seconds < 3600) {
    const minutes = Math.floor(seconds / 60);
    const secs = Math.round(seconds % 60);
    return secs > 0 ? `${minutes}m ${secs}s left` : `${minutes}m left`;
  }
  if (seconds < 86400) {
    const hours = Math.floor(seconds / 3600);
    const minutes = Math.floor((seconds % 3600) / 60);
    return minutes > 0 ? `${hours}h ${minutes}m left` : `${hours}h left`;
  }
  const days = Math.floor(seconds / 86400);
  const hours = Math.floor((seconds % 86400) / 3600);
  return hours > 0 ? `${days}d ${hours}h left` : `${days}d left`;
}
