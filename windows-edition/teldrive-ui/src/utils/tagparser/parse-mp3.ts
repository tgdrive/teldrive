import type { Tags } from "@/types";

import { decode, getBuffer, getBytes, unpackBytes } from "./helpers";

const sampleRatesTable = [[11025, 12000, 8000], null, [22050, 24000, 16000], [44100, 48000, 32000]];
const samplesPerFrameTable = [[384, 1152, 576], null, [384, 1152, 576], [384, 1152, 1152]];

// Bitrates
const version1layer1 = [0, 32, 64, 96, 128, 160, 192, 224, 256, 288, 320, 352, 384, 416, 448, 0];
const version1layer2 = [0, 32, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 384, 0];
const version1layer3 = [0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 0];
const version2layer1 = [0, 32, 48, 56, 64, 80, 96, 112, 128, 144, 160, 176, 192, 224, 256, 0];
const version2layer2 = [0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160, 0];
const version2layer3 = version2layer2;

const bitratesByVersionAndLayer = [
  [null, version2layer3, version2layer2, version2layer1],
  null,
  [null, version2layer3, version2layer2, version2layer1],
  [null, version1layer3, version1layer2, version1layer1],
];

// Used to get ID3 tag size and ID3v2.4 frame size
function getSize(buffer: ArrayBuffer, offset: number) {
  return unpackBytes(getBytes(buffer, offset, 4), {
    endian: "big",
    shiftBase: 7,
  });
}

function getFrameSize(buffer: ArrayBuffer, offset: number, version: number) {
  if (version === 3) {
    return unpackBytes(getBytes(buffer, offset, 4), { endian: "big" });
  }
  return getSize(buffer, offset);
}

// http://id3.org/id3v2.4.0-structure
function decodeFrame(buffer: ArrayBuffer, offset: number, size: number) {
  const bytes = getBytes(buffer, offset, size);
  const [firstByte] = bytes;

  if (firstByte === 0) {
    const string = decode(bytes, "iso-8859-1");

    return bytes[bytes.length - 1] === 0 ? string.slice(1, -1) : string.slice(1);
  }
  if (firstByte === 1) {
    const encoding = bytes[1] === 255 && bytes[2] === 254 ? "utf-16le" : "utf-16be";
    const stringBytes = bytes.length % 2 === 0 ? bytes.slice(3, -1) : bytes.slice(3);

    if (encoding === "utf-16be") {
      stringBytes[0] = 0;
    }
    const string = decode(stringBytes, encoding);

    return bytes[bytes.length - 1] === 0 && bytes[bytes.length - 2] === 0
      ? string.slice(0, -1)
      : string;
  }
  if (firstByte === 2) {
    const stringBytes = bytes.length % 2 === 0 ? bytes.slice(1, -1) : bytes.slice(1);

    return decode(stringBytes, "utf-16le");
  }
  if (firstByte === 3) {
    const string = decode(bytes, "utf-8");

    return bytes[bytes.length - 1] === 0 ? string.slice(1, -1) : string.slice(1);
  }
  return decode(bytes, "iso-8859-1");
}

function getFrameId(buffer: ArrayBuffer, offset: number) {
  const id = decode(getBytes(buffer, offset, 4));

  return /\w{4}/.test(id) ? id : null;
}

function getPictureDataLength(bytes: Uint8Array, offset: number) {
  let length = 0;

  while (bytes[offset]) {
    offset += 1;
    length += 1;
  }
  return length;
}

// http://id3.org/id3v2.4.0-frames
function getPicture(buffer: ArrayBuffer, offset: number, size: number) {
  let pictureOffset = 1;
  const bytes = getBytes(buffer, offset, size);
  const MIMETypeLength = getPictureDataLength(bytes, pictureOffset);
  const MIMETypeBytes = getBytes(buffer, offset + pictureOffset, MIMETypeLength);
  const MIMEType = decode(MIMETypeBytes);

  // Jump over MIME type, terminator and picture type
  pictureOffset += MIMETypeLength + 2;

  // Skip description and its terminator
  const length = getPictureDataLength(bytes, pictureOffset) + 1;
  pictureOffset += length;

  // Description may end in 2 null bytes
  if (bytes[pictureOffset + length + 1] === 0) {
    pictureOffset += 1;
  }
  return new Blob([bytes.slice(pictureOffset)], { type: MIMEType });
}

async function parseID3Tag(
  file: string,
  buffer: ArrayBuffer,
  version: number,
  offset = 0,
  tags: Tags = {},
) {
  const initialOffset = offset;

  // Skip identifier, version, flags
  offset += 6;

  // +10 to include header size
  const tagSize = getSize(buffer, offset) + 10;
  offset += 4;

  if (initialOffset + tagSize > buffer.byteLength) {
    buffer = await getBuffer(file, initialOffset + tagSize + buffer.byteLength);
  }

  while (true) {
    const id = getFrameId(buffer, offset);
    offset += 4;
    const frameSize = getFrameSize(buffer, offset, version);
    offset += 4;

    const [encodingFlagByte] = getBytes(buffer, offset + 1, 2);
    const usesCompression = (encodingFlagByte >> 1) % 2 !== 0;
    offset += 2;

    if (id) {
      const field = mapFrameIdToField(id);
      let frameOffset = offset;
      let size = frameSize;

      if (usesCompression) {
        size = getFrameSize(buffer, frameOffset, version);
        frameOffset += 4;
      }

      if (field && !tags[field]) {
        if (field === "picture") {
          tags[field] = getPicture(buffer, frameOffset, size);
        } else {
          tags[field] = decodeFrame(buffer, frameOffset, size);
        }
      }
    } else {
      offset = initialOffset + tagSize;

      if (decode(getBytes(buffer, offset, 3)) === "ID3") {
        return parseID3Tag(file, buffer, version, offset, tags);
      }
      break;
    }
    offset += frameSize;
  }

  // Skip padding
  while (new DataView(buffer, offset, 1).getUint8(0) === 0) {
    offset += 1;
  }
  let frameCount = 0;
  let isFirstAudioFrame = true;

  while (offset < buffer.byteLength) {
    const bytes = getBytes(buffer, offset, 4);

    if (bytes[0] !== 255 || bytes[1] < 112) {
      tags.duration = getDuration(frameCount, tags);
      return tags;
    }

    if (isFirstAudioFrame) {
      tags = parseAudioFrameHeader(bytes, tags);
      const frameHeaderSize = 36;
      const id = decode(getBytes(buffer, offset + frameHeaderSize, 4));

      if (id === "Xing" || id === "Info") {
        return parseXingHeader(buffer, offset + frameHeaderSize, tags);
      }
      buffer = await getBuffer(file);
      isFirstAudioFrame = false;
    }
    frameCount += 1;
    offset += getAudioFrameSize(bytes[2], tags);
  }
  tags.duration = getDuration(frameCount, tags);
  return tags;
}

function getAudioFrameSize(byte: number, { bitrate, sampleRate }: Tags) {
  const padding = (byte & 0x02) > 0 ? 1 : 0;

  return Math.floor((144000 * (bitrate as number)) / (sampleRate as number)) + padding;
}

// https://www.codeproject.com/Articles/8295/MPEG-Audio-Frame-Header#MPEGAudioFrameHeader
function parseAudioFrameHeader(bytes: Uint8Array, data: Tags) {
  const versionIndex = (bytes[1] >> 3) & 0x03;
  const layerIndex = (bytes[1] >> 1) & 0x03;
  const sampleRateIndex = (bytes[2] >> 2) & 0x03;
  const bitrateIndex = (bytes[2] >> 4) & 0x0f;

  data.sampleRate = sampleRatesTable[versionIndex]![sampleRateIndex];
  data.samplesPerFrame = samplesPerFrameTable[versionIndex]![layerIndex];
  data.bitrate = bitratesByVersionAndLayer[versionIndex]![layerIndex]![bitrateIndex];

  return data;
}

function getDuration(frameCount: number, { samplesPerFrame, sampleRate }: Tags) {
  return Math.floor((frameCount * (samplesPerFrame as number)) / (sampleRate as number));
}

function parseXingHeader(buffer: ArrayBuffer, offset: number, tags: Tags) {
  // +8 to jump to frame count bytes
  const frameCount = unpackBytes(getBytes(buffer, offset + 8, 4), {
    endian: "big",
  });
  tags.duration = getDuration(frameCount, tags);
  return tags;
}

function mapFrameIdToField(id: string) {
  const map = {
    TIT2: "title",
    TPE1: "artist",
    TALB: "album",
    APIC: "picture",
  };
  return map[id as keyof typeof map];
}

export default parseID3Tag;
