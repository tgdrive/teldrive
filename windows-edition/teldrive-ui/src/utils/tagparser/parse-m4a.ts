import type { Tags } from "@/types";

import { decode, getBytes, unpackBytes } from "./helpers";

function getAtomSize(buffer: ArrayBuffer, offset: number) {
  return unpackBytes(getBytes(buffer, offset, 4), {
    endian: "big",
    byteCount: 4,
  });
}

// https://developer.apple.com/library/archive/documentation/QuickTime/QTFF/QTFFChap2/qtff2.html#//apple_ref/doc/uid/TP40000939-CH204-56313
function parseMovieHeaderAtom(buffer: ArrayBuffer, offset: number) {
  const version = new DataView(buffer, offset, 1).getUint8(0);
  let timeUnitPerSecond = 0;
  let durationInTimeUnits = 0;

  // Jump over version and skip flags
  offset += 4;

  if (version === 0) {
    // Skip creation and modification dates
    offset += 8;
    timeUnitPerSecond = getAtomSize(buffer, offset);
    offset += 4;
    durationInTimeUnits = getAtomSize(buffer, offset);
  } else {
    // Skip creation and modification dates
    offset += 16;
    timeUnitPerSecond = getAtomSize(buffer, offset);
    offset += 4;
    durationInTimeUnits = getAtomSize(buffer, offset + 4);
  }
  return Math.floor(durationInTimeUnits / timeUnitPerSecond);
}

function getMIMEType(bytes: Uint8Array) {
  if (bytes[0] === 255 && bytes[1] === 216) {
    return "image/jpg";
  }
  if (decode(bytes.slice(0, 4)) === "\x89PNG") {
    return "image/png";
  }
  return "";
}

function parseMetadataItemListAtom(
  buffer: ArrayBuffer,
  offset: number,
  atomSize: number,
  tags: Tags,
) {
  const atomTypeToField = {
    "\xA9ART": "artist",
    "\xA9nam": "title",
    "\xA9alb": "album",
    "\xA9cmt": "comment",
    "\xA9day": "year",
    "\xA9too": "encoding",
    covr: "picture",
  };

  while (atomSize) {
    const size = getAtomSize(buffer, offset);
    const type = decode(getBytes(buffer, offset + 4, 4), "iso-8859-1");
    const field = atomTypeToField[type as keyof typeof atomTypeToField];

    // Jump size length, atom type and skip flags and reserved bytes
    const headerSize = 24;

    if (field && size > headerSize) {
      const dataSize = size - headerSize;
      const dataBytes = getBytes(buffer, offset + headerSize, dataSize);

      if (field === "picture") {
        tags[field] = new Blob([dataBytes], { type: getMIMEType(dataBytes) });
      } else {
        tags[field] = decode(dataBytes, "utf-8");
      }
    }
    offset += size;
    atomSize -= size;
  }
  return tags;
}

// http://xhelmboyx.tripod.com/formats/mp4-layout.txt
// https://developer.apple.com/library/archive/documentation/QuickTime/QTFF/Metadata/Metadata.html
function traverseAtoms(buffer: ArrayBuffer) {
  const atoms = ["moov", "mvhd", "udta", "meta", "ilst"];
  let tags: Tags = {};
  let offset = 0;

  while (atoms.length && offset < buffer.byteLength) {
    const size = getAtomSize(buffer, offset);
    const type = decode(getBytes(buffer, offset + 4, 4));

    // If atom is found move inside it
    if (atoms[0] === type) {
      offset += 8;
      atoms.shift();

      if (type === "mvhd") {
        tags.duration = parseMovieHeaderAtom(buffer, offset);
        offset += size - 8;
      } else if (type === "ilst") {
        tags = parseMetadataItemListAtom(buffer, offset, size - 8, tags);
      } else if (type === "meta") {
        // Meta atom has extra 4 byte header
        offset += 4;
      }
    } else {
      offset += size;
    }
  }
  return tags;
}

export default traverseAtoms;
