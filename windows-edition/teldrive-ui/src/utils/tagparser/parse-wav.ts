import { decode, getBytes, unpackBytes } from "./helpers";

// http://www-mmsp.ece.mcgill.ca/Documents/AudioFormats/WAVE/WAVE.html

async function parseWavFile(buffer: ArrayBuffer, offset = 4) {
  // Jump to "fmt " chunk size
  offset += 12;

  const chunkSizeBytes = getBytes(buffer, offset, 4);
  const chunkSize = unpackBytes(chunkSizeBytes, { endian: "little" });

  offset += 4;

  const { sampleRate, dataRate } = getFmtChunkData(buffer, offset);

  offset += chunkSize;
  offset = findDataChunk(buffer, offset);

  // Skip data chunkId
  offset += 4;

  const samplesBytes = getBytes(buffer, offset, 4);
  const samples = unpackBytes(samplesBytes, { endian: "little" });

  return {
    sampleRate,
    duration: Math.floor(samples / dataRate),
  };
}

function getFmtChunkData(buffer: ArrayBuffer, offset: number) {
  offset += 4;

  const sampleRateBytes = getBytes(buffer, offset, 4);
  const sampleRate = unpackBytes(sampleRateBytes, { endian: "little" });

  offset += 4;

  const dataRateBytes = getBytes(buffer, offset, 4);
  const dataRate = unpackBytes(dataRateBytes, { endian: "little" });

  return {
    sampleRate,
    dataRate,
  };
}

function findDataChunk(buffer: ArrayBuffer, offset: number) {
  while (offset < buffer.byteLength) {
    const bytes = getBytes(buffer, offset, 4);
    const chunkId = decode(bytes);

    if (chunkId === "data") {
      return offset;
    }
    // Skip chunkId
    offset += 4;
    const chunkSizeBytes = getBytes(buffer, offset, 4);
    let chunkSize = unpackBytes(chunkSizeBytes, { endian: "little" });

    // Add pad byte if chunkSize is odd
    if (chunkSize % 2 === 1) {
      chunkSize += 1;
    }
    // Jump to the next chunk
    offset += 4 + chunkSize;
  }
  return offset;
}

export default parseWavFile;
