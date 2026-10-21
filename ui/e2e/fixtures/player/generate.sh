#!/usr/bin/env bash
# Regenerates the fixtures for the player track tests. Needs ffmpeg (libvpx, libopus) and python3.
#
# multi-track.mkv: 12 s of VP9 test video with four audio tracks that are pure tones, so a test can
# tell which track is playing from its frequency, and two embedded text subtitle tracks with known
# cues:
#   audio 1  Opus   440 Hz  eng "English" (default)
#   audio 2  Opus   880 Hz  jpn "Japanese"
#   audio 3  AAC   1320 Hz  ger "Deutsch"
#   audio 4  E-AC-3 660 Hz  fre "Francais (E-AC-3)"   (no browser decoder; exercises the transcode path)
#   sub 1    SRT  eng "English"
#   sub 2    ASS  jpn "Japanese (ASS)"
# single-track.mkv: one Opus track (440 Hz) and the SRT subtitle. Plays natively.
# eac3-only.mkv: one E-AC-3 track (660 Hz), silent in the browser unless the player converts it.
# long-subs.mkv: ten minutes, video and one subtitle track only, one cue every 10 s
# ("Cue 0" at 0 s, "Cue 1" at 10 s, ...).
set -euo pipefail
cd "$(dirname "$0")"
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

VP9=(-c:v libvpx-vp9 -crf 50 -b:v 0 -g 20 -deadline realtime -cpu-used 8)

cat > "$work/en.srt" <<'EOF'
1
00:00:01,000 --> 00:00:03,000
<i>Hello</i> from the <b>first</b> subtitle

2
00:00:04,000 --> 00:00:06,500
Second line &amp; more
with a break

3
00:00:09,000 --> 00:00:11,000
Late line near the end
EOF

cat > "$work/ja.ass" <<'EOF'
[Script Info]
ScriptType: v4.00+
PlayResX: 384
PlayResY: 288

[V4+ Styles]
Format: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding
Style: Default,Arial,20,&H00FFFFFF,&H000000FF,&H00000000,&H00000000,0,0,0,0,100,100,0,0,1,2,2,2,10,10,10,1

[Events]
Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text
Dialogue: 0,0:00:01.50,0:00:03.50,Default,,0,0,0,,{\an8\i1}ASS first, with {\b1}styling{\b0} tags
Dialogue: 0,0:00:02.00,0:00:05.00,Default,,0,0,0,,Overlapping, second\Nline break
Dialogue: 0,0:00:06.00,0:00:08.00,Default,,0,0,0,,Comma, inside, the, text, field, yes
Dialogue: 0,0:00:07.00,0:00:07.50,Default,,0,0,0,,{\p1}m 0 0 l 100 0 100 100 0 100{\p0}
EOF

ffmpeg -loglevel error -y \
  -f lavfi -i "testsrc=size=96x64:rate=10:duration=12" \
  -f lavfi -i "sine=frequency=440:duration=12:sample_rate=48000" \
  -f lavfi -i "sine=frequency=880:duration=12:sample_rate=48000" \
  -f lavfi -i "sine=frequency=1320:duration=12:sample_rate=48000" \
  -f lavfi -i "sine=frequency=660:duration=12:sample_rate=48000" \
  -i "$work/en.srt" -i "$work/ja.ass" \
  -map 0:v -map 1:a -map 2:a -map 3:a -map 4:a -map 5 -map 6 \
  "${VP9[@]}" \
  -c:a:0 libopus -b:a:0 24k -c:a:1 libopus -b:a:1 24k -c:a:2 aac -b:a:2 32k \
  -c:a:3 eac3 -b:a:3 64k -ac:a:3 2 \
  -c:s:0 srt -c:s:1 ass \
  -metadata:s:a:0 language=eng -metadata:s:a:0 title="English" \
  -metadata:s:a:1 language=jpn -metadata:s:a:1 title="Japanese" \
  -metadata:s:a:2 language=ger -metadata:s:a:2 title="Deutsch" \
  -metadata:s:a:3 language=fre -metadata:s:a:3 title="Francais (E-AC-3)" \
  -metadata:s:s:0 language=eng -metadata:s:s:0 title="English" \
  -metadata:s:s:1 language=jpn -metadata:s:s:1 title="Japanese (ASS)" \
  -disposition:a:0 default -disposition:a:1 0 -disposition:s:0 0 \
  -max_interleave_delta 0 -cluster_time_limit 2000 multi-track.mkv

ffmpeg -loglevel error -y \
  -f lavfi -i "testsrc=size=96x64:rate=10:duration=8" \
  -f lavfi -i "sine=frequency=440:duration=8:sample_rate=48000" \
  -i "$work/en.srt" -map 0:v -map 1:a -map 2 \
  "${VP9[@]}" -c:a libopus -b:a 24k -c:s srt -metadata:s:s:0 language=eng \
  -max_interleave_delta 0 single-track.mkv

ffmpeg -loglevel error -y \
  -f lavfi -i "testsrc=size=96x64:rate=10:duration=8" \
  -f lavfi -i "sine=frequency=660:duration=8:sample_rate=48000" \
  -map 0:v -map 1:a "${VP9[@]}" -c:a eac3 -b:a 64k -ac 2 \
  eac3-only.mkv

python3 - "$work/long.srt" <<'PYTHON'
import sys

def stamp(sec):
    return "%02d:%02d:%02d,000" % (sec // 3600, sec // 60 % 60, sec % 60)

with open(sys.argv[1], "w") as out:
    for i in range(60):
        out.write("%d\n%s --> %s\nCue %d\n\n" % (i + 1, stamp(i * 10), stamp(i * 10 + 4), i))
PYTHON

ffmpeg -loglevel error -y \
  -f lavfi -i "color=c=0x204060:size=64x48:rate=1:duration=600" \
  -i "$work/long.srt" -map 0:v -map 1 \
  -c:v libvpx-vp9 -crf 55 -b:v 0 -g 10 -deadline realtime -cpu-used 8 \
  -c:s srt -metadata:s:s:0 language=eng \
  -max_interleave_delta 0 -cluster_time_limit 5000 long-subs.mkv
