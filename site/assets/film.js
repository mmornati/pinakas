/* pinakas intro film: a pure function of time drawn on a 1920x1080 canvas.
 * PinakasFilm.draw(ctx, t, timeline, {captions, progress}) renders one frame,
 * so the page player and the MP4 renderer share exactly the same pictures. */
(function (global) {
  'use strict';

  var W = 1920, H = 1080;
  var C = {
    bg: '#13110f', bg2: '#1b1815', panel: '#211d1a', line: '#3a332d',
    paper: '#f1ebdf', paper2: '#e3dacb', ink: '#1a1714', ink2: '#5a5249',
    text: '#f1ebdf', dim: '#a39a8c', mute: '#6f675c',
    terra: '#e2643a', olive: '#9db06a', ochre: '#e0b54e', red: '#e5484d'
  };
  var F = {
    serif: '"Instrument Serif", Georgia, serif',
    sans: 'Geist, "Helvetica Neue", Arial, sans-serif',
    mono: '"Geist Mono", ui-monospace, Menlo, monospace'
  };
  var COLS = ['Ready', 'In Progress', 'In Review', 'Done'];

  var clamp = function (v, a, b) { a = a === undefined ? 0 : a; b = b === undefined ? 1 : b; return Math.max(a, Math.min(b, v)); };
  var lerp = function (a, b, t) { return a + (b - a) * t; };
  var eo = function (t) { t = clamp(t); return 1 - Math.pow(1 - t, 3); };
  var eio = function (t) { t = clamp(t); return t < 0.5 ? 4 * t * t * t : 1 - Math.pow(-2 * t + 2, 3) / 2; };
  var back = function (t) { t = clamp(t); var c1 = 1.70158, c3 = c1 + 1; return 1 + c3 * Math.pow(t - 1, 3) + c1 * Math.pow(t - 1, 2); };
  var seg = function (t, a, b) { return clamp((t - a) / (b - a)); };

  function rr(ctx, x, y, w, h, r) {
    ctx.beginPath();
    if (ctx.roundRect) { ctx.roundRect(x, y, w, h, r); return; }
    ctx.moveTo(x + r, y); ctx.arcTo(x + w, y, x + w, y + h, r); ctx.arcTo(x + w, y + h, x, y + h, r);
    ctx.arcTo(x, y + h, x, y, r); ctx.arcTo(x, y, x + w, y, r); ctx.closePath();
  }

  function txt(ctx, s, x, y, o) {
    o = o || {};
    ctx.save();
    ctx.globalAlpha *= o.alpha === undefined ? 1 : o.alpha;
    ctx.font = (o.style || '') + ' ' + (o.weight || 400) + ' ' + (o.size || 32) + 'px ' + (o.font || F.sans);
    ctx.fillStyle = o.color || C.text;
    ctx.textAlign = o.align || 'left';
    ctx.textBaseline = o.base || 'alphabetic';
    if ('letterSpacing' in ctx) ctx.letterSpacing = (o.track || 0) + 'px';
    ctx.fillText(s, x, y);
    var m = ctx.measureText(s).width;
    ctx.restore();
    return m;
  }

  function measure(ctx, s, o) {
    ctx.save();
    ctx.font = (o.style || '') + ' ' + (o.weight || 400) + ' ' + (o.size || 32) + 'px ' + (o.font || F.sans);
    if ('letterSpacing' in ctx) ctx.letterSpacing = (o.track || 0) + 'px';
    var m = ctx.measureText(s).width;
    ctx.restore();
    return m;
  }

  function kicker(ctx, s, a) {
    if (!s) return;
    ctx.save();
    ctx.globalAlpha *= a;
    ctx.fillStyle = C.terra;
    ctx.fillRect(120, 108, 28, 3);
    txt(ctx, s, 164, 118, { font: F.mono, size: 22, color: C.dim, track: 4, weight: 500 });
    ctx.restore();
  }

  function grain(ctx) {
    // a few fixed hairlines give the dark ground some texture without noise
    ctx.save();
    ctx.strokeStyle = 'rgba(241,235,223,0.035)';
    ctx.lineWidth = 1;
    for (var x = 120; x < W; x += 120) { ctx.beginPath(); ctx.moveTo(x + 0.5, 0); ctx.lineTo(x + 0.5, H); ctx.stroke(); }
    ctx.restore();
  }

  /* ---------- board ---------- */

  function boardGeom(x, y, w, rows, cardH) {
    var gap = 18, colW = (w - gap * 3) / 4;
    return { x: x, y: y, w: w, gap: gap, colW: colW, cardH: cardH || 74, rowGap: 14, head: 64, rows: rows };
  }
  function colX(g, col) { return g.x + col * (g.colW + g.gap); }
  function boardH(g) { return g.head + g.rows * (g.cardH + g.rowGap) + 18; }

  function drawBoard(ctx, g, a, counts) {
    ctx.save();
    ctx.globalAlpha *= a;
    var h = boardH(g);
    for (var i = 0; i < 4; i++) {
      var x = colX(g, i);
      rr(ctx, x, g.y, g.colW, h, 14);
      ctx.fillStyle = C.bg2; ctx.fill();
      ctx.strokeStyle = C.line; ctx.lineWidth = 1.5; ctx.stroke();
      var dot = [C.dim, C.terra, C.ochre, C.olive][i];
      ctx.fillStyle = dot; ctx.beginPath(); ctx.arc(x + 26, g.y + 33, 6, 0, Math.PI * 2); ctx.fill();
      txt(ctx, COLS[i], x + 42, g.y + 41, { size: 23, weight: 600, color: C.text });
      if (counts) txt(ctx, String(counts[i]), x + g.colW - 22, g.y + 41, { size: 21, font: F.mono, color: C.mute, align: 'right' });
    }
    ctx.restore();
  }

  function cardPos(g, col, row) {
    return { x: colX(g, col) + 14, y: g.y + g.head + row * (g.cardH + g.rowGap), w: g.colW - 28, h: g.cardH };
  }

  function drawCard(ctx, g, c, a) {
    var p = cardPos(g, c.col, c.row);
    ctx.save();
    ctx.globalAlpha *= a * (c.alpha === undefined ? 1 : c.alpha);
    ctx.translate(p.x + p.w / 2 + (c.dx || 0), p.y + p.h / 2 + (c.dy || 0));
    ctx.rotate(c.rot || 0);
    var s = c.scale || 1; ctx.scale(s, s);
    var x = -p.w / 2, y = -p.h / 2;
    if (c.lift) {
      ctx.shadowColor = 'rgba(226,100,58,' + (0.45 * c.lift) + ')';
      ctx.shadowBlur = 30 * c.lift; ctx.shadowOffsetY = 8 * c.lift;
    }
    rr(ctx, x, y, p.w, p.h, 10);
    if (c.ghost) {
      ctx.setLineDash([10, 8]); ctx.strokeStyle = C.red; ctx.lineWidth = 2.5; ctx.stroke(); ctx.setLineDash([]);
      ctx.fillStyle = 'rgba(229,72,77,0.08)'; ctx.fill();
    } else {
      ctx.fillStyle = C.paper; ctx.fill();
    }
    ctx.shadowColor = 'transparent';
    if (c.bad) { rr(ctx, x - 4, y - 4, p.w + 8, p.h + 8, 13); ctx.strokeStyle = C.red; ctx.lineWidth = 3 * c.bad; ctx.stroke(); }
    if (c.good) { rr(ctx, x - 4, y - 4, p.w + 8, p.h + 8, 13); ctx.strokeStyle = 'rgba(157,176,106,' + c.good + ')'; ctx.lineWidth = 3; ctx.stroke(); }
    var ink = c.ghost ? C.red : C.ink;
    txt(ctx, c.id, x + 16, y + 30, { font: F.mono, size: 19, color: c.ghost ? C.red : C.ink2, weight: 500 });
    txt(ctx, c.title, x + 16, y + 59, { size: 22, weight: 600, color: ink });
    if (c.who) {
      var wa = c.whoA === undefined ? 1 : c.whoA;
      ctx.save(); ctx.globalAlpha *= wa;
      var cw = measure(ctx, c.who, { font: F.mono, size: 16 }) + 22;
      rr(ctx, x + p.w - cw - 12, y + 12, cw, 26, 13);
      ctx.fillStyle = c.whoColor || C.terra; ctx.fill();
      txt(ctx, c.who, x + p.w - cw / 2 - 12, y + 30, { font: F.mono, size: 16, color: '#fff', align: 'center', weight: 600 });
      ctx.restore();
    }
    if (c.clock !== undefined) {
      txt(ctx, c.clock, x + p.w - 14, y + 59, { font: F.mono, size: 18, color: C.terra, align: 'right', weight: 600 });
    }
    ctx.restore();
  }

  /* ---------- terminal ---------- */

  function drawTerm(ctx, x, y, w, h, lines, a, title) {
    ctx.save();
    ctx.globalAlpha *= a;
    rr(ctx, x, y, w, h, 16);
    ctx.fillStyle = '#0c0b0a'; ctx.fill();
    ctx.strokeStyle = C.line; ctx.lineWidth = 1.5; ctx.stroke();
    ['#5a524a', '#5a524a', '#5a524a'].forEach(function (col, i) {
      ctx.fillStyle = col; ctx.beginPath(); ctx.arc(x + 30 + i * 24, y + 30, 7, 0, Math.PI * 2); ctx.fill();
    });
    txt(ctx, title || 'agent · worktree/oauth-login', x + w / 2, y + 37, { font: F.mono, size: 18, color: C.mute, align: 'center' });
    ctx.fillStyle = C.line; ctx.fillRect(x, y + 60, w, 1.5);
    var ly = y + 112;
    lines.forEach(function (l) {
      if (!l || l.alpha === 0) { ly += 44; return; }
      var lx = x + 34;
      if (l.prompt) { lx += txt(ctx, '$ ', lx, ly, { font: F.mono, size: 27, color: C.terra, alpha: l.alpha }); }
      var segs = l.segs || [{ s: l.s, c: l.c }];
      segs.forEach(function (sg) {
        lx += txt(ctx, sg.s, lx, ly, { font: F.mono, size: l.size || 27, color: sg.c || C.text, alpha: l.alpha === undefined ? 1 : l.alpha, weight: sg.w || 400 });
      });
      if (l.cursor) {
        if (Math.floor(l.cursor * 2) % 2 === 0) { ctx.fillStyle = C.text; ctx.fillRect(lx + 4, ly - 24, 14, 30); }
      }
      ly += l.gap || 44;
    });
    ctx.restore();
  }

  function typed(s, p) { return s.slice(0, Math.round(s.length * clamp(p))); }

  function wrapLines(ctx, s, maxW, o) {
    var words = s.split(' '), out = [], cur = '';
    words.forEach(function (wd) {
      var t = cur ? cur + ' ' + wd : wd;
      if (measure(ctx, t, o) > maxW && cur) { out.push(cur); cur = wd; } else cur = t;
    });
    if (cur) out.push(cur);
    return out;
  }

  /* ---------- scenes ---------- */
  // each scene: function (ctx, lt, d, a) where lt is local time, d its duration and a its fade-in

  var S = {};

  var fastCards = [
    { id: '#31', title: 'Rate limit API' },
    { id: '#34', title: 'CSV export' },
    { id: '#36', title: 'Dark mode' },
    { id: '#39', title: 'Audit log' },
    { id: '#41', title: 'Search filters' }
  ];

  S.fast = function (ctx, lt, d, a) {
    kicker(ctx, 'THE PROMISE', a);
    var g = boardGeom(120, 430, 1680, 5, 66);
    g.rowGap = 12;
    var done = 0;
    var cols = fastCards.map(function (c, i) {
      var p = eio(seg(lt, 1.0 + i * 0.75, 1.0 + i * 0.75 + 2.4));
      if (p >= 1) done++;
      return p * 3;
    });
    drawBoard(ctx, g, a * eo(seg(lt, 0, 0.8)), [5 - cols.filter(function (c) { return c > 0.2; }).length, cols.filter(function (c) { return c > 0.5 && c < 1.5; }).length, cols.filter(function (c) { return c >= 1.5 && c < 2.5; }).length, cols.filter(function (c) { return c >= 2.5; }).length]);
    fastCards.forEach(function (c, i) {
      var col = cols[i];
      var moving = col > 0.02 && col < 2.98;
      drawCard(ctx, g, { id: c.id, title: c.title, col: col, row: i, who: col > 0.3 ? 'agent' : '', lift: moving ? 1 : 0, good: col >= 2.98 ? 0.9 : 0 }, a * eo(seg(lt, 0.2 + i * 0.08, 0.9 + i * 0.08)));
    });
    var hA = a * eo(seg(lt, 0.1, 1.0));
    txt(ctx, 'Agents ship stories', 120, 260, { font: F.serif, size: 108, alpha: hA });
    var k = eo(seg(lt, 3.4, 4.6));
    var w = txt(ctx, '3–6×', 120, 380, { font: F.serif, size: 132, style: 'italic', color: C.terra, alpha: a * k });
    txt(ctx, 'faster than estimated', 120 + w + 34, 370, { font: F.serif, size: 84, alpha: a * k * eo(seg(lt, 4.0, 5.0)) });
  };

  S.drift = function (ctx, lt, d, a) {
    kicker(ctx, 'THE PROBLEM', a);
    var g = boardGeom(120, 400, 1080, 4, 74);
    var wob = eo(seg(lt, 0.4, 3));
    drawBoard(ctx, g, a);
    var b1 = eo(seg(lt, d * 0.2, d * 0.2 + 0.6));
    var b2 = eo(seg(lt, d * 0.42, d * 0.42 + 0.6));
    var b3 = eo(seg(lt, d * 0.7, d * 0.7 + 0.6));
    var cards = [
      { id: '#18', title: 'Webhook retries', col: 0, row: 0 },
      { id: '#12', title: 'Billing page', col: 1, row: 0, who: 'agent', bad: b1, rot: 0.03 * wob },
      { id: '#88', title: 'PR: refactor auth', col: 2, row: 1, ghost: true, alpha: b2, scale: lerp(0.8, 1, back(seg(lt, d * 0.42, d * 0.42 + 0.7))) },
      { id: '#20', title: 'Invoice PDF', col: 3, row: 0, who: 'agent', bad: b3, rot: -0.025 * wob },
      { id: '#21', title: 'Team invites', col: 0, row: 1, rot: 0.015 * wob },
      { id: '#15', title: 'Status emails', col: 3, row: 2, who: 'agent', rot: 0.02 * wob }
    ];
    cards.forEach(function (c) { drawCard(ctx, g, c, a); });
    // "PR merged" tag hanging off the stuck card
    if (b1 > 0) {
      var p = cardPos(g, 1, 0);
      ctx.save(); ctx.globalAlpha *= a * b1;
      rr(ctx, p.x + 18, p.y + p.h + 10, 150, 34, 17); ctx.fillStyle = '#8957e5'; ctx.fill();
      txt(ctx, 'PR merged', p.x + 93, p.y + p.h + 33, { size: 18, weight: 600, color: '#fff', align: 'center' });
      ctx.restore();
    }
    if (b3 > 0) {
      var q = cardPos(g, 3, 0);
      ctx.save(); ctx.globalAlpha *= a * b3;
      rr(ctx, q.x + 18, q.y + q.h + 10, 160, 34, 17); ctx.fillStyle = C.red; ctx.fill();
      txt(ctx, 'actuals: —', q.x + 98, q.y + q.h + 33, { font: F.mono, size: 18, weight: 600, color: '#fff', align: 'center' });
      ctx.restore();
    }
    txt(ctx, 'Left alone, agents', 120, 250, { font: F.serif, size: 104, alpha: a * eo(seg(lt, 0, 0.9)) });
    var dw = txt(ctx, 'Left alone, agents ', 120, 250, { font: F.serif, size: 104, alpha: 0 });
    ctx.save();
    ctx.translate(120 + dw, 250);
    ctx.rotate(-0.05 * wob);
    txt(ctx, 'drift.', 0, 0, { font: F.serif, size: 104, style: 'italic', color: C.terra, alpha: a * eo(seg(lt, 0.5, 1.4)) });
    ctx.restore();
    // failure list
    var items = [[b1, 'Cards never moved'], [b2, 'PRs for stories nobody claimed'], [b3, 'No record of what it cost']];
    items.forEach(function (it, i) {
      var y = 470 + i * 130, x = 1290;
      ctx.save(); ctx.globalAlpha *= a * it[0]; ctx.translate((1 - it[0]) * 40, 0);
      ctx.strokeStyle = C.red; ctx.lineWidth = 5; ctx.lineCap = 'round';
      ctx.beginPath(); ctx.moveTo(x, y - 30); ctx.lineTo(x + 30, y); ctx.moveTo(x + 30, y - 30); ctx.lineTo(x, y); ctx.stroke();
      txt(ctx, it[1], x + 58, y, { size: 36, weight: 500 });
      ctx.restore();
    });
  };

  S.enforce = function (ctx, lt, d, a) {
    var l1 = eo(seg(lt, 0, 0.7)), l2 = eo(seg(lt, 1.6, 2.3));
    txt(ctx, 'A skill can describe the workflow.', W / 2, 470, { font: F.serif, size: 104, align: 'center', alpha: a * l1 });
    txt(ctx, 'It can’t enforce it.', W / 2, 630, { font: F.serif, size: 104, style: 'italic', align: 'center', color: C.terra, alpha: a * l2 });
    var u = eio(seg(lt, 2.2, 3.0));
    var w = measure(ctx, 'It can’t enforce it.', { font: F.serif, size: 104, style: 'italic' });
    ctx.save(); ctx.globalAlpha *= a; ctx.fillStyle = C.terra;
    ctx.fillRect(W / 2 - w / 2, 662, w * u, 4); ctx.restore();
  };

  function wordmark(ctx, cx, cy, size, a, sub) {
    var w = measure(ctx, 'pinakas', { font: F.serif, size: size });
    // the tablet: a frame drawn around the name, like a pinax
    txt(ctx, 'pinakas', cx, cy, { font: F.serif, size: size, align: 'center', alpha: a });
    return w;
  }

  S.meet = function (ctx, lt, d, a) {
    var f = eio(seg(lt, 0.0, 1.4));
    var bw = 980, bh = 430, bx = W / 2 - bw / 2, by = 250;
    ctx.save(); ctx.globalAlpha *= a;
    ctx.strokeStyle = C.terra; ctx.lineWidth = 3;
    var per = 2 * (bw + bh), len = per * f;
    ctx.setLineDash([len, per]);
    rr(ctx, bx, by, bw, bh, 22); ctx.stroke();
    ctx.setLineDash([]);
    // four column ticks along the bottom of the tablet
    for (var i = 1; i < 4; i++) {
      ctx.globalAlpha = a * eo(seg(lt, 1.0 + i * 0.12, 1.5 + i * 0.12)) * 0.5;
      ctx.fillStyle = C.terra; ctx.fillRect(bx + (bw / 4) * i, by + bh - 70, 2, 46);
    }
    ctx.restore();
    var n = eo(seg(lt, 0.5, 1.5));
    ctx.save(); ctx.globalAlpha *= a * n; ctx.translate(0, (1 - n) * 30);
    wordmark(ctx, W / 2, by + 245, 230, 1);
    ctx.restore();
    txt(ctx, 'πίνακας  ·  “board”', W / 2, by + bh - 34, { font: F.mono, size: 26, align: 'center', color: C.dim, track: 3, alpha: a * eo(seg(lt, 1.3, 2.2)) });
    var t2 = eo(seg(lt, 3.0, 3.9));
    txt(ctx, 'The board is the single source of truth', W / 2, 830, { font: F.serif, size: 66, align: 'center', alpha: a * t2 });
    txt(ctx, 'for human + agent teams.', W / 2, 910, { font: F.serif, size: 66, style: 'italic', align: 'center', color: C.terra, alpha: a * eo(seg(lt, 3.6, 4.5)) });
  };

  function fmtClock(sec) {
    sec = Math.max(0, Math.floor(sec));
    var m = Math.floor(sec / 60), s = sec % 60;
    return '00:' + (m < 10 ? '0' : '') + m + ':' + (s < 10 ? '0' : '') + s;
  }

  S.claim = function (ctx, lt, d, a) {
    kicker(ctx, '01 · CLAIM', a);
    txt(ctx, 'Claim a card. Start the clock.', 120, 250, { font: F.serif, size: 92, alpha: a * eo(seg(lt, 0, 0.8)) });
    var cmd = 'pinakas claim 42';
    var tp = seg(lt, 0.8, 2.2);
    var out = eo(seg(lt, 2.6, 3.0));
    drawTerm(ctx, 120, 340, 820, 560, [
      { prompt: true, s: 'pinakas status --mine', c: C.dim, alpha: 0.55 },
      { s: 'Ready (3)', c: C.dim, alpha: 0.55, size: 24 },
      { s: '  #42  Add OAuth login', c: C.dim, alpha: 0.55, size: 24, gap: 64 },
      { prompt: true, s: typed(cmd, tp), cursor: tp < 1 ? lt : undefined },
      { segs: [{ s: 'Claimed ', c: C.dim }, { s: '#42', c: C.text, w: 600 }, { s: ' Add OAuth login:', c: C.dim }], alpha: out, size: 24 },
      { segs: [{ s: 'assigned to ', c: C.dim }, { s: 'agent', c: C.terra, w: 600 }, { s: ', moved to', c: C.dim }], alpha: out, size: 24 },
      { segs: [{ s: '"In Progress".', c: C.olive, w: 600 }], alpha: out, size: 24 }
    ], a * eo(seg(lt, 0.3, 1.0)));
    var g = boardGeom(1000, 340, 800, 4, 82);
    g.gap = 12; g.colW = (800 - 36) / 4;
    drawBoard(ctx, g, a * eo(seg(lt, 0.4, 1.1)));
    var mv = eio(seg(lt, 2.7, 3.7));
    var clockStart = 3.7;
    drawCard(ctx, g, { id: '#43', title: 'Rate limits', col: 0, row: 1 }, a);
    drawCard(ctx, g, { id: '#47', title: 'SSO docs', col: 0, row: 2 }, a);
    drawCard(ctx, g, { id: '#38', title: 'Audit log', col: 3, row: 0 }, a);
    drawCard(ctx, g, {
      id: '#42', title: 'OAuth login', col: mv, row: 0, who: lt > 2.7 ? 'agent' : '', whoA: eo(seg(lt, 2.7, 3.1)),
      lift: mv > 0 && mv < 1 ? 1 : 0.4 * eo(seg(lt, 2.6, 3)),
      clock: lt > clockStart ? fmtClock((lt - clockStart) * 37) : undefined
    }, a);
  };

  S.hooks = function (ctx, lt, d, a) {
    kicker(ctx, '02 · HOOKS', a);
    txt(ctx, 'Skipping a step is', 120, 250, { font: F.serif, size: 92, alpha: a * eo(seg(lt, 0, 0.8)) });
    var w = measure(ctx, 'Skipping a step is ', { font: F.serif, size: 92 });
    txt(ctx, 'not an option.', 120 + w, 250, { font: F.serif, size: 92, style: 'italic', color: C.terra, alpha: a * eo(seg(lt, 0.4, 1.2)) });
    var c1 = 'gh pr create --fill', c2 = 'stop';
    var t1 = seg(lt, 0.6, 1.6), o1 = eo(seg(lt, 1.9, 2.3));
    var s2 = d * 0.55;
    var t2 = seg(lt, s2, s2 + 0.4), o2 = eo(seg(lt, s2 + 0.6, s2 + 1.0));
    var R = C.red;
    drawTerm(ctx, 120, 340, 1680, 540, [
      { prompt: true, s: typed(c1, t1), cursor: t1 < 1 ? lt : undefined },
      { segs: [{ s: '✕ PreToolUse blocked ', c: R, w: 600 }, { s: '(exit 2)', c: C.mute }], alpha: o1 },
      { segs: [{ s: 'pinakas: no card is claimed in this working tree. Run `pinakas status`', c: C.dim }], alpha: o1, size: 24, gap: 36 },
      { segs: [{ s: 'to pick an unblocked card, then `pinakas claim <issue>` before opening a PR.', c: C.dim }], alpha: o1, size: 24, gap: 76 },
      { segs: [{ s: '● ', c: C.terra }, { s: 'agent wants to ' + typed(c2, t2), c: C.text }], alpha: eo(seg(lt, s2 - 0.3, s2)) },
      { segs: [{ s: '✕ Stop blocked ', c: R, w: 600 }, { s: '(exit 2)', c: C.mute }], alpha: o2 },
      { segs: [{ s: 'pinakas: card #42 (Add OAuth login) has no actuals yet. Record them with', c: C.dim }], alpha: o2, size: 24, gap: 36 },
      { segs: [{ s: '`pinakas actuals 42 --tokens <n> --first-pass=<yes|no>` before stopping.', c: C.dim }], alpha: o2, size: 24 }
    ], a * eo(seg(lt, 0.2, 0.8)), 'claude · PreToolUse + Stop hooks');
    // shield stamp
    [[o1, 1.9], [o2, s2 + 0.6]].forEach(function (s, i) {
      var k = back(seg(lt, s[1], s[1] + 0.5));
      if (k <= 0) return;
      ctx.save(); ctx.globalAlpha *= a * clamp(s[0]);
      ctx.translate(1660, 470 + i * 340); ctx.rotate(-0.12); ctx.scale(k, k);
      rr(ctx, -95, -32, 190, 64, 10); ctx.strokeStyle = R; ctx.lineWidth = 4; ctx.stroke();
      txt(ctx, 'BLOCKED', 0, 11, { font: F.mono, size: 30, weight: 700, color: R, align: 'center', track: 4 });
      ctx.restore();
    });
  };

  S.actuals = function (ctx, lt, d, a) {
    kicker(ctx, '03 · ACTUALS', a);
    txt(ctx, 'Every story, the same numbers.', 120, 250, { font: F.serif, size: 92, alpha: a * eo(seg(lt, 0, 0.8)) });
    var tiles = [
      { k: 'Duration', v: function (p) { return Math.round(47 * p) + ' min'; }, sub: 'claim → actuals, from timestamps' },
      { k: 'Tokens', v: function (p) { return Math.round(182 * p) + 'k'; }, sub: 'reported by the agent' },
      { k: 'First pass', v: function (p) { return p > 0.5 ? 'yes' : '—'; }, sub: 'review passed without changes', color: C.olive },
      { k: 'Findings', v: function (p) { return 'major ' + Math.round(0 * p) + ' · minor ' + Math.round(2 * p); }, sub: 'review findings by severity', small: true }
    ];
    tiles.forEach(function (t, i) {
      var x = 120 + i * 432, y = 360, w = 408, h = 330;
      var p = eo(seg(lt, 0.5 + i * 0.45, 1.6 + i * 0.45));
      ctx.save(); ctx.globalAlpha *= a * p; ctx.translate(0, (1 - p) * 40);
      rr(ctx, x, y, w, h, 18); ctx.fillStyle = C.panel; ctx.fill(); ctx.strokeStyle = C.line; ctx.lineWidth = 1.5; ctx.stroke();
      txt(ctx, t.k.toUpperCase(), x + 32, y + 62, { font: F.mono, size: 20, color: C.dim, track: 3 });
      txt(ctx, t.v(eo(seg(lt, 0.8 + i * 0.45, 2.4 + i * 0.45))), x + 32, y + 190, { font: F.serif, size: t.small ? 64 : 104, color: t.color || C.text });
      ctx.fillStyle = C.line; ctx.fillRect(x + 32, y + 236, w - 64, 1.5);
      txt(ctx, t.sub, x + 32, y + 282, { size: 21, color: C.dim });
      ctx.restore();
    });
    var cm = eo(seg(lt, 3.0, 3.8));
    txt(ctx, '<!-- pinakas:actuals {"v":1,"number":42,"duration_min":47,"tokens":182000,"first_pass":true,"findings":{"minor":2}} -->', 120, 790, { font: F.mono, size: 22, color: C.mute, alpha: a * cm });
    txt(ctx, 'Posted on the issue with a machine-readable marker that CI and reports parse back.', 120, 850, { size: 28, color: C.dim, alpha: a * cm });
  };

  S.action = function (ctx, lt, d, a) {
    kicker(ctx, '04 · GITHUB ACTION', a);
    txt(ctx, 'The board follows the PR.', 120, 250, { font: F.serif, size: 92, alpha: a * eo(seg(lt, 0, 0.8)) });
    var e1 = 1.2, e2 = d * 0.42;
    var g = boardGeom(820, 360, 980, 3, 82);
    drawBoard(ctx, g, a * eo(seg(lt, 0.2, 0.9)));
    var m1 = eio(seg(lt, e1, e1 + 0.9)), m2 = eio(seg(lt, e2, e2 + 0.9));
    drawCard(ctx, g, { id: '#44', title: 'Retry queue', col: 0, row: 0 }, a);
    drawCard(ctx, g, { id: '#38', title: 'Audit log', col: 3, row: 1 }, a);
    drawCard(ctx, g, { id: '#42', title: 'OAuth login', col: 1 + m1 + m2, row: 0, who: 'agent', lift: (m1 > 0 && m1 < 1) || (m2 > 0 && m2 < 1) ? 1 : 0, good: m2 >= 1 ? 0.9 : 0 }, a);
    var ev = [
      { t: e1, c: '#3fb950', label: 'PR #57 opened', sub: 'Closes #42  →  card to In Review' },
      { t: e2, c: '#8957e5', label: 'PR #57 merged', sub: 'card to Done, timing comment posted' }
    ];
    ev.forEach(function (e, i) {
      var p = eo(seg(lt, e.t - 0.2, e.t + 0.4));
      var y = 400 + i * 150;
      ctx.save(); ctx.globalAlpha *= a * p; ctx.translate(-(1 - p) * 30, 0);
      ctx.fillStyle = e.c; ctx.beginPath(); ctx.arc(142, y, 14, 0, Math.PI * 2); ctx.fill();
      if (i === 0) { ctx.fillStyle = C.line; ctx.fillRect(141, y + 22, 2, 106); }
      txt(ctx, e.label, 176, y + 10, { size: 34, weight: 600 });
      txt(ctx, e.sub, 176, y + 54, { size: 23, color: C.dim });
      ctx.restore();
    });
    var cp = eo(seg(lt, e2 + 1.0, e2 + 1.8));
    ctx.save(); ctx.globalAlpha *= a * cp;
    rr(ctx, 120, 650, 640, 220, 16); ctx.fillStyle = C.panel; ctx.fill(); ctx.strokeStyle = C.line; ctx.lineWidth = 1.5; ctx.stroke();
    txt(ctx, 'pinakas · timing', 150, 696, { font: F.mono, size: 20, color: C.dim, track: 2 });
    [['claim → PR', '41 min'], ['PR → merge', '2 h 10 min'], ['claim → merge', '2 h 51 min']].forEach(function (r, i) {
      txt(ctx, r[0], 150, 750 + i * 46, { size: 26, color: C.text });
      txt(ctx, r[1], 730, 750 + i * 46, { font: F.mono, size: 26, color: i === 2 ? C.terra : C.text, align: 'right', weight: 600 });
    });
    ctx.restore();
    txt(ctx, 'Measured from timestamps, not from what the agent says.', 820, 800, { font: F.serif, size: 40, style: 'italic', color: C.dim, alpha: a * eo(seg(lt, e2 + 1.8, e2 + 2.6)) });
  };

  S.check = function (ctx, lt, d, a) {
    kicker(ctx, '05 · REQUIRED CHECK', a);
    txt(ctx, 'A check that keeps everyone honest.', 120, 250, { font: F.serif, size: 92, alpha: a * eo(seg(lt, 0, 0.8)) });
    var x = 120, y = 330, w = 1680, h = 560;
    ctx.save(); ctx.globalAlpha *= a * eo(seg(lt, 0.3, 1.0));
    rr(ctx, x, y, w, h, 18); ctx.fillStyle = C.panel; ctx.fill(); ctx.strokeStyle = C.line; ctx.lineWidth = 1.5; ctx.stroke();
    ctx.restore();
    var rows = [
      ['Pull request closes a card of the board', 'Closes #42'],
      ['Every linked card has actuals', '47 min · 182k tokens'],
      ['risk:high has an approval from a human', 'approved by @marco']
    ];
    var at = [d * 0.22, d * 0.45, d * 0.68];
    rows.forEach(function (r, i) {
      var ry = y + 90 + i * 120;
      var p = eo(seg(lt, at[i] - 0.4, at[i] + 0.2));
      var ok = back(seg(lt, at[i], at[i] + 0.45));
      ctx.save(); ctx.globalAlpha *= a * p;
      ctx.beginPath(); ctx.arc(x + 70, ry, 24, 0, Math.PI * 2);
      ctx.fillStyle = ok > 0 ? 'rgba(157,176,106,' + clamp(ok) + ')' : 'transparent'; ctx.fill();
      ctx.strokeStyle = ok > 0 ? C.olive : C.mute; ctx.lineWidth = 2.5; ctx.stroke();
      if (ok > 0) {
        ctx.save(); ctx.translate(x + 70, ry); ctx.scale(ok, ok);
        ctx.strokeStyle = C.bg; ctx.lineWidth = 5; ctx.lineCap = 'round'; ctx.lineJoin = 'round';
        ctx.beginPath(); ctx.moveTo(-10, 1); ctx.lineTo(-3, 9); ctx.lineTo(11, -8); ctx.stroke(); ctx.restore();
      }
      txt(ctx, r[0], x + 120, ry + 12, { size: 36, weight: 500 });
      txt(ctx, r[1], x + w - 50, ry + 10, { font: F.mono, size: 24, color: C.dim, align: 'right' });
      ctx.restore();
      if (i < 2) { ctx.save(); ctx.globalAlpha *= a * 0.6; ctx.fillStyle = C.line; ctx.fillRect(x + 40, ry + 60, w - 80, 1.5); ctx.restore(); }
    });
    var fin = eo(seg(lt, at[2] + 0.6, at[2] + 1.2));
    ctx.save(); ctx.globalAlpha *= a * fin;
    rr(ctx, x + 40, y + h - 130, w - 80, 90, 12); ctx.fillStyle = 'rgba(157,176,106,0.14)'; ctx.fill(); ctx.strokeStyle = C.olive; ctx.lineWidth = 2; ctx.stroke();
    txt(ctx, '✓  pinakas / check', x + 80, y + h - 72, { font: F.mono, size: 30, weight: 600, color: C.olive });
    txt(ctx, 'Required · Successful', x + w - 80, y + h - 72, { size: 26, color: C.dim, align: 'right' });
    ctx.restore();
  };


  var bars = [42, 18, 65, 33, 27, 51, 12, 38, 46, 22, 58, 31, 20, 36];
  var defectAt = { 2: '#41', 10: '#52' };

  S.report = function (ctx, lt, d, a) {
    kicker(ctx, '06 · REPORT', a);
    txt(ctx, 'From actuals to answers.', 120, 250, { font: F.serif, size: 92, alpha: a * eo(seg(lt, 0, 0.8)) });
    var cmd = 'pinakas report --since 30d';
    var tp = seg(lt, 0.5, 1.5);
    ctx.save(); ctx.globalAlpha *= a * eo(seg(lt, 0.3, 0.7));
    var cw = txt(ctx, '$ ', 1180, 240, { font: F.mono, size: 28, color: C.terra });
    var tw = txt(ctx, typed(cmd, tp), 1180 + cw, 240, { font: F.mono, size: 28 });
    if (tp < 1 && Math.floor(lt * 2) % 2 === 0) { ctx.fillStyle = C.text; ctx.fillRect(1186 + cw + tw, 216, 14, 30); }
    ctx.restore();
    var tiles = [
      { k: 'Median agent time', v: function (p) { return Math.round(33 * p) + ' min'; }, at: 0.3 },
      { k: 'First pass', v: function (p) { return Math.round(72 * p) + '%'; }, at: 0.42, color: C.olive },
      { k: 'Median lead time', v: function (p) { return p < 1 ? Math.round(130 * p) + ' min' : '2 h 10'; }, at: 0.54 },
      { k: 'Defects in 30 days', v: function (p) { return Math.round(8 * p) + '%'; }, at: 0.68, color: C.red }
    ];
    tiles.forEach(function (t, i) {
      var x = 120 + i * 432, y = 330, w = 408, h = 200;
      var s0 = d * t.at - 0.6;
      var p = eo(seg(lt, s0, s0 + 0.7));
      ctx.save(); ctx.globalAlpha *= a * p; ctx.translate(0, (1 - p) * 30);
      rr(ctx, x, y, w, h, 18); ctx.fillStyle = C.panel; ctx.fill(); ctx.strokeStyle = C.line; ctx.lineWidth = 1.5; ctx.stroke();
      txt(ctx, t.k.toUpperCase(), x + 30, y + 52, { font: F.mono, size: 18, color: C.dim, track: 2 });
      txt(ctx, t.v(eo(seg(lt, s0 + 0.2, s0 + 1.4))), x + 30, y + 158, { font: F.serif, size: 86, color: t.color || C.text });
      ctx.restore();
    });
    // agent time per story, defects marked in red
    var bx = 120, by = 820, bw = 1680, maxH = 160, gap = 18;
    var w1 = (bw - gap * (bars.length - 1)) / bars.length;
    ctx.save(); ctx.globalAlpha *= a * eo(seg(lt, 1.0, 1.6));
    ctx.fillStyle = C.line; ctx.fillRect(bx, by + 2, bw, 2);
    txt(ctx, 'AGENT TIME PER STORY · LAST 30 DAYS', bx + bw, 590, { font: F.mono, size: 18, color: C.mute, track: 2, align: 'right' });
    ctx.restore();
    var dl = eo(seg(lt, d * 0.7, d * 0.7 + 0.6));
    bars.forEach(function (v, i) {
      var g = eo(seg(lt, 1.2 + i * 0.12, 2.0 + i * 0.12));
      var h = maxH * v / 65 * g, x = bx + i * (w1 + gap);
      var bad = defectAt[i];
      ctx.save(); ctx.globalAlpha *= a;
      rr(ctx, x, by - h, w1, h, 6);
      ctx.fillStyle = bad && dl > 0 ? lerpColor(C.paper2, C.red, dl) : C.paper2; ctx.fill();
      if (bad && dl > 0) {
        ctx.globalAlpha = a * dl;
        ctx.beginPath(); ctx.arc(x + w1 / 2, by - h - 26, 10, 0, Math.PI * 2); ctx.fillStyle = C.red; ctx.fill();
        txt(ctx, 'defect ' + bad, x + w1 / 2, by - h - 48, { font: F.mono, size: 18, color: C.red, align: 'center', weight: 600 });
      }
      ctx.restore();
    });
  };

  S.failopen = function (ctx, lt, d, a) {
    kicker(ctx, '07 · FAIL-OPEN', a);
    txt(ctx, 'Guardrails fail open.', 120, 250, { font: F.serif, size: 92, alpha: a * eo(seg(lt, 0, 0.8)) });
    var cut = eo(seg(lt, 1.0, 1.6));
    // two nodes: agent and board, with a broken link between them
    var ax = 420, bx = 1500, y = 560;
    ctx.save(); ctx.globalAlpha *= a;
    [[ax, 'agent'], [bx, 'board']].forEach(function (n, i) {
      rr(ctx, n[0] - 160, y - 80, 320, 160, 22);
      ctx.fillStyle = C.panel; ctx.fill();
      ctx.strokeStyle = i === 1 ? 'rgba(229,72,77,' + (0.3 + 0.7 * cut) + ')' : C.line; ctx.lineWidth = 2; ctx.stroke();
      txt(ctx, n[1], n[0], y + 16, { font: F.serif, size: 60, align: 'center', color: i === 1 ? lerpColor(C.text, C.mute, cut) : C.text });
    });
    ctx.setLineDash([14, 12]); ctx.strokeStyle = C.line; ctx.lineWidth = 3;
    ctx.beginPath(); ctx.moveTo(ax + 180, y); ctx.lineTo(960 - 40 * cut, y); ctx.moveTo(960 + 40 * cut, y); ctx.lineTo(bx - 180, y); ctx.stroke();
    ctx.setLineDash([]);
    if (cut > 0) {
      ctx.strokeStyle = C.red; ctx.lineWidth = 5; ctx.lineCap = 'round';
      ctx.beginPath(); ctx.moveTo(945, y - 20); ctx.lineTo(975, y + 20); ctx.moveTo(975, y - 20); ctx.lineTo(945, y + 20); ctx.globalAlpha = a * cut; ctx.stroke();
    }
    ctx.restore();
    // the work keeps flowing: commits moving along a track under the agent
    var flow = eo(seg(lt, 1.8, 2.4));
    ctx.save(); ctx.globalAlpha *= a * flow;
    ctx.fillStyle = C.line; ctx.fillRect(120, 790, 1680, 3);
    for (var i = 0; i < 9; i++) {
      var px = 120 + ((lt * 180 + i * 200) % 1680);
      ctx.beginPath(); ctx.arc(px, 791, 11, 0, Math.PI * 2); ctx.fillStyle = C.olive; ctx.fill();
    }
    ctx.restore();
    txt(ctx, 'pinakas: board unreachable, checking the local claim only', 120, 880, { font: F.mono, size: 26, color: C.ochre, alpha: a * eo(seg(lt, 1.6, 2.2)) });
    txt(ctx, 'Work goes on.', 1800, 880, { font: F.serif, size: 54, style: 'italic', color: C.terra, align: 'right', alpha: a * eo(seg(lt, 2.4, 3.1)) });
  };

  function lerpColor(c1, c2, t) {
    var a = parseInt(c1.slice(1), 16), b = parseInt(c2.slice(1), 16);
    var r = Math.round(lerp(a >> 16, b >> 16, t)), g = Math.round(lerp((a >> 8) & 255, (b >> 8) & 255, t)), bl = Math.round(lerp(a & 255, b & 255, t));
    return 'rgb(' + r + ',' + g + ',' + bl + ')';
  }

  S.tools = function (ctx, lt, d, a) {
    kicker(ctx, 'WORKS WITH', a);
    txt(ctx, 'Your agents, your board.', 120, 250, { font: F.serif, size: 92, alpha: a * eo(seg(lt, 0, 0.8)) });
    var items = [
      ['Claude Code', 'plugin: skills + hooks', 'today', C.olive],
      ['GitHub Copilot', 'repository hooks', 'today', C.olive],
      ['Codex', 'project hooks + skills', 'today', C.olive],
      ['GitHub Projects', 'board backend', 'today', C.olive],
      ['GitLab', 'board backend', 'v0.4', C.ochre],
      ['MCP server', 'any MCP client', 'v0.4', C.ochre]
    ];
    items.forEach(function (it, i) {
      var cx = 120 + (i % 3) * 568, cy = 350 + Math.floor(i / 3) * 280;
      var start = i < 4 ? 0.4 + i * 0.35 : d * 0.62 + (i - 4) * 0.3;
      var p = eo(seg(lt, start, start + 0.7));
      ctx.save(); ctx.globalAlpha *= a * p; ctx.translate(0, (1 - p) * 30);
      rr(ctx, cx, cy, 544, 240, 18); ctx.fillStyle = C.panel; ctx.fill(); ctx.strokeStyle = C.line; ctx.lineWidth = 1.5; ctx.stroke();
      txt(ctx, it[0], cx + 36, cy + 112, { font: F.serif, size: 62 });
      txt(ctx, it[1], cx + 36, cy + 170, { size: 24, color: C.dim });
      var bw = measure(ctx, it[2], { font: F.mono, size: 18, weight: 600 }) + 30;
      rr(ctx, cx + 544 - bw - 28, cy + 28, bw, 34, 17); ctx.fillStyle = it[3]; ctx.fill();
      txt(ctx, it[2], cx + 544 - bw / 2 - 28, cy + 51, { font: F.mono, size: 18, weight: 600, color: C.bg, align: 'center' });
      ctx.restore();
    });
  };

  S.cta = function (ctx, lt, d, a) {
    var n = eo(seg(lt, 0, 1.0));
    ctx.save(); ctx.globalAlpha *= a * n; ctx.translate(0, (1 - n) * 24);
    wordmark(ctx, W / 2, 420, 220, 1);
    ctx.restore();
    txt(ctx, 'Keep AI coding agents', W / 2, 560, { font: F.serif, size: 76, align: 'center', alpha: a * eo(seg(lt, 0.6, 1.4)) });
    txt(ctx, 'on the board.', W / 2, 648, { font: F.serif, size: 76, style: 'italic', color: C.terra, align: 'center', alpha: a * eo(seg(lt, 1.0, 1.8)) });
    var p = eo(seg(lt, 2.0, 2.8));
    ctx.save(); ctx.globalAlpha *= a * p;
    var label = 'github.com/mmornati/pinakas';
    var w = measure(ctx, label, { font: F.mono, size: 30, weight: 500 }) + 64;
    rr(ctx, W / 2 - w / 2, 720, w, 70, 35); ctx.fillStyle = C.terra; ctx.fill();
    txt(ctx, label, W / 2, 765, { font: F.mono, size: 30, weight: 500, color: '#fff', align: 'center' });
    ctx.restore();
    txt(ctx, 'Open source · Apache 2.0 · Go', W / 2, 850, { font: F.mono, size: 22, color: C.dim, align: 'center', track: 3, alpha: a * eo(seg(lt, 2.6, 3.4)) });
  };

  /* ---------- timeline ---------- */

  function sceneAt(tl, t) {
    var sc = tl.scenes, i = 0;
    for (var k = 0; k < sc.length; k++) if (t >= sc[k].start - (k === 0 ? 99 : 0.25)) i = k;
    return i;
  }
  function sceneSpan(tl, i) {
    var sc = tl.scenes;
    var s = i === 0 ? 0 : sc[i].start - 0.25;
    var e = i + 1 < sc.length ? sc[i + 1].start - 0.25 : tl.duration;
    return [s, e];
  }

  function caption(ctx, tl, t) {
    var sc = tl.scenes[sceneAt(tl, t)];
    if (t < sc.start - 0.1 || t > sc.end + 0.3) return;
    // split into sentences and give each a share of the spoken time by length
    var parts = sc.text.match(/[^.?!]+[.?!]+/g) || [sc.text];
    var total = parts.reduce(function (n, p) { return n + p.length; }, 0);
    var acc = sc.start, cur = parts[parts.length - 1];
    for (var i = 0; i < parts.length; i++) {
      var dur = (sc.end - sc.start) * parts[i].length / total;
      if (t < acc + dur) { cur = parts[i]; break; }
      acc += dur;
    }
    cur = cur.trim();
    var o = { size: 34, weight: 500 };
    var lines = wrapLines(ctx, cur, 1400, o);
    var lh = 46, bh = lines.length * lh + 26, by = H - 70 - bh;
    var bw = Math.max.apply(null, lines.map(function (l) { return measure(ctx, l, o); })) + 52;
    ctx.save();
    rr(ctx, W / 2 - bw / 2, by, bw, bh, 12); ctx.fillStyle = 'rgba(12,11,10,0.82)'; ctx.fill();
    lines.forEach(function (l, k) { txt(ctx, l, W / 2, by + 46 + k * lh, { size: 34, weight: 500, align: 'center', color: '#fff' }); });
    ctx.restore();
  }

  function draw(ctx, t, tl, opts) {
    opts = opts || {};
    var sx = ctx.canvas.width / W, sy = ctx.canvas.height / H;
    ctx.save();
    ctx.setTransform(sx, 0, 0, sy, 0, 0);
    ctx.fillStyle = C.bg; ctx.fillRect(0, 0, W, H);
    grain(ctx);
    var i = sceneAt(tl, t), span = sceneSpan(tl, i);
    var lt = t - span[0], d = span[1] - span[0];
    var fade = 0.45;
    var aIn = i === 0 ? 1 : eo(lt / fade);
    if (i > 0 && lt < fade) {
      var ps = sceneSpan(tl, i - 1);
      ctx.save(); ctx.globalAlpha = 1 - aIn;
      S[tl.scenes[i - 1].id](ctx, ps[1] - ps[0], ps[1] - ps[0], 1);
      ctx.restore();
    }
    ctx.save(); ctx.globalAlpha = aIn;
    S[tl.scenes[i].id](ctx, lt, d, 1);
    ctx.restore();
    if (opts.captions) caption(ctx, tl, t);
    if (opts.progress) {
      ctx.fillStyle = 'rgba(241,235,223,0.12)'; ctx.fillRect(0, H - 6, W, 6);
      ctx.fillStyle = C.terra; ctx.fillRect(0, H - 6, W * clamp(t / tl.duration), 6);
    }
    ctx.restore();
  }

  function fontsReady() {
    if (!document.fonts || !document.fonts.load) return Promise.resolve();
    return Promise.all([
      document.fonts.load('400 40px "Instrument Serif"'),
      document.fonts.load('italic 400 40px "Instrument Serif"'),
      document.fonts.load('400 40px Geist'), document.fonts.load('600 40px Geist'),
      document.fonts.load('400 40px "Geist Mono"'), document.fonts.load('600 40px "Geist Mono"')
    ]).catch(function () {});
  }

  global.PinakasFilm = { draw: draw, W: W, H: H, fontsReady: fontsReady, sceneAt: sceneAt };
})(window);
