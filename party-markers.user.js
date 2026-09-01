// ==UserScript==
// @name         Tarkov.dev Party Markers (Local Overlay)
// @namespace    http://tampermonkey.net/
// @version      2.4
// @description  Shows party members on tarkov.dev map via local overlay server
// @author       You
// @match        https://tarkov.dev/*
// @icon         https://www.google.com/s2/favicons?sz=64&domain=tarkov.dev
// @run-at       document-start
// @grant        none
// ==/UserScript==

(function () {
    'use strict';

    console.log("[Overlay] Party Markers script loaded");

    let mapInstance = null;
    let partyLayer = null;
    const partyMembers = {};
    const OVERLAY_WS_URL = 'ws://localhost:44444';
    let overlayWs = null;

    // Hook into Leaflet to get the map instance
    const checkLeaflet = setInterval(() => {
        if (window.L && window.L.Map) {
            clearInterval(checkLeaflet);
            console.log("[Overlay] Leaflet found, hooking Map constructor");

            // Use Leaflet's documented extension point instead of replacing
            // window.L.Map. Swapping the constructor out, reassigning its
            // prototype and copying statics by hand breaks instanceof, drops
            // non-enumerable properties, and wraps L.map and
            // L.Layer.prototype.addTo as well — a lot of surface area to break
            // on a page we do not own. addInitHook is what Leaflet provides for
            // this and leaves their classes intact.
            window.L.Map.addInitHook(function () {
                mapInstance = this;
                console.log("[Overlay] Map instance captured via addInitHook");
                initPartyLayer(this);
            });

            // A map may already exist if Leaflet initialised before this ran.
            if (window.map && window.map instanceof window.L.Map) {
                mapInstance = window.map;
                console.log("[Overlay] Map instance found in window.map");
                initPartyLayer(mapInstance);
            } else {
                // addInitHook only fires for maps constructed after this point,
                // and tarkov.dev keeps its map in a React ref rather than on
                // window. Wrapping addTo catches the next layer added to a live
                // map, restores the original before capturing (so adding our own
                // layer does not recurse), and is gone after one capture.
                const origAddTo = window.L.Layer.prototype.addTo;
                window.L.Layer.prototype.addTo = function (target) {
                    const result = origAddTo.call(this, target);
                    if (mapInstance) {
                        // addInitHook got there first — uninstall without
                        // capturing, so the wrapper does not outlive its purpose.
                        window.L.Layer.prototype.addTo = origAddTo;
                    } else if (target && typeof target.getContainer === 'function') {
                        window.L.Layer.prototype.addTo = origAddTo;
                        mapInstance = target;
                        console.log("[Overlay] Map instance captured via addTo fallback");
                        initPartyLayer(target);
                    }
                    return result;
                };
            }

            // Start connecting to overlay server once Leaflet is ready
            connectOverlay();
        }
    }, 10); // Check every 10ms to catch it early

    function initPartyLayer(map) {
        if (partyLayer) return; // Already initialized

        console.log("[Overlay] Initializing party layer");
        partyLayer = window.L.layerGroup().addTo(map);

        // Add to layer control if it exists
        if (map.layerControl) {
            map.layerControl.addOverlay(partyLayer, "Party Members");
        } else {
            // Watch for layer control - wait for body to be ready
            const watchForLayerControl = () => {
                const observer = new MutationObserver(() => {
                    if (map.layerControl) {
                        map.layerControl.addOverlay(partyLayer, "Party Members");
                        observer.disconnect();
                    }
                });
                observer.observe(document.body, { childList: true, subtree: true });
            };

            if (document.body) {
                watchForLayerControl();
            } else {
                document.addEventListener('DOMContentLoaded', watchForLayerControl);
            }
        }
    }


    // Debug UI
    function createDebugUI() {
        // Safety check: ensure document.body exists
        if (!document.body) {
            console.warn('[Overlay] Cannot create debug UI - document.body not ready');
            return;
        }

        const debugDiv = document.createElement('div');
        debugDiv.style.cssText = 'position: fixed; bottom: 10px; right: 10px; background: rgba(0,0,0,0.8); color: white; padding: 10px; z-index: 9999; font-family: monospace; font-size: 12px; pointer-events: none;';
        debugDiv.id = 'overlay-debug';
        debugDiv.innerHTML = `
            <div>Overlay Status: <span id="overlay-status" style="color: red">Disconnected</span></div>
            <div>Map: <span id="overlay-map">Unknown</span></div>
            <div>Markers: <span id="overlay-markers">0</span></div>
            <div id="overlay-log" style="max-height: 100px; overflow-y: auto; margin-top: 5px; border-top: 1px solid #555;"></div>
        `;
        document.body.appendChild(debugDiv);

        // Test Button (clickable)
        const btn = document.createElement('button');
        btn.innerText = "Test Marker";
        btn.style.cssText = 'position: fixed; bottom: 10px; right: 200px; z-index: 9999; pointer-events: auto;';
        btn.onclick = () => {
            const currentMap = getMapFromUrl();
            log(`Testing marker on map: ${currentMap}`);
            handleOverlayMessage({
                type: 'partyPosition',
                data: {
                    remoteId: 'test-user',
                    displayName: 'TEST USER',
                    position: { x: 0, y: 0, z: 0 }, // Center of map usually
                    rotation: 0,
                    map: currentMap
                }
            });
        };
        document.body.appendChild(btn);
    }

    function log(msg) {
        console.log(`[Overlay] ${msg}`);
        const logDiv = document.getElementById('overlay-log');
        if (logDiv) {
            const line = document.createElement('div');
            line.innerText = msg;
            logDiv.appendChild(line);
            logDiv.scrollTop = logDiv.scrollHeight;
        }
    }

    function updateStatus(connected) {
        const el = document.getElementById('overlay-status');
        if (el) {
            el.innerText = connected ? "Connected" : "Disconnected";
            el.style.color = connected ? "lime" : "red";
        }
    }

    function getMapFromUrl() {
        const pathParts = window.location.pathname.split('/');
        const map = pathParts.length > 2 && pathParts[1] === 'map' ? pathParts[2] : null;
        // Debug log for map detection
        if (!map) {
            console.log(`[Overlay] Current map from URL: null (path: ${window.location.pathname})`);
        }
        return map;
    }

    // Initialize Debug UI - wait for DOM to be ready
    function initDebugUI() {
        if (document.body) {
            createDebugUI();
        } else if (document.readyState === 'loading') {
            document.addEventListener('DOMContentLoaded', () => {
                // Double-check body exists after DOMContentLoaded
                if (document.body) {
                    createDebugUI();
                } else {
                    console.warn('[Overlay] DOMContentLoaded fired but document.body still null');
                }
            });
        } else {
            // DOM is interactive/complete but body somehow missing - poll for it
            const waitForBody = setInterval(() => {
                if (document.body) {
                    clearInterval(waitForBody);
                    createDebugUI();
                }
            }, 50);
            // Give up after 5 seconds
            setTimeout(() => clearInterval(waitForBody), 5000);
        }
    }

    initDebugUI();

    function connectOverlay() {
        log(`Connecting to ${OVERLAY_WS_URL}...`);
        overlayWs = new WebSocket(OVERLAY_WS_URL);

        overlayWs.onopen = () => {
            log("Connected to local overlay server");
            updateStatus(true);
        };

        overlayWs.onmessage = (event) => {
            try {
                const msg = JSON.parse(event.data);
                handleOverlayMessage(msg);
            } catch (e) {
                console.error("[Overlay] Failed to parse message:", e);
            }
        };

        overlayWs.onclose = () => {
            log("Disconnected. Reconnecting in 5s...");
            updateStatus(false);
            setTimeout(connectOverlay, 5000);
        };

        overlayWs.onerror = (err) => {
            console.error("[Overlay] WebSocket error:", err);
            overlayWs.close();
        };
    }

    function handleOverlayMessage(msg) {
        log(`Received: ${msg.type}`);

        if (!mapInstance || !window.L) {
            log("WARN: Map not ready");
            return;
        }

        if (msg.type === 'partyPosition') {
            updateMemberPosition(msg.data);
        } else if (msg.type === 'partyMemberLeft') {
            removeMember(msg.data.remoteId);
        }
    }

    function updateMemberPosition(data) {
        const { remoteId, displayName, position, rotation, map } = data;

        // Check if member is on the same map
        // Get current map from URL (e.g. /map/customs)
        const currentMap = getMapFromUrl();

        const mapEl = document.getElementById('overlay-map');
        if (mapEl) mapEl.innerText = currentMap || "None";

        if (!currentMap || currentMap !== map) {
            log(`Map mismatch: currentMap="${currentMap}" != msgMap="${map}" for ${displayName}`);
            // Member is on a different map, remove them if they exist
            if (partyMembers[remoteId]) {
                removeMember(remoteId);
            }
            return;
        }

        if (!partyMembers[remoteId]) {
            log(`Creating marker for ${displayName}`);
            createMemberMarker(remoteId, displayName);
        } else {
            // Log position updates for existing markers (only every ~10 updates to avoid spam)
            if (Math.random() < 0.1) {
                log(`Updating ${displayName} at [${position.x.toFixed(1)}, ${position.y.toFixed(1)}, ${position.z.toFixed(1)}]`);
            }
        }

        const member = partyMembers[remoteId];
        // Leaflet uses [lat, lng]. Tarkov coords are [x, y, z].
        // Tarkov.dev maps usually map Z to Lat (y) and X to Lng (x).
        const latLng = [position.z, position.x];

        member.marker.setLatLng(latLng);

        // Update rotation
        if (member.marker._icon) {
            const iconImg = member.marker._icon.querySelector('img');
            if (iconImg) {
                // Adjust rotation based on map rotation
                let mapRotation = 0;
                if (mapInstance.options.baseData && mapInstance.options.baseData.coordinateRotation) {
                    mapRotation = mapInstance.options.baseData.coordinateRotation;
                }
                // Some maps have 180 rotation in config
                const finalRotation = (rotation || 0) + mapRotation;
                iconImg.style.transform = `rotate(${finalRotation}deg)`;
            }
        }

        // Update debug count
        const countEl = document.getElementById('overlay-markers');
        if (countEl) countEl.innerText = Object.keys(partyMembers).length;
    }

    function createMemberMarker(remoteId, displayName) {
        // Create a custom icon similar to the player one
        const iconHtml = `
            <div style="position: relative; width: 24px; height: 24px;">
                <img src="/maps/interactive/player-position.png" style="width: 24px; height: 24px; filter: hue-rotate(120deg);"/>
                <div style="position: absolute; top: -20px; left: 50%; transform: translateX(-50%); white-space: nowrap; background: rgba(0,0,0,0.7); color: #00ff00; padding: 2px 4px; border-radius: 4px; font-size: 10px;">
                    ${displayName}
                </div>
            </div>
        `;

        const icon = window.L.divIcon({
            className: 'marker party-marker',
            html: iconHtml,
            iconSize: [24, 24],
            iconAnchor: [12, 12],
            popupAnchor: [0, -12],
        });

        const marker = window.L.marker([0, 0], {
            icon: icon,
            zIndexOffset: 900,
            title: displayName
        }).addTo(partyLayer);

        marker.bindPopup(displayName);

        partyMembers[remoteId] = {
            marker: marker,
            displayName: displayName
        };
    }

    function removeMember(remoteId) {
        if (partyMembers[remoteId]) {
            partyLayer.removeLayer(partyMembers[remoteId].marker);
            delete partyMembers[remoteId];
            // Update debug count
            const countEl = document.getElementById('overlay-markers');
            if (countEl) countEl.innerText = Object.keys(partyMembers).length;
        }
    }

})();
