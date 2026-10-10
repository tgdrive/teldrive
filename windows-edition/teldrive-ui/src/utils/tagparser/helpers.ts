import fetch from "@/utils/fetch-throw";

function getBytes(buffer: ArrayBuffer, offset: number, count: number) {
  return new Uint8Array(buffer, offset, count);
}

function sliceBytes(bytes: Uint8Array, offset: number, count: number) {
  return bytes.slice(offset, offset + count);
}

interface Options {
  endian: "big" | "little";
  shiftBase?: number;
  byteCount?: number;
}

function unpackBytes(bytes: Uint8Array, options: Partial<Options>) {
  if (options.endian === "little") {
    return bytes[0] | (bytes[1] << 8) | (bytes[2] << 16) | (bytes[3] << 24);
  }
  if (options.shiftBase === 7) {
    return (bytes[0] << 21) | (bytes[1] << 14) | (bytes[2] << 7) | bytes[3];
  }
  let value = (bytes[1] << 16) | (bytes[2] << 8) | bytes[3];

  if (options.byteCount === 4) {
    value = (bytes[0] << 24) | value;
  }
  return value;
}

function decode(bytes: Uint8Array, encoding = "utf-8") {
  const decoder = new TextDecoder(encoding);

  return decoder.decode(bytes);
}

async function getBuffer(url: string, size?: number, signal?: AbortSignal): Promise<ArrayBuffer> {
  const response = await fetch(url, {
    headers: { Range: `bytes=${0}-${size}` },
    signal,
  });
  return response.arrayBuffer();
}

export { getBytes, sliceBytes, unpackBytes, decode, getBuffer };
