# pinakas website

Static site published to GitHub Pages by `.github/workflows/pages.yml` on every push to `main` that touches `site/`. No build step.

- `index.html`: landing page. `docs/index.html`: documentation.
- `assets/film.js`: the intro film, a pure function of time drawn on a canvas. `assets/player.js` drives it from the voice-over audio.
- `assets/voiceover.mp3` + `assets/timeline.json`: English voice-over generated with the Kokoro TTS model (voice `af_heart`), one clip per scene; the timeline holds each scene's start and end.
- `assets/pinakas-intro.mp4` and `assets/og.png`: rendered from the same `film.js` with Playwright and ffmpeg (30 fps, captions burned in).

Preview locally with any static server, for example `python3 -m http.server -d site`.
