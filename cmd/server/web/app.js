'use strict';

/* ============================================================
   American Blackjack — interface de table
   Le serveur Go est seul maître du jeu : cette page n'implémente
   aucune règle. Elle affiche un état et transmet des actions, ce
   qui garantit que ce qu'on joue ici est exactement ce que le
   moteur simule.
   ============================================================ */

const SUITS = {
  Pique:   { glyph: '♠', red: false },
  Coeur:   { glyph: '♥', red: true  },
  Carreau: { glyph: '♦', red: true  },
  Trefle:  { glyph: '♣', red: false },
};

const ACTION_LABELS = {
  hit: 'Tirer', stand: 'Rester', double: 'Doubler',
  split: 'Séparer', surrender: 'Abandonner',
  insure: 'Assurance', decline: 'Refuser',
};

const RESULT_LABELS = {
  gagne: 'Gagné', perdu: 'Perdu', egalite: 'Égalité',
  saute: 'Sauté', abandon: 'Abandon', blackjack: 'Blackjack',
};

const KEYS = { t: 'hit', r: 'stand', d: 'double', s: 'split', a: 'surrender' };

const CHIPS = [1, 5, 25, 100];

let state = null;
let chip = 5;
let pending = { main: 0, perfectPairs: 0, twentyOnePlus3: 0, luckyLadies: 0, buster: 0 };
let lastBets = null;
let busy = false;

const $ = (sel) => document.querySelector(sel);
const $$ = (sel) => Array.from(document.querySelectorAll(sel));

/* ---------------------- Réseau ---------------------- */

async function api(path, body) {
  const opts = body === undefined
    ? {}
    : { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) };
  const res = await fetch(path, opts);
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(data.error || `erreur ${res.status}`);
  return data;
}

function toast(msg) {
  const el = $('#toast');
  el.textContent = msg;
  el.classList.add('show');
  clearTimeout(toast._t);
  toast._t = setTimeout(() => el.classList.remove('show'), 2600);
}

async function guard(fn) {
  if (busy) return;
  busy = true;
  try { await fn(); }
  catch (e) { toast(e.message); }
  finally { busy = false; }
}

/* ---------------------- Rendu des cartes ---------------------- */

function cardEl(card) {
  const el = document.createElement('div');
  const suit = SUITS[card.suit] || { glyph: '?', red: false };
  el.className = 'card' + (suit.red ? ' red' : '');
  el.innerHTML =
    `<span class="r">${card.rank}</span>` +
    `<span class="mid">${suit.glyph}</span>` +
    `<span class="s">${suit.glyph}</span>`;
  return el;
}

function backEl() {
  const el = document.createElement('div');
  el.className = 'card back';
  return el;
}

/* ---------------------- Rendu de la table ---------------------- */

function render() {
  if (!state) return;

  $('#bankroll').textContent = fmtMoney(state.bankroll);
  renderRules();
  renderShoe();
  renderDealer();
  renderHands();
  renderBanner();
  renderSideResults();
  renderControls();
}

function renderRules() {
  const r = state.rules;
  $('#rules-summary').textContent =
    `${r.numDecks} jeux · croupier ${r.dealerHitsSoft17 ? 'tire' : 'reste'} sur 17 souple · ` +
    `blackjack ${r.blackjackPayout === 1.5 ? '3:2' : r.blackjackPayout + ':1'} · ` +
    `${r.lateSurrender ? 'abandon' : 'sans abandon'} · ${r.maxSplitHands} mains max`;
}

function renderShoe() {
  const s = state.shoe;
  const frac = s.size ? s.remaining / s.size : 0;
  $('#shoe-fill').style.width = (frac * 100).toFixed(1) + '%';
  $('#shoe-count').textContent = `${s.remaining}/${s.size}`;
  $('#shoe-shuffles').textContent =
    s.cutReached ? 'coupe atteinte, rebattage au prochain coup' : `${s.shuffles} rebattage(s)`;
}

function renderDealer() {
  const box = $('#dealer-cards');
  box.innerHTML = '';
  const d = state.dealer || { cards: [] };

  (d.cards || []).forEach((c) => box.appendChild(cardEl(c)));
  if (d.hidden) box.appendChild(backEl());

  const badge = $('#dealer-total');
  if (!d.cards || !d.cards.length) {
    badge.textContent = '';
    badge.className = 'total-badge';
    return;
  }
  badge.className = 'total-badge' + (d.bust ? ' bust' : '');
  if (d.blackjack) badge.textContent = 'Blackjack';
  else if (d.bust) badge.textContent = `${d.total} — sauté`;
  else badge.textContent = d.hidden ? `${d.total}+` : `${d.total}${d.soft ? ' souple' : ''}`;
}

function renderHands() {
  const box = $('#player-hands');
  box.innerHTML = '';
  if (!state.hands || !state.hands.length) return;

  state.hands.forEach((h, i) => {
    const el = document.createElement('div');
    el.className = 'hand' + (h.active ? ' active' : '') + (h.result ? ' settled' : '');

    const label = document.createElement('div');
    label.className = 'zone-label';
    label.textContent = state.hands.length > 1 ? `Main ${i + 1}` : 'Vous';
    el.appendChild(label);

    const cards = document.createElement('div');
    cards.className = 'cards';
    h.cards.forEach((c) => cards.appendChild(cardEl(c)));
    el.appendChild(cards);

    const badge = document.createElement('div');
    badge.className = 'total-badge' + (h.bust ? ' bust' : '');
    if (h.blackjack) badge.textContent = 'Blackjack';
    else if (h.bust) badge.textContent = `${h.total} — sauté`;
    else badge.textContent = `${h.total}${h.soft ? ' souple' : ''}`;
    el.appendChild(badge);

    const bet = document.createElement('div');
    bet.className = 'hand-bet';
    bet.textContent = fmtMoney(h.bet) + (h.doubled ? ' (doublée)' : '');
    el.appendChild(bet);

    if (h.result) {
      const res = document.createElement('div');
      res.className = 'hand-result r-' + h.result;
      res.textContent = RESULT_LABELS[h.result] || h.result;
      el.appendChild(res);
    }

    box.appendChild(el);
  });
}

function renderBanner() {
  const parts = [...(state.messages || [])];
  if (state.phase === 'insurance') parts.push("Le croupier montre un As : assurance ?");
  if (state.phase === 'done' && state.roundNet !== 0) {
    parts.push(state.roundNet > 0
      ? `Vous gagnez ${fmtMoney(state.roundNet)}`
      : `Vous perdez ${fmtMoney(-state.roundNet)}`);
  }
  $('#banner').textContent = parts.join('  ·  ');

  const box = $('#roundnet-box');
  if (state.phase === 'done') {
    box.hidden = false;
    const el = $('#roundnet');
    el.textContent = (state.roundNet > 0 ? '+' : '') + fmtMoney(state.roundNet);
    el.style.color = state.roundNet > 0 ? 'var(--win)' : state.roundNet < 0 ? 'var(--lose)' : 'var(--push)';
  } else {
    box.hidden = true;
  }
}

function renderSideResults() {
  const box = $('#side-results');
  box.innerHTML = '';
  (state.sideResults || []).forEach((sr) => {
    const el = document.createElement('div');
    el.className = 'sr' + (sr.net > 0 ? ' win' : '');
    el.innerHTML = `<b>${sr.name}</b>${sr.label === 'perdu' ? 'perdu' : sr.label.replace(/_/g, ' ')} · ` +
      `${sr.net > 0 ? '+' : ''}${fmtMoney(sr.net)}`;
    box.appendChild(el);
  });
}

function renderControls() {
  const betting = state.phase === 'betting' || state.phase === 'done';
  $('#betting-ui').hidden = !betting;
  $('#action-ui').hidden = betting;

  if (betting) {
    renderSpots();
    $('#btn-deal').disabled = pending.main <= 0 || totalPending() > state.bankroll;
    $('#btn-rebet').disabled = !lastBets;
    $('#btn-deal').textContent = state.phase === 'done' ? 'Coup suivant' : 'Distribuer';
    return;
  }

  const legal = state.legalActions || [];
  const showHint = $('#hint-toggle').checked;
  $$('.actions button').forEach((b) => {
    const a = b.dataset.action;
    const ok = legal.includes(a);
    b.hidden = !ok;
    b.disabled = !ok;
    b.classList.toggle('advised', showHint && ok && a === state.hint);
  });
}

function renderSpots() {
  $$('.spot').forEach((el) => {
    const key = el.dataset.spot;
    const v = pending[key] || 0;
    el.querySelector('[data-amount]').textContent = v;
    el.classList.toggle('filled', v > 0);
  });
}

function totalPending() {
  return Object.values(pending).reduce((a, b) => a + b, 0);
}

function fmtMoney(v) {
  return (Math.round(v * 100) / 100).toLocaleString('fr-FR');
}

/* ---------------------- Interactions de table ---------------------- */

function buildChips() {
  const box = $('#chips');
  CHIPS.forEach((v) => {
    const b = document.createElement('button');
    b.className = 'chip' + (v === chip ? ' sel' : '');
    b.dataset.v = v;
    b.textContent = v;
    b.onclick = () => {
      chip = v;
      $$('.chip').forEach((c) => c.classList.toggle('sel', Number(c.dataset.v) === v));
    };
    box.appendChild(b);
  });
}

function wireSpots() {
  $$('.spot').forEach((el) => {
    const key = el.dataset.spot;
    el.onclick = () => {
      if (totalPending() + chip > state.bankroll) { toast('Solde insuffisant.'); return; }
      pending[key] += chip;
      renderControls();
    };
    el.oncontextmenu = (e) => {
      e.preventDefault();
      pending[key] = Math.max(0, pending[key] - chip);
      renderControls();
    };
  });
}

async function deal() {
  const bet = pending.main;
  const sideBets = {
    perfectPairs: pending.perfectPairs,
    twentyOnePlus3: pending.twentyOnePlus3,
    luckyLadies: pending.luckyLadies,
    buster: pending.buster,
  };
  lastBets = { ...pending };
  state = await api('/api/deal', { bet, sideBets });
  render();
}

async function act(action) {
  state = await api('/api/action', { action });
  render();
}

/* ---------------------- Onglets ---------------------- */

function wireTabs() {
  $$('.tab-btn').forEach((btn) => {
    btn.onclick = () => {
      $$('.tab-btn').forEach((b) => b.classList.toggle('active', b === btn));
      $$('.panel').forEach((p) => p.classList.toggle('active', p.id === 'tab-' + btn.dataset.tab));
      if (btn.dataset.tab === 'strategy') loadStrategy();
    };
  });
}

/* ---------------------- Grille de stratégie ---------------------- */

let strategyLoaded = false;

async function loadStrategy() {
  if (strategyLoaded) return;
  const s = await api('/api/strategy');
  const box = $('#strategy-grids');
  box.innerHTML = '';
  box.appendChild(gridTable('Totaux durs', s.dealer, s.hard, (k) => Number(k), (k) => k));
  box.appendChild(gridTable('Totaux souples', s.dealer, s.soft, (k) => Number(k), (k) => `A,${Number(k) - 11}`));
  box.appendChild(gridTable('Paires', s.dealer, s.pairs, pairOrder, (k) => `${k},${k}`));
  strategyLoaded = true;
}

function pairOrder(k) {
  if (k === 'A') return 99;
  return Number(k);
}

function gridTable(title, dealer, rows, orderFn, labelFn) {
  const block = document.createElement('div');
  block.className = 'grid-block';
  block.innerHTML = `<h3>${title}</h3>`;

  const table = document.createElement('table');
  table.className = 'strat';

  const head = document.createElement('tr');
  head.innerHTML = '<th></th>' + dealer.map((d) => `<th>${d}</th>`).join('');
  table.appendChild(head);

  Object.keys(rows)
    .sort((a, b) => orderFn(b) - orderFn(a))
    .forEach((key) => {
      const tr = document.createElement('tr');
      tr.innerHTML = `<th>${labelFn(key)}</th>` +
        rows[key].split('').map((d) => `<td class="d-${d}">${d}</td>`).join('');
      table.appendChild(tr);
    });

  block.appendChild(table);
  return block;
}

/* ---------------------- Statistiques ---------------------- */

async function runSim() {
  const btn = $('#btn-sim');
  btn.disabled = true;
  btn.textContent = 'Simulation en cours…';
  try {
    const q = new URLSearchParams({
      rounds: $('#sim-rounds').value,
      decks: $('#sim-decks').value,
      h17: $('#sim-h17').checked,
      sidebets: $('#sim-side').checked,
    });
    const d = await api('/api/simulate?' + q);
    $('#sim-out').innerHTML = [
      metric('Avantage de la maison', d.houseEdge.toFixed(4), '%', true),
      metric('Erreur-type', '± ' + d.stdError.toFixed(4), 'point'),
      metric('Element of risk', d.elementOfRisk.toFixed(4), '%'),
      metric('Écart-type par coup', d.stdDev.toFixed(4), 'unité de mise'),
      metric('Débit', Math.round(d.roundsPerSec).toLocaleString('fr-FR'), 'coups/s', true),
      metric('Temps par coup', Math.round(d.nsPerRound).toLocaleString('fr-FR'), 'ns'),
      metric('Coups simulés', d.rounds.toLocaleString('fr-FR'), ''),
      metric('Durée', d.seconds.toFixed(2), 's'),
      metric('Blackjacks joueur', d.playerBJ.toFixed(3), '%'),
      metric('Croupier a joué', d.dealerPlayed.toFixed(2), '% des coups'),
      metric('Croupier sauté', d.dealerBust.toFixed(3), '% des mains jouées'),
      d.sideWagered > 0 ? metric('Avantage paris annexes', d.sideEdge.toFixed(3), '%') : '',
    ].join('');
  } finally {
    btn.disabled = false;
    btn.textContent = 'Lancer la simulation';
  }
}

function metric(k, v, u, hl) {
  return `<div class="metric${hl ? ' hl' : ''}"><div class="k">${k}</div>` +
    `<div class="v">${v} <span class="u">${u}</span></div></div>`;
}

/* ---------------------- Graphique de convergence ---------------------- */

async function drawCurve() {
  const btn = $('#btn-curve');
  btn.disabled = true;
  btn.textContent = 'Calcul…';
  try {
    const pts = await api('/api/curve?rounds=2000000&points=60');
    const cv = $('#chart-curve');
    const g = cv.getContext('2d');
    const W = cv.width, H = cv.height;
    const m = { l: 62, r: 20, t: 18, b: 42 };

    g.clearRect(0, 0, W, H);

    const xs = pts.map((p) => Math.log10(p.rounds));
    const x0 = Math.min(...xs), x1 = Math.max(...xs);
    const band = pts.map((p) => Math.abs(p.edge) + 1.96 * p.stderr);
    const yMax = Math.min(4, Math.max(1.2, Math.max(...band) * 1.1));
    const y0 = -yMax, y1 = yMax;

    const X = (v) => m.l + (v - x0) / (x1 - x0) * (W - m.l - m.r);
    const Y = (v) => m.t + (y1 - v) / (y1 - y0) * (H - m.t - m.b);

    // Grille horizontale
    g.strokeStyle = 'rgba(255,255,255,.08)';
    g.fillStyle = '#93a09b';
    g.font = '11px Segoe UI, sans-serif';
    g.textAlign = 'right';
    for (let v = Math.ceil(y0); v <= y1; v++) {
      g.beginPath(); g.moveTo(m.l, Y(v)); g.lineTo(W - m.r, Y(v)); g.stroke();
      g.fillText(v.toFixed(0) + '%', m.l - 8, Y(v) + 4);
    }

    // Graduations logarithmiques
    g.textAlign = 'center';
    for (let e = Math.ceil(x0); e <= x1; e++) {
      g.strokeStyle = 'rgba(255,255,255,.08)';
      g.beginPath(); g.moveTo(X(e), m.t); g.lineTo(X(e), H - m.b); g.stroke();
      g.fillText('10' + sup(e), X(e), H - m.b + 18);
    }
    g.fillText('coups simulés (échelle logarithmique)', (m.l + W - m.r) / 2, H - 8);

    // Bande de confiance à 95 %
    g.fillStyle = 'rgba(216,178,95,.16)';
    g.beginPath();
    pts.forEach((p, i) => {
      const x = X(Math.log10(p.rounds)), y = Y(p.edge + 1.96 * p.stderr);
      i ? g.lineTo(x, y) : g.moveTo(x, y);
    });
    for (let i = pts.length - 1; i >= 0; i--) {
      const p = pts[i];
      g.lineTo(X(Math.log10(p.rounds)), Y(p.edge - 1.96 * p.stderr));
    }
    g.closePath(); g.fill();

    // Valeur publiée
    g.strokeStyle = '#4fd48a';
    g.setLineDash([5, 4]);
    g.beginPath(); g.moveTo(m.l, Y(0.35)); g.lineTo(W - m.r, Y(0.35)); g.stroke();
    g.setLineDash([]);
    g.fillStyle = '#4fd48a';
    g.textAlign = 'left';
    g.fillText('valeur publiée  0,35 %', m.l + 8, Y(0.35) - 7);

    // Courbe mesurée
    g.strokeStyle = '#d8b25f';
    g.lineWidth = 2;
    g.beginPath();
    pts.forEach((p, i) => {
      const x = X(Math.log10(p.rounds)), y = Y(p.edge);
      i ? g.lineTo(x, y) : g.moveTo(x, y);
    });
    g.stroke();
    g.lineWidth = 1;

    const last = pts[pts.length - 1];
    g.fillStyle = '#e8eceb';
    g.textAlign = 'right';
    g.fillText(`${last.edge.toFixed(3)} % ± ${(1.96 * last.stderr).toFixed(3)}`,
      W - m.r - 6, Y(last.edge) - 9);
  } finally {
    btn.disabled = false;
    btn.textContent = 'Tracer la convergence';
  }
}

function sup(n) {
  const map = { '-': '⁻', 0: '⁰', 1: '¹', 2: '²', 3: '³',
    4: '⁴', 5: '⁵', 6: '⁶', 7: '⁷', 8: '⁸', 9: '⁹' };
  return String(n).split('').map((c) => map[c] || c).join('');
}

/* ---------------------- Récit d'un coup ---------------------- */

async function drawSample() {
  const q = new URLSearchParams({ sidebets: $('#log-side').checked });
  const d = await api('/api/sample-round?' + q);
  const box = $('#narration');
  box.innerHTML = '';
  (d.log || []).forEach((line) => {
    const el = document.createElement('div');
    el.className = 'line';
    el.textContent = line;
    box.appendChild(el);
  });
  if (!d.log || !d.log.length) toast('Aucun récit renvoyé.');
}

/* ---------------------- Comparaison des règles ---------------------- */

async function drawRules() {
  const btn = $('#btn-rules');
  btn.disabled = true;
  btn.textContent = 'Calcul…';
  try {
    const rows = await api('/api/rules-comparison?rounds=500000');
    const cv = $('#chart-rules');
    const g = cv.getContext('2d');
    const W = cv.width, H = cv.height;
    const m = { l: 232, r: 90, t: 16, b: 38 };

    g.clearRect(0, 0, W, H);

    const vals = rows.map((r) => Math.abs(r.edge) + 1.96 * r.error);
    const xMax = Math.max(1, Math.max(...vals) * 1.15);
    const xMin = Math.min(0, ...rows.map((r) => r.edge - 1.96 * r.error)) * 1.15;

    const X = (v) => m.l + (v - xMin) / (xMax - xMin) * (W - m.l - m.r);
    const rowH = (H - m.t - m.b) / rows.length;

    g.font = '12px Segoe UI, sans-serif';

    // Axe vertical à zéro
    g.strokeStyle = 'rgba(255,255,255,.22)';
    g.beginPath(); g.moveTo(X(0), m.t); g.lineTo(X(0), H - m.b); g.stroke();

    // Graduations
    g.fillStyle = '#93a09b';
    g.textAlign = 'center';
    const step = xMax > 2 ? 0.5 : 0.25;
    for (let v = Math.ceil(xMin / step) * step; v <= xMax; v += step) {
      g.strokeStyle = 'rgba(255,255,255,.07)';
      g.beginPath(); g.moveTo(X(v), m.t); g.lineTo(X(v), H - m.b); g.stroke();
      g.fillText(v.toFixed(2) + '%', X(v), H - m.b + 17);
    }
    g.fillText("avantage de la maison (%)", (m.l + W - m.r) / 2, H - 6);

    rows.forEach((r, i) => {
      const y = m.t + i * rowH + rowH / 2;
      const isRef = i === 0;

      g.fillStyle = isRef ? '#e8eceb' : '#c3cdc9';
      g.textAlign = 'right';
      g.fillText(r.label, m.l - 12, y + 4);

      const h = Math.min(20, rowH * 0.5);
      g.fillStyle = isRef ? '#d8b25f' : (r.edge > rows[0].edge ? '#ef6060' : '#4fd48a');
      const xa = X(Math.min(0, r.edge)), xb = X(Math.max(0, r.edge));
      g.fillRect(xa, y - h / 2, Math.max(1, xb - xa), h);

      // Barre d'erreur
      const e = 1.96 * r.error;
      g.strokeStyle = 'rgba(255,255,255,.62)';
      g.beginPath();
      g.moveTo(X(r.edge - e), y); g.lineTo(X(r.edge + e), y);
      g.moveTo(X(r.edge - e), y - 5); g.lineTo(X(r.edge - e), y + 5);
      g.moveTo(X(r.edge + e), y - 5); g.lineTo(X(r.edge + e), y + 5);
      g.stroke();

      g.fillStyle = '#e8eceb';
      g.textAlign = 'left';
      g.fillText(r.edge.toFixed(3) + ' %', X(r.edge + e) + 9, y + 4);
    });
  } finally {
    btn.disabled = false;
    btn.textContent = 'Comparer les règles';
  }
}

/* ---------------------- Démarrage ---------------------- */

function wire() {
  buildChips();
  wireSpots();
  wireTabs();

  $('#btn-deal').onclick = () => guard(deal);
  $('#btn-clear').onclick = () => {
    Object.keys(pending).forEach((k) => (pending[k] = 0));
    renderControls();
  };
  $('#btn-rebet').onclick = () => {
    if (!lastBets) return;
    pending = { ...lastBets };
    renderControls();
  };
  $('#btn-reset').onclick = () => guard(async () => {
    state = await api('/api/reset', {});
    Object.keys(pending).forEach((k) => (pending[k] = 0));
    render();
  });

  $$('.actions button').forEach((b) => {
    b.onclick = () => guard(() => act(b.dataset.action));
  });

  $('#hint-toggle').onchange = renderControls;
  $('#btn-sim').onclick = () => guard(runSim);
  $('#btn-curve').onclick = () => guard(drawCurve);
  $('#btn-rules').onclick = () => guard(drawRules);
  $('#btn-sample').onclick = () => guard(drawSample);

  document.addEventListener('keydown', (e) => {
    if (e.target.tagName === 'INPUT' || e.target.tagName === 'SELECT') return;
    if (e.key === 'Enter' && !$('#betting-ui').hidden && !$('#btn-deal').disabled) {
      guard(deal);
      return;
    }
    const a = KEYS[e.key.toLowerCase()];
    if (a && state && (state.legalActions || []).includes(a)) guard(() => act(a));
  });
}

(async function start() {
  wire();
  try {
    state = await api('/api/state');
    pending.main = 10;
    render();
  } catch (e) {
    toast('Serveur injoignable : ' + e.message);
  }
})();
