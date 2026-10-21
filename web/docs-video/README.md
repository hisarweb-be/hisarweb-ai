# HisarWeb AI uitlegvideo (hisarweb-ai-rehber)

Bron van `web/public/docs/hisarweb-ai-rehber.mp4` en `.jpg`: 99 s, 1280x720, 30 fps, zonder audio, Turkse ekranteksten.
Gebouwd met [HyperFrames](https://github.com/heygen-com/hyperframes) (Apache-2.0, HTML naar video, pin 0.8.185) en GSAP 3.14.2 (`assets/gsap.min.js`).

Scenes (seconden): titel 0-8, hesap 8-21, startbonus 21-34, API-anahtarı 34-48, bakiye 48-61, Playground 61-74, Claude Code 74-89, slot 89-99.
Schermafbeeldingen komen uit `web/public/docs/adim-*.png`; er staat nooit een echte sleutel in beeld (alleen de gemaskeerde lijst en de plaatshouder BURAYA_ANAHTARINIZ).

## Opnieuw renderen

Vereisten: Node 22+, FFmpeg in het pad. Eerste run haalt Chrome en lettertypen op.

    bash web/docs-video/render.sh

Het script kopieert de vijf PNG's naar `assets/`, rendert (`hyperframes render -f 30 -q looks`), codeert met `libx264 -crf 24 -pix_fmt yuv420p -movflags +faststart` (ca. 3,4 MB) en zet de poster (frame op 4 s) ernaast.
Controle zonder render: `npx hyperframes@0.8.185 check` in deze map.
