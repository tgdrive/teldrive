import type { Tags } from "@/types";

import { decode, getBytes, sliceBytes, unpackBytes } from "./helpers";
import { parseVorbisComment } from "./vorbis-comment";

function mergeTypedArrays(a: Uint8Array, b: Uint8Array) {
  const c = new Uint8Array(a.length + b.length);

  c.set(a);
  c.set(b, a.length);
  return c;
}

// https://tools.ietf.org/html/rfc7845#section-5.1
// https://xiph.org/vorbis/doc/Vorbis_I_spec.html#x1-630004.2.2
function parseIdHeader(bytes: Uint8Array, tags: Tags) {
  tags.sampleRate = unpackBytes(sliceBytes(bytes, 12, 4), { endian: "little" });
  return tags;
}

function parseSegment(segment: Uint8Array, tags: Tags) {
  const type = decode(sliceBytes(segment, 0, 5));

  if (type === "OpusH" || type === "\x01vorb") {
    return parseIdHeader(segment, tags);
  }
  if (type === "OpusT") {
    return parseVorbisComment(segment, tags, 8);
  }
  if (type === "\x03vorb") {
    return parseVorbisComment(segment, tags, 7);
  }
  throw new Error("Unknown type");
}

// https://en.wikipedia.org/wiki/Ogg#Page_structure
//@ts-ignore
function parsePages(buffer: ArrayBuffer) {
  let tags: Tags = {};
  let offset = 0;
  let headersToFind = 2;
  let segment = new Uint8Array();

  while (offset < buffer.byteLength) {
    // Jump to header type
    offset += 5;
    const [headerType] = getBytes(buffer, offset, 1);
    offset += 1;

    // 4 = end of stream
    if (headerType === 4) {
      const samples = unpackBytes(getBytes(buffer, offset, 4), {
        endian: "little",
      });
      tags.duration = Math.floor(samples / (tags.sampleRate as number));

      return tags;
    }

    // Jump to segment count
    offset += 20;

    const [segmentCount] = getBytes(buffer, offset, 1);
    offset += 1;

    const segmentTable = getBytes(buffer, offset, segmentCount);
    let segmentLength = 0;
    offset += segmentCount;

    for (let i = 0; i < segmentCount; i++) {
      segmentLength += segmentTable[i];
    }

    if (headersToFind) {
      const finalSegment = segmentTable[segmentTable.length - 1];
      segment = mergeTypedArrays(segment, getBytes(buffer, offset, segmentLength));

      if (segmentLength % 255 !== 0 || !finalSegment) {
        headersToFind -= 1;
        tags = parseSegment(segment, tags);
        segment = new Uint8Array();
      }
    }
    offset += segmentLength;
  }
}

export default parsePages;
