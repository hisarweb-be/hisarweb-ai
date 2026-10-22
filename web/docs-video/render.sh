#!/usr/bin/env bash
# Rebuilds hisarweb-ai-rehber.mp4 and .jpg from the sources in this folder.
# Needs: Node >= 22, FFmpeg on PATH, internet on the first run (HyperFrames downloads Chrome and fonts once).
set -euo pipefail
cd "$(dirname "$0")"
export HYPERFRAMES_NO_TELEMETRY=1 DO_NOT_TRACK=1 HYPERFRAMES_SKIP_SKILLS=1
DOCS="../public/rehber"   # the site serves these as /rehber/*
for f in adim-1-kayit adim-2-startbonus adim-2-anahtar-olustur adim-3-anahtar-listesi adim-4-playground; do
  cp "$DOCS/$f.png" "assets/$f.png"      # the screenshots are not duplicated in git
done
mkdir -p out
npx --yes hyperframes@0.8.185 render -f 30 -q looks -w 4 -o out/raw.mp4
ffmpeg -y -v error -i out/raw.mp4 -c:v libx264 -preset slow -crf 24 -pix_fmt yuv420p -movflags +faststart -an -r 30 "$DOCS/hisarweb-ai-rehber.mp4"
ffmpeg -y -v error -ss 4 -i "$DOCS/hisarweb-ai-rehber.mp4" -frames:v 1 -q:v 4 "$DOCS/hisarweb-ai-rehber.jpg"
rm -rf out assets/adim-*.png
