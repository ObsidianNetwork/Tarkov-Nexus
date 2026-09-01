package overlay

// proxyInjectScript is injected into tarkov.dev's HTML by the local proxy.
// It polls for Leaflet (which tarkov.dev loads async), then connects to the
// overlay WebSocket server and overlays party member markers on the map.
const proxyInjectScript = `
(function () {
  if (window._nexusPartyInjected) return;
  window._nexusPartyInjected = true;

  // ── Compact chrome for the overlay window ──────────────────────────────
  //
  // This window is a small always-on-top map, so the site's top navigation is
  // hidden to give the map the whole viewport. That is the only cosmetic change
  // made, and it is purely visual.
  //
  // Three things this used to do and deliberately no longer does:
  //   * set tarkov.dev's CookieConsent cookie, answering their consent prompt
  //     on the user's behalf without the user ever being asked;
  //   * hide the site's own branding and session widget (.id-wrapper);
  //   * programmatically click their map filter checkboxes to uncheck
  //     "Loose Loot", "Lootable Items", "Tasks" and "Usable".
  // None of that is ours to decide on someone else's site. The consent banner
  // is shown normally and the user's answer persists in the window's own
  // profile directory.
  (function () {
    var s = document.createElement('style');
    s.textContent = '.navigation { display: none !important; }' +
                    'body { padding-top: 0 !important; }';
    (document.head || document.documentElement).appendChild(s);
  })();

  // ── Capture the session ID ───────────────────────────────────────────
  // Waits for React to initialise, reads the active session ID from the
  // Remote Control widget in the DOM, and reports it back to the app so the
  // two halves agree on which tarkov.dev session to drive.
  //
  // This no longer writes socketEnabled into tarkov.dev's localStorage; their
  // settings are not ours to change. When no ?connection= parameter was
  // supplied the widget is clicked once to establish the pairing the user
  // opened this window for.
  (function reportSessionId() {
    var hasConnectionParam = /[?&]connection=/.test(window.location.search);

    // If no ?connection= param, auto-click "Click to connect" once it appears.
    if (!hasConnectionParam) {
      var clickPoll = setInterval(function () {
        var label = document.querySelector('.update-label');
        if (label && label.textContent && /Click/i.test(label.textContent)) {
          var wrapper = label.closest('.id-wrapper') || label.parentElement;
          if (wrapper) {
            clearInterval(clickPoll);
            wrapper.click();
            console.log('[Nexus] Auto-clicked "Click to connect"');
          }
        }
      }, 300);
    }

    // Capture the active session ID from the DOM once the socket connects.
    var sent = false;
    var idPoll = setInterval(function () {
      if (sent) { clearInterval(idPoll); return; }
      var el = document.querySelector('.session-id');
      if (!el) return;
      var id = (el.textContent || '').trim();
      if (id && id.length >= 2 && id.length <= 10 && !/click|connect/i.test(id)) {
        sent = true;
        clearInterval(idPoll);
        console.log('[Nexus] Captured Remote ID from map:', id);
        fetch('/nexus/capture-id', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ sessionId: id })
        }).catch(function () {});
      }
    }, 500);
  })();

  var mapInstance = null;
  var partyLayer  = null;
  var members     = {};
  var ws          = null;
  var WS_URL      = 'ws://localhost:44444/ws';

  // Poll until Leaflet is ready (tarkov.dev loads it asynchronously)
  var poll = setInterval(function () {
    if (!window.L || !window.L.Map) return;
    clearInterval(poll);
    hookLeaflet();
    connectWS();
  }, 100);

  function hookLeaflet() {
    // Use Leaflet's documented extension point rather than replacing
    // window.L.Map. The old approach swapped in a plain function, copied the
    // prototype and static properties by hand, and wrapped L.map and
    // L.Layer.prototype.addTo as well — which breaks instanceof, loses
    // anything non-enumerable, and is a plausible source of client-side errors
    // on a page we do not own. addInitHook is the API Leaflet provides for
    // exactly this and leaves their classes untouched.
    window.L.Map.addInitHook(function () {
      captureMap(this);
    });

    // A map may already exist if Leaflet initialised before this script ran.
    if (window.map && window.map.getContainer) {
      captureMap(window.map);
      return;
    }

    // addInitHook only fires for maps constructed after this point, and
    // tarkov.dev keeps its map in a React ref rather than on window — so on a
    // fast load the map can already exist and we would never see it.
    //
    // Wrap addTo as a late-capture path: every live map keeps getting layers
    // added to it, so the next one reveals the instance. The wrapper calls
    // straight through, restores the original before doing anything else (which
    // also avoids recursing when captureMap adds our own layer), and is gone
    // after a single successful capture — their Leaflet is left as we found it.
    var origAddTo = window.L.Layer.prototype.addTo;
    window.L.Layer.prototype.addTo = function (target) {
      var result = origAddTo.call(this, target);
      if (mapInstance) {
        // addInitHook got there first — uninstall without capturing. Without
        // this branch the wrapper stays on their prototype for the life of the
        // page, which is the permanent patch we set out to avoid.
        window.L.Layer.prototype.addTo = origAddTo;
      } else if (target && typeof target.getContainer === 'function') {
        window.L.Layer.prototype.addTo = origAddTo;
        captureMap(target);
      }
      return result;
    };
  }

  function captureMap(m) {
    if (mapInstance) return;
    mapInstance = m;
    partyLayer = window.L.layerGroup().addTo(m);
  }

  // ── WebSocket ──────────────────────────────────────────────────────────

  function connectWS() {
    ws = new WebSocket(WS_URL);
    ws.onopen    = function () {};
    ws.onclose   = function () { setTimeout(connectWS, 5000); };
    ws.onerror   = function () { ws.close(); };
    ws.onmessage = function (evt) {
      try { handle(JSON.parse(evt.data)); } catch (e) {}
    };
  }

  function handle(msg) {
    if (msg.type === 'partyPosition' && msg.data) {
      updateMember(msg.data);
    } else if (msg.type === 'partyMemberLeft' && msg.data) {
      removeMember(msg.data.remoteId);
    } else if (msg.type === 'fullState') {
      if (msg.partyPositions) {
        var p = msg.partyPositions;
        for (var id in p) updateMember(p[id]);
      }
    }
  }

  // ── Markers ────────────────────────────────────────────────────────────

  function currentMap() {
    var parts = window.location.pathname.split('/');
    return (parts.length > 2 && parts[1] === 'map') ? parts[2] : null;
  }

  function updateMember(data) {
    if (!mapInstance || !window.L) return;
    var id   = data.remoteId;
    var name = data.displayName || 'Player';
    var pos  = data.position;
    var rot  = data.rotation || 0;
    var map  = data.map;

    var cur = currentMap();
    if (cur && map && map !== cur) { removeMember(id); return; }
    if (!pos) return;

    if (!members[id]) createMarker(id, name);
    var entry = members[id];
    if (!entry) return;

    entry.marker.setLatLng([pos.z, pos.x]);

    if (entry.imgEl) {
      var mapRot = 0;
      if (mapInstance.options && mapInstance.options.baseData &&
          mapInstance.options.baseData.coordinateRotation) {
        mapRot = mapInstance.options.baseData.coordinateRotation;
      }
      entry.imgEl.style.transform = 'rotate(' + (rot + mapRot) + 'deg)';
    }
  }

  function createMarker(id, name) {
    var label = name.replace(/</g, '&lt;').replace(/>/g, '&gt;');
    var html =
      '<div style="position:relative;width:28px;height:28px;">' +
        '<div style="position:absolute;top:-20px;left:50%;transform:translateX(-50%);' +
             'white-space:nowrap;background:rgba(0,0,0,0.8);color:#4ade80;' +
             'padding:2px 6px;border-radius:4px;font-size:11px;font-weight:700;' +
             'border:1px solid #166534;font-family:sans-serif;">' + label + '</div>' +
        '<img id="nexus-arrow-' + id.replace(/\W/g,'_') + '" ' +
             'src="/maps/interactive/player-position.png" ' +
             'style="width:28px;height:28px;filter:hue-rotate(120deg);"/>' +
      '</div>';

    var icon = window.L.divIcon({
      className: 'nexus-party-marker',
      html: html,
      iconSize: [28, 28],
      iconAnchor: [14, 14],
    });

    var marker = window.L.marker([0, 0], {
      icon: icon,
      zIndexOffset: 900,
      title: name,
    });

    if (partyLayer) marker.addTo(partyLayer);
    else marker.addTo(mapInstance);

    marker.bindPopup('<b>' + label + '</b>');

    var imgEl = null;
    requestAnimationFrame(function () {
      imgEl = document.getElementById('nexus-arrow-' + id.replace(/\W/g,'_'));
      if (members[id]) members[id].imgEl = imgEl;
    });

    members[id] = { marker: marker, imgEl: imgEl };
  }

  function removeMember(id) {
    var e = members[id];
    if (!e) return;
    if (partyLayer) partyLayer.removeLayer(e.marker);
    else if (mapInstance) mapInstance.removeLayer(e.marker);
    delete members[id];
  }
})();
`
