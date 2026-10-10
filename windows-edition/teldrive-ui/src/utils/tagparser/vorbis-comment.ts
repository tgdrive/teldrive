import type { Tags } from "@/types";

import { decode, sliceBytes, unpackBytes } from "./helpers";

function convertBase64ToUint8(data: string) {
  const raw = atob(data);
  const array = new Uint8Array(raw.length);

  for (let i = 0; i < raw.length; i++) {
    array[i] = raw.charCodeAt(i);
  }
  return array;
}

function unpackPictureBlockBytes(bytes: Uint8Array, offset: number) {
  return unpackBytes(sliceBytes(bytes, offset, 4), { endian: "big" });
}

// https://xiph.org/flac/format.html#metadata_block_picture
function parsePictureBlock(bytes: Uint8Array, tags: Tags) {
  // Start from 4th byte to skip picture type
  let offset = 4;

  const MIMETypeLength = unpackPictureBlockBytes(bytes, offset);
  offset += 4;

  const MIMEType = decode(sliceBytes(bytes, offset, MIMETypeLength));
  offset += MIMETypeLength;

  const descriptionLength = unpackPictureBlockBytes(bytes, offset);
  offset += 4;

  // Skip description
  offset += descriptionLength;

  // Skip picture width, height, color depth, number of colors used
  offset += 16;

  const pictureLength = unpackPictureBlockBytes(bytes, offset);
  offset += 4;

  tags.picture = new Blob([sliceBytes(bytes, offset, pictureLength)], {
    type: MIMEType,
  });
  return tags;
}

function unpackVorbisCommentBytes(bytes: Uint8Array, offset: number) {
  return unpackBytes(sliceBytes(bytes, offset, 4), { endian: "little" });
}

// https://xiph.org/flac/format.html#metadata_block_vorbis_comment
// https://tools.ietf.org/html/rfc7845#section-5.2
function parseVorbisComment(bytes: Uint8Array, tags: Tags, offset = 0) {
  const vendorStringLength = unpackVorbisCommentBytes(bytes, offset);
  offset += vendorStringLength + 4;
  let userCommentCount = unpackVorbisCommentBytes(bytes, offset);
  offset += 4;

  while (userCommentCount) {
    const userCommentLength = unpackVorbisCommentBytes(bytes, offset);
    offset += 4;

    const userComment = decode(sliceBytes(bytes, offset, userCommentLength), "utf-8");
    const [name, value] = userComment.split("=");

    if (name === "METADATA_BLOCK_PICTURE") {
      tags = parsePictureBlock(convertBase64ToUint8(value), tags);
    } else {
      tags[name.toLowerCase()] = value;
    }
    offset += userCommentLength;
    userCommentCount -= 1;
  }
  return tags;
}

export { parseVorbisComment, parsePictureBlock };
