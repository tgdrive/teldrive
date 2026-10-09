import { decode, getBuffer, getBytes } from "./helpers";
import parseFlacFile from "./parse-flac";
import parseM4aFile from "./parse-m4a";
import parseMp3File from "./parse-mp3";
import parseOggOpusFile from "./parse-ogg-opus";
import parseWavFile from "./parse-wav";

// http://id3lib.sourceforge.net/id3/id3v2com-00.html
function getID3TagSize(buffer: ArrayBuffer) {
  const bytes = getBytes(buffer, 6, 4);
  return bytes[0] * 2097152 + bytes[1] * 16384 + bytes[2] * 128 + bytes[3];
}

async function parseFile(file: string, buffer: ArrayBuffer) {
  const bytes = getBytes(buffer, 0, 8);
  const string = decode(bytes);

  if (string.startsWith("ID3")) {
    if (bytes[3] < 3) {
      throw new Error("Unsupported ID3 tag version");
    }
    // +10 to skip tag header
    const size = getID3TagSize(buffer) + 10;
    const string = decode(getBytes(buffer, size, 4));

    // Edge case when there is ID3 tag embedded in .flac file.
    // Instead of parsing ID3 tag - ignore it and treat it as normal .flac file.
    if (string === "fLaC") {
      return parseFlacFile(file, buffer, size + 4);
    }
    return parseMp3File(file, buffer, bytes[3]);
  }
  if (string.startsWith("fLaC")) {
    return parseFlacFile(file, buffer);
  }
  if (string.startsWith("OggS")) {
    buffer = await getBuffer(file);
    return parseOggOpusFile(buffer);
  }
  if (string.endsWith("ftyp")) {
    buffer = await getBuffer(file);
    return parseM4aFile(buffer);
  }
  if (string.startsWith("RIFF")) {
    return parseWavFile(buffer);
  }
  throw new Error("Invalid or unsupported file");
}

export async function parseAudioMetadata(url: string, signal?: AbortSignal) {
  const buffer = await getBuffer(url, 512 * 1024 - 1, signal);

  return parseFile(url, buffer);
}
