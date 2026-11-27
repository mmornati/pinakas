/* page player for the intro film: the audio element is the clock, the canvas follows it */
(function () {
  'use strict';
  var box = document.querySelector('[data-film]');
  if (!box || !window.PinakasFilm) return;
  var canvas = box.querySelector('canvas');
  var ctx = canvas.getContext('2d');
  var audio = document.getElementById('film-audio');
  var playBtn = document.getElementById('film-play');
  var big = box.querySelector('.big-play');
  var seek = document.getElementById('film-seek');
  var time = document.getElementById('film-time');
  var ccBtn = document.getElementById('film-cc');
  var fsBtn = document.getElementById('film-fs');
  var chapters = document.getElementById('film-chapters');
  var tl = null, raf = 0, captions = true, posterT = 0;
  // fallback clock when audio cannot play (blocked, missing): the film still runs silently
  var silent = false, silentStart = 0, silentAt = 0, silentPlaying = false;

  var ICON_PLAY = '<svg viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M7 4.5v15l13-7.5z"/></svg>';
  var ICON_PAUSE = '<svg viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M6 4h4v16H6zM14 4h4v16h-4z"/></svg>';

  function now() {
    if (!silent) return audio.currentTime;
    return silentPlaying ? silentAt + (performance.now() - silentStart) / 1000 : silentAt;
  }
  function playing() { return silent ? silentPlaying : !audio.paused && !audio.ended; }

  function size() {
    var dpr = Math.min(window.devicePixelRatio || 1, 2);
    var w = Math.round(box.clientWidth * dpr), h = Math.round(w * 9 / 16);
    if (canvas.width !== w || canvas.height !== h) { canvas.width = w; canvas.height = h; }
  }

  function fmt(s) { s = Math.max(0, Math.floor(s)); return Math.floor(s / 60) + ':' + ('0' + (s % 60)).slice(-2); }

  function frame() {
    if (!tl) return;
    var t = box.classList.contains('started') ? Math.min(now(), tl.duration) : posterT;
    PinakasFilm.draw(ctx, t, tl, { captions: captions && box.classList.contains('started'), progress: false });
    var shown = box.classList.contains('started') ? t : 0;
    seek.value = String(shown / tl.duration * 1000);
    time.textContent = fmt(shown) + ' / ' + fmt(tl.duration);
    var cur = PinakasFilm.sceneAt(tl, t);
    if (chapters) chapters.querySelectorAll('button').forEach(function (b) { b.classList.toggle('on', box.classList.contains('started') && +b.dataset.i === cur); });
    if (silent && silentPlaying && t >= tl.duration) { silentPlaying = false; silentAt = tl.duration; sync(); }
  }

  function loop() { frame(); if (playing()) raf = requestAnimationFrame(loop); }

  function sync() {
    var p = playing();
    box.classList.toggle('playing', p);
    playBtn.innerHTML = p ? ICON_PAUSE : ICON_PLAY;
    playBtn.setAttribute('aria-label', p ? 'Pause' : 'Play');
    cancelAnimationFrame(raf);
    if (p) raf = requestAnimationFrame(loop); else frame();
  }

  function play() {
    box.classList.add('started');
    if (now() >= tl.duration - 0.05) setTime(0);
    if (silent) { silentStart = performance.now(); silentPlaying = true; sync(); return; }
    var p = audio.play();
    if (p && p.catch) p.catch(function () { silent = true; silentAt = audio.currentTime || 0; play(); });
  }
  function pause() {
    if (silent) { silentAt = now(); silentPlaying = false; sync(); return; }
    audio.pause();
  }
  function toggle() { playing() ? pause() : play(); }
  function setTime(t) {
    t = Math.max(0, Math.min(t, tl.duration));
    if (silent) { silentAt = t; silentStart = performance.now(); } else { audio.currentTime = t; }
    frame();
  }

  ['play', 'pause', 'ended', 'seeked'].forEach(function (ev) { audio.addEventListener(ev, sync); });
  audio.addEventListener('error', function () { silent = true; });

  big.addEventListener('click', play);
  playBtn.addEventListener('click', toggle);
  canvas.addEventListener('click', function () { if (box.classList.contains('started')) toggle(); });
  seek.addEventListener('input', function () { box.classList.add('started'); setTime(seek.value / 1000 * tl.duration); });
  ccBtn.addEventListener('click', function () {
    captions = !captions; ccBtn.setAttribute('aria-pressed', String(captions)); frame();
  });
  fsBtn.addEventListener('click', function () {
    var el = box;
    if (document.fullscreenElement) document.exitFullscreen();
    else if (el.requestFullscreen) el.requestFullscreen();
  });
  box.addEventListener('keydown', function (e) {
    if (e.key === ' ' || e.key === 'k') { e.preventDefault(); toggle(); }
    if (e.key === 'ArrowRight') setTime(now() + 5);
    if (e.key === 'ArrowLeft') setTime(now() - 5);
  });
  window.addEventListener('resize', function () { size(); frame(); });

  var titles = { fast: 'Promise', drift: 'Problem', enforce: 'Skills', meet: 'pinakas', claim: 'Claim', hooks: 'Hooks', actuals: 'Actuals', action: 'Action', check: 'Check', failopen: 'Fail-open', tools: 'Tools', cta: 'Get it' };

  fetch(box.getAttribute('data-timeline'))
    .then(function (r) { return r.json(); })
    .then(function (data) {
      tl = data;
      if (chapters) {
        tl.scenes.forEach(function (s, i) {
          var b = document.createElement('button');
          b.type = 'button'; b.dataset.i = i;
          b.textContent = fmt(s.start) + ' ' + (titles[s.id] || s.id);
          b.addEventListener('click', function () { box.classList.add('started'); setTime(Math.max(0, s.start - 0.2)); if (!playing()) play(); });
          chapters.appendChild(b);
        });
      }
      // transcript for screen readers and search engines
      var tr = document.getElementById('film-transcript');
      if (tr) tr.textContent = tl.scenes.map(function (s) { return s.text; }).join(' ');
      return PinakasFilm.fontsReady();
    })
    .then(function () {
      size();
      // poster: the wordmark frame
      var poster = tl.scenes.filter(function (s) { return s.id === 'meet'; })[0];
      posterT = poster ? poster.start + 4.6 : 0;
      frame();
      seek.value = '0'; time.textContent = '0:00 / ' + fmt(tl.duration);
    });
})();
