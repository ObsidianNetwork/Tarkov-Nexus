package overlay

// mapPageHTML is the self-contained HTML for the party map viewer window.
// Served at GET /map by the overlay HTTP server.
const mapPageHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Tarkov Party Map – Tarkov Nexus</title>
  <link rel="stylesheet" href="https://unpkg.com/leaflet@1.9.4/dist/leaflet.css" crossorigin=""/>
  <script src="https://unpkg.com/leaflet@1.9.4/dist/leaflet.js" crossorigin=""></script>
  <style>
    *, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }

    body {
      background: #0c0e14;
      color: #e5e7eb;
      font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', system-ui, sans-serif;
      overflow: hidden;
      height: 100vh;
    }

    #map {
      width: 100vw;
      height: 100vh;
      background: #111827;
    }

    /* ---- Toolbar ---- */
    #toolbar {
      position: fixed;
      top: 12px;
      left: 50%;
      transform: translateX(-50%);
      z-index: 1000;
      background: rgba(12, 14, 20, 0.96);
      border: 1px solid #374151;
      border-radius: 10px;
      padding: 8px 16px;
      display: flex;
      gap: 12px;
      align-items: center;
      backdrop-filter: blur(12px);
      box-shadow: 0 4px 24px rgba(0,0,0,0.6);
      white-space: nowrap;
    }

    .toolbar-logo {
      color: #facc15;
      font-weight: 800;
      font-size: 13px;
      letter-spacing: 0.5px;
      text-transform: uppercase;
    }

    .toolbar-sep {
      width: 1px;
      height: 20px;
      background: #374151;
    }

    #mapSelect {
      background: #1f2937;
      border: 1px solid #374151;
      color: #e5e7eb;
      border-radius: 6px;
      padding: 4px 28px 4px 10px;
      font-size: 13px;
      cursor: pointer;
      appearance: none;
      background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='16' height='16' fill='%236b7280' viewBox='0 0 16 16'%3E%3Cpath d='M7.247 11.14L2.451 5.658C1.885 5.013 2.345 4 3.204 4h9.592a1 1 0 0 1 .753 1.659l-4.796 5.48a1 1 0 0 1-1.506 0z'/%3E%3C/svg%3E");
      background-repeat: no-repeat;
      background-position: right 8px center;
    }
    #mapSelect:focus { outline: none; border-color: #facc15; }

    .party-badge {
      background: #052e16;
      color: #4ade80;
      border: 1px solid #166534;
      border-radius: 20px;
      padding: 3px 12px;
      font-size: 12px;
      font-weight: 700;
      min-width: 70px;
      text-align: center;
    }

    .party-badge.offline {
      background: #1f2937;
      color: #6b7280;
      border-color: #374151;
    }

    /* ---- Status bar ---- */
    #status-bar {
      position: fixed;
      bottom: 12px;
      right: 12px;
      z-index: 1000;
      background: rgba(12, 14, 20, 0.92);
      border: 1px solid #374151;
      border-radius: 8px;
      padding: 6px 14px;
      font-size: 12px;
      display: flex;
      align-items: center;
      gap: 8px;
    }

    .status-dot {
      width: 8px;
      height: 8px;
      border-radius: 50%;
      background: #ef4444;
      flex-shrink: 0;
    }
    .status-dot.connected { background: #22c55e; animation: pulse-dot 2s infinite; }

    @keyframes pulse-dot {
      0%, 100% { opacity: 1; }
      50% { opacity: 0.4; }
    }

    /* ---- Coordinate info ---- */
    #coord-info {
      position: fixed;
      bottom: 12px;
      left: 12px;
      z-index: 1000;
      background: rgba(12, 14, 20, 0.85);
      border: 1px solid #374151;
      border-radius: 8px;
      padding: 5px 12px;
      font-size: 11px;
      color: #6b7280;
      font-variant-numeric: tabular-nums;
    }

    /* ---- Zoom controls override ---- */
    .leaflet-control-zoom {
      border: 1px solid #374151 !important;
      border-radius: 8px !important;
      overflow: hidden !important;
      box-shadow: 0 2px 8px rgba(0,0,0,0.4) !important;
    }
    .leaflet-control-zoom a {
      background: #1f2937 !important;
      color: #e5e7eb !important;
      border-bottom: 1px solid #374151 !important;
      font-size: 16px !important;
      line-height: 28px !important;
      width: 28px !important;
      height: 28px !important;
    }
    .leaflet-control-zoom a:hover { background: #374151 !important; color: #facc15 !important; }
    .leaflet-control-zoom-in { border-radius: 7px 7px 0 0 !important; }
    .leaflet-control-zoom-out { border-radius: 0 0 7px 7px !important; border-bottom: none !important; }

    .leaflet-container { background: #111827 !important; cursor: crosshair !important; }
    .leaflet-control-attribution { display: none !important; }

    /* ---- Party markers ---- */
    .pm-wrap {
      position: relative;
      width: 32px;
      height: 32px;
      pointer-events: auto;
    }
    .pm-circle {
      width: 32px;
      height: 32px;
      border-radius: 50%;
      background: radial-gradient(circle at 40% 35%, #34d399, #059669 70%, #047857);
      border: 2px solid #6ee7b7;
      display: flex;
      align-items: center;
      justify-content: center;
      box-shadow: 0 0 10px rgba(52, 211, 153, 0.45), inset 0 1px 0 rgba(255,255,255,0.15);
      transition: transform 0.15s;
    }
    .pm-circle:hover { transform: scale(1.15); }

    .pm-arrow {
      width: 0;
      height: 0;
      border-left: 5px solid transparent;
      border-right: 5px solid transparent;
      border-bottom: 10px solid rgba(255,255,255,0.9);
      transform-origin: 5px 10px;
      transition: transform 0.2s;
    }

    .pm-name {
      position: absolute;
      top: -22px;
      left: 50%;
      transform: translateX(-50%);
      white-space: nowrap;
      background: rgba(0,0,0,0.82);
      color: #4ade80;
      padding: 2px 7px;
      border-radius: 4px;
      font-size: 11px;
      font-weight: 700;
      border: 1px solid #059669;
      pointer-events: none;
      letter-spacing: 0.3px;
    }

    /* Self marker – golden */
    .pm-self .pm-circle {
      background: radial-gradient(circle at 40% 35%, #fde68a, #d97706 70%, #b45309);
      border-color: #fbbf24;
      box-shadow: 0 0 10px rgba(251, 191, 36, 0.45), inset 0 1px 0 rgba(255,255,255,0.15);
    }
    .pm-self .pm-name { color: #fbbf24; border-color: #d97706; }

    /* ---- Grid lines ---- */
    .grid-line { pointer-events: none; }

    /* ---- Loading overlay ---- */
    #loading {
      position: fixed;
      inset: 0;
      background: #0c0e14;
      display: flex;
      flex-direction: column;
      align-items: center;
      justify-content: center;
      gap: 14px;
      z-index: 5000;
    }
    #loading.hidden { display: none; }
    .spin {
      width: 38px;
      height: 38px;
      border: 3px solid #1f2937;
      border-top-color: #facc15;
      border-radius: 50%;
      animation: spin 0.9s linear infinite;
    }
    @keyframes spin { to { transform: rotate(360deg); } }
    .load-text { color: #6b7280; font-size: 13px; }

    /* ---- Fit button ---- */
    #btn-fit {
      position: fixed;
      top: 60px;
      right: 12px;
      z-index: 1000;
      background: #1f2937;
      border: 1px solid #374151;
      border-radius: 7px;
      color: #9ca3af;
      padding: 6px 10px;
      font-size: 12px;
      cursor: pointer;
      transition: background 0.15s, color 0.15s;
    }
    #btn-fit:hover { background: #374151; color: #e5e7eb; }
  </style>
</head>
<body>

<div id="loading">
  <div class="spin"></div>
  <div class="load-text">Loading Tarkov Map…</div>
</div>

<div id="toolbar">
  <span class="toolbar-logo">⚔ Tarkov Party Map</span>
  <div class="toolbar-sep"></div>
  <select id="mapSelect">
    <option value="">— Select Map —</option>
    <option value="customs">Customs</option>
    <option value="woods">Woods</option>
    <option value="shoreline">Shoreline</option>
    <option value="interchange">Interchange</option>
    <option value="reserve">Reserve</option>
    <option value="lighthouse">Lighthouse</option>
    <option value="streets-of-tarkov">Streets of Tarkov</option>
    <option value="factory">Factory (Day)</option>
    <option value="night-factory">Factory (Night)</option>
    <option value="the-lab">The Lab</option>
    <option value="ground-zero">Ground Zero</option>
  </select>
  <div class="toolbar-sep"></div>
  <span class="party-badge offline" id="partyBadge">0 online</span>
</div>

<button id="btn-fit" title="Fit map to view (F)">⊡ Fit</button>

<div id="map"></div>

<div id="coord-info" id="coord-info">X: — &nbsp; Z: —</div>

<div id="status-bar">
  <div class="status-dot" id="statusDot"></div>
  <span id="statusText">Connecting to Tarkov Nexus…</span>
</div>

<script>
'use strict';

// ── Configuration ──────────────────────────────────────────────────────────

const WS_URL   = 'ws://localhost:44444/ws';
const API_URL  = 'http://localhost:44444/nexus/state';

// Map configs: bounds = [[minZ, minX], [maxZ, maxX]] in game-world coords.
// Leaflet CRS.Simple maps lat→Z, lng→X from game coordinates.
const MAPS = {
  customs: {
    name: 'Customs',
    bounds: [[-355, -60], [255, 480]],
    center: [-50, 210],
    zoom: 0,
    rotation: 0,
  },
  woods: {
    name: 'Woods',
    bounds: [[-285, -490], [230, 490]],
    center: [-25, 0],
    zoom: 0,
    rotation: 0,
  },
  shoreline: {
    name: 'Shoreline',
    bounds: [[-425, -265], [410, 625]],
    center: [-5, 180],
    zoom: 0,
    rotation: 0,
  },
  interchange: {
    name: 'Interchange',
    bounds: [[-270, -260], [260, 260]],
    center: [0, 0],
    zoom: 0,
    rotation: 0,
  },
  reserve: {
    name: 'Reserve',
    bounds: [[-275, -240], [245, 260]],
    center: [-15, 10],
    zoom: 0,
    rotation: 0,
  },
  lighthouse: {
    name: 'Lighthouse',
    bounds: [[-440, -315], [405, 375]],
    center: [-15, 30],
    zoom: 0,
    rotation: 0,
  },
  'streets-of-tarkov': {
    name: 'Streets of Tarkov',
    bounds: [[-415, -260], [285, 340]],
    center: [-65, 40],
    zoom: 0,
    rotation: 0,
  },
  factory: {
    name: 'Factory (Day)',
    bounds: [[-125, -130], [125, 130]],
    center: [0, 0],
    zoom: 1,
    rotation: 0,
  },
  'night-factory': {
    name: 'Factory (Night)',
    bounds: [[-125, -130], [125, 130]],
    center: [0, 0],
    zoom: 1,
    rotation: 0,
  },
  'the-lab': {
    name: 'The Lab',
    bounds: [[-115, -120], [120, 120]],
    center: [0, 0],
    zoom: 1,
    rotation: 0,
  },
  'ground-zero': {
    name: 'Ground Zero',
    bounds: [[-245, -190], [205, 190]],
    center: [-20, 0],
    zoom: 1,
    rotation: 0,
  },
};

// ── State ──────────────────────────────────────────────────────────────────

let leaflet       = null;
let partyLayer    = null;
let gridLayer     = null;
let members       = {};   // remoteId → { marker, arrowEl }
let currentMap    = '';
let currentCfg    = null;
let ws            = null;
let connected     = false;
let selfRemoteId  = null; // set from API state if available

// ── Init ───────────────────────────────────────────────────────────────────

window.addEventListener('load', () => {
  initLeaflet();
  loadInitialState();
  connectWS();
  setupKeyboard();
});

function initLeaflet() {
  leaflet = L.map('map', {
    crs: L.CRS.Simple,
    zoomControl: true,
    attributionControl: false,
    minZoom: -3,
    maxZoom: 5,
    zoomSnap: 0.25,
    zoomDelta: 0.5,
  });

  partyLayer = L.layerGroup().addTo(leaflet);
  gridLayer  = L.layerGroup().addTo(leaflet);

  leaflet.on('mousemove', (e) => {
    const ll = e.latlng;
    document.getElementById('coord-info').textContent =
      'X: ' + ll.lng.toFixed(1).padStart(7) + '   Z: ' + ll.lat.toFixed(1).padStart(7);
  });

  document.getElementById('mapSelect').addEventListener('change', (e) => {
    if (e.target.value) loadMap(e.target.value);
  });

  document.getElementById('btn-fit').addEventListener('click', fitMap);

  document.getElementById('loading').classList.add('hidden');
}

// ── Map loading ────────────────────────────────────────────────────────────

function loadMap(mapKey) {
  const cfg = MAPS[mapKey];
  if (!cfg) return;

  currentMap = mapKey;
  currentCfg = cfg;

  document.getElementById('mapSelect').value = mapKey;
  document.title = cfg.name + ' – Tarkov Party Map';

  // Clear grid
  gridLayer.clearLayers();
  drawGrid(cfg.bounds);

  // Try to load map background image from tarkov.dev assets
  tryLoadMapImage(mapKey, cfg.bounds);

  fitMap();

  // Remove markers from members on other maps (keep those on this map)
  Object.keys(members).forEach(id => {
    if (members[id].mapKey && members[id].mapKey !== mapKey) {
      removeMember(id);
    }
  });
}

function tryLoadMapImage(mapKey, bounds) {
  // Try a few known URL patterns for tarkov.dev map images.
  // Falls back gracefully if none load – markers still work correctly.
  const candidates = [
    'https://assets.tarkov.dev/maps/' + mapKey + '/svg',
    'https://assets.tarkov.dev/' + mapKey + '-map.jpg',
    'https://assets.tarkov.dev/' + mapKey + '.jpg',
  ];

  // Remove any existing image overlay
  if (leaflet._mapImageOverlay) {
    leaflet.removeLayer(leaflet._mapImageOverlay);
    leaflet._mapImageOverlay = null;
  }

  tryNextImage(candidates, 0, bounds);
}

function tryNextImage(urls, idx, bounds) {
  if (idx >= urls.length) return; // all failed – coordinate grid is the background

  const img = new Image();
  img.onload = () => {
    // Make sure map hasn't changed while we were loading
    if (MAPS[currentMap] && MAPS[currentMap].bounds === bounds) {
      leaflet._mapImageOverlay = L.imageOverlay(urls[idx], bounds, { opacity: 0.85, zIndex: 1 })
        .addTo(leaflet);
      leaflet._mapImageOverlay.bringToBack();
    }
  };
  img.onerror = () => tryNextImage(urls, idx + 1, bounds);
  img.src = urls[idx];
}

function drawGrid(bounds) {
  const [[minZ, minX], [maxZ, maxX]] = bounds;
  const rangeX = maxX - minX;
  const rangeZ = maxZ - minZ;
  const step   = Math.pow(10, Math.floor(Math.log10(Math.max(rangeX, rangeZ))) - 1);
  const nice   = Math.ceil(step / 50) * 50 || 50;

  const lineStyle = { color: '#1e293b', weight: 1, opacity: 0.6, interactive: false };

  for (let x = Math.ceil(minX / nice) * nice; x <= maxX; x += nice) {
    L.polyline([[minZ, x], [maxZ, x]], lineStyle).addTo(gridLayer);
    if (x === 0) L.polyline([[minZ, x], [maxZ, x]], { ...lineStyle, color: '#334155', weight: 2 }).addTo(gridLayer);
  }
  for (let z = Math.ceil(minZ / nice) * nice; z <= maxZ; z += nice) {
    L.polyline([[z, minX], [z, maxX]], lineStyle).addTo(gridLayer);
    if (z === 0) L.polyline([[z, minX], [z, maxX]], { ...lineStyle, color: '#334155', weight: 2 }).addTo(gridLayer);
  }

  // Axis labels at borders
  for (let x = Math.ceil(minX / (nice * 2)) * (nice * 2); x <= maxX; x += nice * 2) {
    L.marker([minZ, x], {
      icon: L.divIcon({
        html: '<div style="color:#374151;font-size:9px;white-space:nowrap;transform:translateX(-50%);">' + x + '</div>',
        className: '', iconSize: [1, 1], iconAnchor: [0, 0],
      }),
      interactive: false,
    }).addTo(gridLayer);
  }
}

function fitMap() {
  if (!leaflet || !currentCfg) return;
  leaflet.fitBounds(currentCfg.bounds, { padding: [50, 50], animate: true });
}

// ── Party markers ──────────────────────────────────────────────────────────

function updateMember(data) {
  const { remoteId, displayName, position, rotation, map: memberMap } = data;
  if (!remoteId || !position) return;

  // If on a different map, remove them
  if (memberMap && currentMap && memberMap !== currentMap) {
    removeMember(remoteId);
    return;
  }

  const isSelf = (remoteId === selfRemoteId);

  if (!members[remoteId]) {
    createMarker(remoteId, displayName, isSelf);
  }

  const entry = members[remoteId];
  if (!entry) return;

  // Game coords: lat=Z, lng=X
  entry.marker.setLatLng(L.latLng(position.z, position.x));
  entry.mapKey = memberMap || currentMap;

  // Rotate arrow to face direction of travel
  if (entry.arrowEl && typeof rotation === 'number') {
    const mapRot = (currentCfg && currentCfg.rotation) || 0;
    entry.arrowEl.style.transform = 'rotate(' + (rotation + mapRot) + 'deg)';
    entry.arrowEl.style.transformOrigin = '5px 5px';
  }

  updateBadge();
}

function createMarker(remoteId, displayName, isSelf) {
  const safeId   = remoteId.replace(/[^a-zA-Z0-9_-]/g, '_');
  const selfCls  = isSelf ? ' pm-self' : '';
  const safeHtml = escHtml(displayName);

  const html = '<div class="pm-wrap' + selfCls + '">' +
    '<div class="pm-name">' + safeHtml + '</div>' +
    '<div class="pm-circle"><div class="pm-arrow" id="arr-' + safeId + '"></div></div>' +
  '</div>';

  const icon = L.divIcon({ className: 'party-marker', html, iconSize: [32, 32], iconAnchor: [16, 16] });
  const marker = L.marker([0, 0], { icon, zIndexOffset: isSelf ? 1000 : 900, title: displayName })
    .addTo(partyLayer);

  marker.bindPopup('<b>' + safeHtml + '</b>' + (isSelf ? '<br><i style="color:#facc15">You</i>' : ''));

  members[remoteId] = {
    marker,
    arrowEl: null, // resolved on next tick after DOM insertion
    mapKey: currentMap,
  };

  // Arrow element is in the marker's DOM - resolve after Leaflet renders it
  requestAnimationFrame(() => {
    const el = document.getElementById('arr-' + safeId);
    if (el && members[remoteId]) members[remoteId].arrowEl = el;
  });
}

function removeMember(remoteId) {
  const entry = members[remoteId];
  if (!entry) return;
  partyLayer.removeLayer(entry.marker);
  delete members[remoteId];
  updateBadge();
}

function updateBadge() {
  const count = Object.keys(members).length;
  const badge = document.getElementById('partyBadge');
  badge.textContent = count + (count === 1 ? ' online' : ' online');
  badge.className = 'party-badge' + (count > 0 ? '' : ' offline');
}

// ── WebSocket ──────────────────────────────────────────────────────────────

function connectWS() {
  ws = new WebSocket(WS_URL);

  ws.onopen = () => {
    connected = true;
    setStatus(true, 'Connected to Tarkov Nexus');
  };

  ws.onclose = () => {
    connected = false;
    setStatus(false, 'Disconnected – retrying in 5s…');
    setTimeout(connectWS, 5000);
  };

  ws.onerror = () => { ws.close(); };

  ws.onmessage = (evt) => {
    let msg;
    try { msg = JSON.parse(evt.data); } catch { return; }
    handleMessage(msg);
  };
}

function handleMessage(msg) {
  switch (msg.type) {
    case 'partyPosition':
      if (msg.data) updateMember(msg.data);
      break;
    case 'partyMemberLeft':
      if (msg.data && msg.data.remoteId) removeMember(msg.data.remoteId);
      break;
    case 'mapChange':
      if (msg.map && msg.map !== currentMap) loadMap(msg.map);
      break;
    case 'fullState':
      if (msg.currentMap) loadMap(msg.currentMap);
      if (msg.selfRemoteId) selfRemoteId = msg.selfRemoteId;
      if (msg.partyPositions) {
        Object.values(msg.partyPositions).forEach(p => updateMember(p));
      }
      break;
  }
}

// ── Initial state fetch ────────────────────────────────────────────────────

function loadInitialState() {
  fetch(API_URL)
    .then(r => r.json())
    .then(state => {
      if (state.selfRemoteId) selfRemoteId = state.selfRemoteId;
      const map = state.currentMap || 'customs';
      loadMap(map);
      if (state.partyPositions) {
        Object.values(state.partyPositions).forEach(p => updateMember(p));
      }
    })
    .catch(() => {
      loadMap('customs');
    });
}

// ── Status bar ─────────────────────────────────────────────────────────────

function setStatus(ok, text) {
  const dot = document.getElementById('statusDot');
  dot.className = 'status-dot' + (ok ? ' connected' : '');
  document.getElementById('statusText').textContent = text;
}

// ── Keyboard shortcuts ─────────────────────────────────────────────────────

function setupKeyboard() {
  document.addEventListener('keydown', (e) => {
    if (e.target.tagName === 'INPUT' || e.target.tagName === 'SELECT') return;
    switch (e.key) {
      case '+': case '=': leaflet && leaflet.zoomIn(0.5); break;
      case '-':           leaflet && leaflet.zoomOut(0.5); break;
      case 'f': case 'F': fitMap(); break;
    }
  });
  // Mouse wheel zoom is already handled by Leaflet
}

// ── Util ───────────────────────────────────────────────────────────────────

function escHtml(str) {
  return String(str)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}
</script>
</body>
</html>`
