import { readFileSync } from "node:fs";
import { createServer } from "node:http";
import type { MatroskaFile } from "../../../src/features/player/matroska";
import {
  blockToCue,
  subtitleFormat,
  type TextCue,
} from "../../../src/features/player/subtitle-text";

export const fixture = (name: string) =>
  readFileSync(new URL(`./${name}`, import.meta.url));

export async function serve(data: Buffer, options: { ranges?: boolean } = {}) {
  const requests: { start: number; end: number }[] = [];
  const server = createServer((request, response) => {
    const match = /^bytes=(\d+)-(\d*)$/.exec(request.headers.range ?? "");
    if (!match || options.ranges === false) {
      response.writeHead(200, { "Content-Length": data.length });
      response.end(data);
      return;
    }
    const start = Number(match[1]);
    const end = Math.min(match[2] ? Number(match[2]) : data.length - 1, data.length - 1);
    if (start >= data.length) {
      response.writeHead(416, { "Content-Range": `bytes */${data.length}` });
      response.end();
      return;
    }
    requests.push({ start, end });
    response.writeHead(206, {
      "Content-Range": `bytes ${start}-${end}/${data.length}`,
      "Content-Length": end - start + 1,
    });
    response.end(data.subarray(start, end + 1));
  });
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  const address = server.address();
  if (!address || typeof address === "string") throw new Error("no address");
  return {
    url: `http://127.0.0.1:${address.port}/video.mkv`,
    requests,
    bytesRequested: () => requests.reduce((sum, r) => sum + (r.end - r.start + 1), 0),
    close: () =>
      new Promise<void>((resolve) => {
        server.close(() => resolve());
        server.closeAllConnections();
      }),
  };
}

export async function readTrack(
  mkv: MatroskaFile,
  number: number,
  until = Number.POSITIVE_INFINITY,
) {
  const track = mkv.info.tracks.find((candidate) => candidate.number === number);
  const format = track && subtitleFormat(track);
  if (!format) throw new Error(`track ${number} is not a text subtitle`);
  const cues: TextCue[] = [];
  const offset = await mkv.clusterOffsetFor(0);
  await mkv.scan(offset ?? 0, new Set([number]), {
    onCluster: (time) => time <= until,
    onBlock: (block) => {
      const cue = blockToCue(block, format);
      if (cue) cues.push(cue);
    },
  });
  return cues;
}

export const round = (cues: TextCue[]) =>
  cues.map((cue) => ({
    ...cue,
    start: Math.round(cue.start * 1000) / 1000,
    end: Math.round(cue.end * 1000) / 1000,
  }));
