// server browser

var browserServers = [];  // raw list from master server
var browserIconCache = {}; // key -> object URL for icon
var browserIconRequested = {}; // key -> true (pending fetch)
var browserAutoRefreshTimer = null;
var browserPlayerCache = {}; // key -> player count from side-channel
var browserPlayerNames = {}; // key -> array of player names from side-channel
var browserPingCache = {}; // key -> ping ms from side-channel probe
var selectedBrowserKey = null; // key of the last-clicked browser entry
var browserSortAsc = false; // false = descending (default for players)
var _browserRenderTimer = null; // debounce timer for filterBrowserList

// convert base64 to a reusable blob object URL (avoids re-decoding on every render)
var _blobUrlCache = {};
function toBlobUrl(base64, mime) {
    if (!base64) return '';
    if (base64.startsWith('blob:')) return base64; // already converted
    var cacheKey = base64.length + ':' + base64.substring(0, 64) + base64.substring(base64.length - 32);
    if (_blobUrlCache[cacheKey]) return _blobUrlCache[cacheKey];
    try {
        var bin = atob(base64);
        var arr = new Uint8Array(bin.length);
        for (var i = 0; i < bin.length; i++) arr[i] = bin.charCodeAt(i);
        var url = URL.createObjectURL(new Blob([arr], { type: mime || 'image/jpeg' }));
        _blobUrlCache[cacheKey] = url;
        return url;
    } catch (e) { return ''; }
}

// game icon src cache, lazy loaded from hidden dom elements
var GAME_ICON_SRC = {};
function getGameIconSrc(game) {
    if (!GAME_ICON_SRC._init) {
        GAME_ICON_SRC._init = true;
        ['GW1', 'GW2', 'BFN'].forEach(function(g) {
            var el = document.getElementById('iconData' + g);
            if (el) GAME_ICON_SRC[g] = el.getAttribute('src') || '';
        });
    }
    return GAME_ICON_SRC[game] || '';
}

// resolve level id to friendly name
function resolveLevelName(game, levelId) {
    if (!levelId || typeof GAME_DATA === 'undefined') return levelId;
    var gd = GAME_DATA[game];
    if (!gd || !gd.levels) return levelId;
    for (var i = 0; i < gd.levels.length; i++) {
        if (gd.levels[i].id === levelId) return gd.levels[i].name;
    }
    // try partial match (level id might be a substring)
    for (var i = 0; i < gd.levels.length; i++) {
        if (levelId.indexOf(gd.levels[i].id) !== -1 || gd.levels[i].id.indexOf(levelId) !== -1) return gd.levels[i].name;
    }
    return levelId;
}

// resolve mode id to friendly name
function resolveModeName(game, modeId) {
    if (!modeId || typeof GAME_DATA === 'undefined') return modeId;
    var gd = GAME_DATA[game];
    if (!gd || !gd.modes) return modeId;
    for (var i = 0; i < gd.modes.length; i++) {
        if (gd.modes[i].id === modeId) return gd.modes[i].name;
    }
    // prefix match for gw1 variant suffixes (Coop0 -> Coop)
    for (var i = 0; i < gd.modes.length; i++) {
        if (modeId.indexOf(gd.modes[i].id) === 0) return gd.modes[i].name;
    }
    return modeId;
}

function resolveBrowserDisplay(s) {
    var game = s.game || 'GW2';
    var levelName = resolveLevelName(game, s.level);
    var modeName = resolveModeName(game, s.mode);
    var mapBgKey = typeof LEVEL_MAP_BG !== 'undefined' ? LEVEL_MAP_BG[s.level] : null;
    var modeBgKey = typeof MODE_BG !== 'undefined' ? MODE_BG[s.mode] : null;
    // prefix match for mode bg
    if (!modeBgKey && s.mode && typeof MODE_BG !== 'undefined') {
        for (var k in MODE_BG) {
            if (s.mode.indexOf(k) === 0) { modeBgKey = MODE_BG[k]; break; }
        }
    }

    if (game === 'GW2' && s.level && s.mode && typeof GW2_LOADSCREEN_OVERRIDES !== 'undefined') {
        var lsKey = s.level + '+' + s.mode;
        var ls = GW2_LOADSCREEN_OVERRIDES[lsKey];
        if (ls) {
            if (ls.displayName) modeName = ls.displayName;
            if (ls.bg) mapBgKey = ls.bg;
        }
    }
    return { levelName: levelName, modeName: modeName, mapBgKey: mapBgKey, modeBgKey: modeBgKey };
}

function refreshBrowser() {
    send('fetchBrowser', {});
}

function onBrowserList(data) {
    browserServers = data.servers || [];
    // clear live caches so stale entries from dead servers don't persist
    browserPlayerCache = {};
    browserPlayerNames = {};
    filterBrowserList();
    // lazy-fetch icons and ping for live player counts
    for (var i = 0; i < browserServers.length; i++) {
        var s = browserServers[i];
        var key = s.address + ':' + (s.port || 14638);
        if (s.hasIcon && !browserIconCache[key] && !browserIconRequested[key]) {
            browserIconRequested[key] = true;
            send('fetchBrowserIcon', { key: key });
        }
        // ping side-channel for live player count
        var pingData = { address: key, browserPing: true };
        if (s.relayAddress && s.relayKey) {
            pingData.relayAddress = s.relayAddress;
            pingData.relayKey = s.relayKey;
        }
        send('checkServer', pingData);
    }
}

function onBrowserIcon(data) {
    if (!data.key) return;
    delete browserIconRequested[data.key];
    if (data.icon) {
        if (typeof isValidBase64 === 'function' && isValidBase64(data.icon)) {
            browserIconCache[data.key] = typeof base64ToBlobUrl === 'function' ? base64ToBlobUrl(data.icon) : toBlobUrl(data.icon);
        }
        var el = document.querySelector('.browser-entry-icon[data-key="' + CSS.escape(data.key) + '"]');
        if (el && browserIconCache[data.key]) {
            var img = document.createElement('img');
            img.alt = '';
            img.draggable = false;
            img.src = browserIconCache[data.key];
            el.innerHTML = '';
            el.appendChild(img);
        }
    }
}

function filterBrowserList() {
    // debounce rapid calls (e.g. from multiple onServerInfoResult in a row)
    if (_browserRenderTimer) clearTimeout(_browserRenderTimer);
    _browserRenderTimer = setTimeout(_filterBrowserListNow, 100);
}

function _filterBrowserListNow() {
    _browserRenderTimer = null;
    var gameFilter = document.getElementById('browserFilterGame').value;
    var searchTerm = (document.getElementById('browserSearch').value || '').toLowerCase();
    var sortBy = document.getElementById('browserSort').value || 'players';
    var moddedFilter = document.getElementById('browserFilterModded') ? document.getElementById('browserFilterModded').value : '';
    var minPlayersEl = document.getElementById('browserMinPlayers');
    var minPlayers = minPlayersEl ? parseInt(minPlayersEl.value) || 0 : 0;

    var filtered = browserServers.filter(function (s) {
        if (gameFilter && s.game !== gameFilter) return false;
        if (moddedFilter === 'modded' && !s.modded) return false;
        if (moddedFilter === 'vanilla' && s.modded) return false;
        var key = s.address + ':' + (s.port || 14638);
        var playerCount = browserPlayerCache[key] != null ? browserPlayerCache[key] : (s.players || 0);
        if (playerCount < minPlayers) return false;
        if (searchTerm) {
            var motdPlain = typeof stripMotdTags === 'function' ? stripMotdTags(s.motd || '') : (s.motd || '');
            var haystack = (motdPlain + ' ' + (s.address || '') + ' ' + (s.game || '') + ' ' + (s.level || '') + ' ' + (s.mode || '')).toLowerCase();
            if (haystack.indexOf(searchTerm) === -1) return false;
        }
        return true;
    });

    var dir = browserSortAsc ? 1 : -1;
    filtered.sort(function (a, b) {
        // pinned servers always on top
        var aPinned = a.pinned ? 1 : 0;
        var bPinned = b.pinned ? 1 : 0;
        if (aPinned !== bPinned) return bPinned - aPinned;

        var aKey = a.address + ':' + (a.port || 14638);
        var bKey = b.address + ':' + (b.port || 14638);
        if (sortBy === 'players') {
            var aPlayers = browserPlayerCache[aKey] != null ? browserPlayerCache[aKey] : (a.players || 0);
            var bPlayers = browserPlayerCache[bKey] != null ? browserPlayerCache[bKey] : (b.players || 0);
            return (bPlayers - aPlayers) * dir;
        } else if (sortBy === 'name') {
            var aName = typeof stripMotdTags === 'function' ? stripMotdTags(a.motd || '') : (a.motd || '');
            var bName = typeof stripMotdTags === 'function' ? stripMotdTags(b.motd || '') : (b.motd || '');
            return aName.localeCompare(bName) * dir;
        } else if (sortBy === 'game') {
            return (a.game || '').localeCompare(b.game || '') * dir;
        } else if (sortBy === 'ping') {
            var aPing = browserPingCache[aKey] != null ? browserPingCache[aKey] : 9999;
            var bPing = browserPingCache[bKey] != null ? browserPingCache[bKey] : 9999;
            return (aPing - bPing) * dir;
        }
        return 0;
    });

    renderBrowserList(filtered);
}

function renderBrowserList(servers) {
    var container = document.getElementById('browserList');
    var emptyEl = document.getElementById('browserEmpty');

    if (!servers.length) {
        container.querySelectorAll('.browser-entry').forEach(function (e) { e.remove(); });
        if (emptyEl) emptyEl.style.display = 'flex';
        return;
    }

    // detach empty-state element before replacing innerHTML so it survives
    if (emptyEl) emptyEl.remove();
    if (emptyEl) emptyEl.style.display = 'none';

    var html = '';
    for (var i = 0; i < servers.length; i++) {
        var s = servers[i];
        var key = s.address + ':' + (s.port || 14638);
        var motd = s.motd || t('browser.default_server_name');
        var liveCount = browserPlayerCache[key];
        var players = liveCount !== undefined ? liveCount : s.players;
        var safePlayerCount = (players !== undefined && players !== null && isFinite(+players)) ? Math.floor(+players) : '?';
        var safeMaxPlayers = (s.maxPlayers !== undefined && s.maxPlayers !== null && isFinite(+s.maxPlayers)) ? Math.floor(+s.maxPlayers) : '?';
        var playerText = safePlayerCount + '/' + safeMaxPlayers;
        var gameClass = (s.game || 'GW2').toLowerCase().replace(/[^a-z0-9]/g, '');
        var cachedIcon = browserIconCache[key];

        html += '<div class="browser-entry" onclick="onBrowserEntryClick(\'' + escapeJs(key) + '\')" ondblclick="onBrowserEntryDblClick(\'' + escapeJs(key) + '\')">';
        // game background
        initGameBgCache();
        var gameBgUrl = GAME_BG_CACHE[s.game || 'GW2'] ? toBlobUrl(GAME_BG_CACHE[s.game || 'GW2']) : '';
        if (gameBgUrl) html += '<img class="browser-entry-bg" src="' + gameBgUrl + '" alt="" draggable="false">';
        html += '<div class="browser-entry-icon" data-key="' + escapeAttr(key) + '">';
        if (cachedIcon) {
            html += '<img src="' + cachedIcon + '" alt="" draggable="false">';
        } else {
            html += '<svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="var(--accent)" stroke-width="1.5" opacity="0.6"><rect x="2" y="2" width="20" height="20" rx="3" ry="3"/><path d="M8 12h8M12 8v8"/></svg>';
        }
        html += '</div>';
        html += '<div class="browser-entry-info">';
        html += '<div class="browser-entry-title">';
        var iconSrc = getGameIconSrc(s.game || 'GW2');
        html += '<span class="game-pill game-pill-' + gameClass + '">' + (iconSrc ? '<img src="' + iconSrc + '" class="game-pill-icon" alt="" draggable="false">' : escapeHtml(s.game || 'GW2')) + '</span> ';
        html += '<span class="browser-entry-motd motd-rendered">' + (typeof renderMotd === 'function' ? renderMotd(motd) : escapeHtml(motd)) + '</span>';
        if (s.hasPassword) {
            html += ' <span class="browser-lock-badge"><svg class="browser-lock-icon" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="11" width="18" height="11" rx="2" ry="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/></svg> ' + t('browser.locked_badge') + '</span>';
        }
        if (s.vpnType) {
            html += ' <span class="browser-vpn-badge"><svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/></svg> ' + escapeHtml(s.vpnType) + '</span>';
        }
        html += '</div>';
        html += '<div class="browser-entry-meta">';
        var isRelay = s.relayAddress && s.relayKey;
        var displayAddr = isRelay ? 'Relay' : (s.gamePort && s.gamePort !== 25200 ? s.address + ':' + s.gamePort : s.address);
        html += escapeHtml(displayAddr);
        // overlay live level/mode from side-channel
        var live = typeof serverStatusCache !== 'undefined' ? serverStatusCache[key] : null;
        var dispServer = { game: s.game, level: s.level, mode: s.mode };
        if (live) {
            if (live.level) dispServer.level = live.level;
            if (live.mode) dispServer.mode = live.mode;
        }
        var disp = resolveBrowserDisplay(dispServer);
        var metaTags = [];
        if (dispServer.level) {
            var mapBg = disp.mapBgKey && typeof MAP_BG_CACHE !== 'undefined' && MAP_BG_CACHE[disp.mapBgKey] ? toBlobUrl(MAP_BG_CACHE[disp.mapBgKey]) : null;
            var mapPill = '<span class="browser-tag-pill' + (mapBg ? ' has-bg' : '') + '"';
            if (mapBg) mapPill += ' style="background-image:url(' + mapBg + ')"';
            mapPill += '>' + escapeHtml(disp.levelName) + '</span>';
            metaTags.push(mapPill);
            if (disp.mapBgKey && typeof MAP_BG_CACHE !== 'undefined' && MAP_BG_CACHE[disp.mapBgKey] === undefined) {
                MAP_BG_CACHE[disp.mapBgKey] = null;
                send('getMapBg', { key: disp.mapBgKey });
            }
        }
        if (dispServer.mode) {
            var modBg = disp.modeBgKey && typeof MODE_BG_CACHE !== 'undefined' && MODE_BG_CACHE[disp.modeBgKey] ? toBlobUrl(MODE_BG_CACHE[disp.modeBgKey]) : null;
            var modPill = '<span class="browser-tag-pill' + (modBg ? ' has-bg' : '') + '"';
            if (modBg) modPill += ' style="background-image:url(' + modBg + ')"';
            modPill += '>' + escapeHtml(disp.modeName) + '</span>';
            metaTags.push(modPill);
            if (disp.modeBgKey && typeof MODE_BG_CACHE !== 'undefined' && MODE_BG_CACHE[disp.modeBgKey] === undefined) {
                MODE_BG_CACHE[disp.modeBgKey] = null;
                send('getModeBg', { key: disp.modeBgKey });
            }
        }
        if (s.modded) metaTags.push('<span class="browser-tag-pill browser-tag-modded">' + t('browser.modded_tag') + '</span>');
        if (metaTags.length) html += ' &middot; ' + metaTags.join(' ');
        html += '</div>';
        if (s.modded && s.modpackUrl) {
            html += '<a class="server-entry-modpack" href="#" data-url="' + escapeAttr(s.modpackUrl) + '" onclick="event.stopPropagation(); openModpackLink(this.dataset.url); return false;" title="' + escapeHtml(t('server_entry.download_modpack')) + '">';
            html += '<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>';
            html += ' ' + t('browser.modpack_link');
            html += '</a>';
        }
        html += '</div>';
        html += '<div class="browser-entry-players">';
        html += '<span class="browser-player-count">' + playerText + '</span>';
        html += '<span class="browser-player-label">' + t('browser.players_label') + '</span>';
        var names = browserPlayerNames[key] || (s.playerNames && s.playerNames.length ? s.playerNames : null);
        html += '<div class="browser-player-tooltip">';
        if (names && names.length) {
            for (var n = 0; n < names.length; n++) {
                html += '<div class="browser-player-tooltip-name">' + escapeHtml(names[n]) + '</div>';
            }
        } else {
            html += '<div class="browser-player-tooltip-name browser-player-tooltip-muted">' + t('browser.players_connected', { count: playerText }) + '</div>';
        }
        html += '</div>';
        html += '</div>';
        // ping column
        var ping = browserPingCache[key];
        var pingNum = (ping != null && isFinite(+ping)) ? Math.floor(+ping) : null;
        html += '<div class="browser-entry-ping" data-key="' + escapeAttr(key) + '">';
        if (pingNum !== null) {
            var pingClass = pingNum < 80 ? 'ping-good' : pingNum < 150 ? 'ping-ok' : 'ping-bad';
            html += '<div class="ping-bars ' + pingClass + '"><span></span><span></span><span></span></div>';
            html += '<span class="ping-ms">' + pingNum + 'ms</span>';
        } else {
            html += '<div class="ping-bars ping-unknown"><span></span><span></span><span></span></div>';
            html += '<span class="ping-ms">--</span>';
        }
        html += '</div>';
        if (modLoggedIn) {
            var isPinned = s.pinned;
            html += '<button class="browser-pin-btn' + (isPinned ? ' pinned' : '') + '" onclick="event.stopPropagation(); browserTogglePin(\'' + escapeJs(key) + '\')" title="' + (isPinned ? t('browser.unpin_title') : t('browser.pin_title')) + '">';
            html += '<svg width="14" height="14" viewBox="0 0 24 24" fill="' + (isPinned ? 'currentColor' : 'none') + '" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 2l2.09 6.26L21 9.27l-5 3.9L17.18 20 12 16.77 6.82 20 8 13.17l-5-3.9 6.91-1.01z"/></svg>';
            html += '</button>';
            html += '<button class="browser-ban-btn" onclick="event.stopPropagation(); browserBanServer(\'' + escapeJs(key) + '\')" title="' + t('browser.ban_title') + '">';
            html += '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><line x1="4.93" y1="4.93" x2="19.07" y2="19.07"/></svg>';
            html += '</button>';
        } else if (s.pinned) {
            html += '<span class="browser-pin-badge" title="' + t('browser.pinned_by_mod') + '">';
            html += '<svg width="12" height="12" viewBox="0 0 24 24" fill="currentColor" stroke="currentColor" stroke-width="1.5"><path d="M12 2l2.09 6.26L21 9.27l-5 3.9L17.18 20 12 16.77 6.82 20 8 13.17l-5-3.9 6.91-1.01z"/></svg>';
            html += '</span>';
        }
        html += '</div>';
    }
    container.innerHTML = html;
    // re-append the empty-state element so it's available for future renders
    if (emptyEl) container.appendChild(emptyEl);

    // re-select previously clicked entry
    if (selectedBrowserKey) {
        var entries = container.querySelectorAll('.browser-entry');
        entries.forEach(function (e) {
            if (e.getAttribute('onclick') && e.getAttribute('onclick').indexOf(selectedBrowserKey) !== -1) {
                e.classList.add('selected');
            }
        });
    }

    // position fixed tooltips on hover
    container.querySelectorAll('.browser-entry-players').forEach(function (el) {
        el.addEventListener('mouseenter', function () {
            var tip = el.querySelector('.browser-player-tooltip');
            if (!tip) return;
            var rect = el.getBoundingClientRect();
            tip.style.right = (window.innerWidth - rect.right) + 'px';
            tip.style.bottom = (window.innerHeight - rect.top + 4) + 'px';
        });
    });
}

// patch a single ping cell in-place without re-rendering the whole list
function updateBrowserEntryPing(address) {
    var ping = browserPingCache[address];
    var el = document.querySelector('.browser-entry-ping[data-key="' + CSS.escape(address) + '"]');
    if (!el || ping == null) return;
    var pingNum = isFinite(+ping) ? Math.floor(+ping) : null;
    if (pingNum === null) return;
    var pingClass = pingNum < 80 ? 'ping-good' : pingNum < 150 ? 'ping-ok' : 'ping-bad';
    var bars = document.createElement('div');
    bars.className = 'ping-bars ' + pingClass;
    bars.innerHTML = '<span></span><span></span><span></span>';
    var ms = document.createElement('span');
    ms.className = 'ping-ms';
    ms.textContent = pingNum + 'ms';
    el.innerHTML = '';
    el.appendChild(bars);
    el.appendChild(ms);
}

function onBrowserEntryClick(address) {
    selectedBrowserKey = address;
    // find the server entry to check for relay info
    var server = findBrowserServer(address);
    // auto-select the right game
    if (server && server.game && typeof selectGame === 'function') {
        selectGame(server.game);
    }
    if (server && server.relayAddress && server.relayKey) {
        // relay server, auto-configure relay join
        setRelayMode('join', 'Relay');
        document.getElementById('joinRelayAddress').value = server.relayAddress;
        document.getElementById('joinRelayKey').value = server.relayKey;
        var codeField = document.getElementById('joinRelayCode');
        if (codeField) codeField.value = server.relayCode || '';
        var hintEl = document.getElementById('joinRelayCodeHint');
        if (hintEl) hintEl.textContent = t('browser.autofilled_relay');
        var infoEl = document.getElementById('joinRelayResolved');
        if (infoEl) {
            infoEl.style.display = '';
            infoEl.innerHTML = '<strong>' + escapeHtml(server.motd || 'Server') + '</strong> <span class="text-muted">(' + escapeHtml(server.game || '?') + ' ' + t('browser.via_relay') + ')</span>';
        }
    } else {
        // direct server
        setRelayMode('join', 'Direct');
        var input = document.getElementById('serverIP');
        if (input) input.value = address.replace(/:.*$/, '');
        // stash the gamePort so joinServer can use it
        var cached = typeof browserPlayerCache !== 'undefined' && typeof serverStatusCache !== 'undefined' ? serverStatusCache[address] : null;
        var server = findBrowserServer(address);
        window._selectedBrowserGamePort = (cached && cached.gamePort) ? cached.gamePort : (server && server.gamePort) ? server.gamePort : 0;
        console.log('[browser] click', address, 'cached:', cached, 'gamePort:', window._selectedBrowserGamePort);
    }
}

function onBrowserEntryDblClick(address) {
    onBrowserEntryClick(address);
    var server = findBrowserServer(address);
    if (server && server.vpnType) {
        showVpnModal(address);
        return;
    }
    if (server && server.hasPassword) {
        showPasswordModal(address);
        return;
    }
    if (typeof joinServer === 'function') joinServer();
}

function findBrowserServer(key) {
    for (var i = 0; i < browserServers.length; i++) {
        var s = browserServers[i];
        var k = s.address + ':' + (s.port || 14638);
        if (k === key) return s;
    }
    return null;
}

function toggleSortDirection() {
    browserSortAsc = !browserSortAsc;
    var btn = document.getElementById('browserSortDir');
    if (btn) btn.classList.toggle('sort-asc', browserSortAsc);
    filterBrowserList();
}

function startBrowserAutoRefresh() {
    if (browserAutoRefreshTimer) return;
    browserAutoRefreshTimer = setInterval(refreshBrowser, 30000);
}

function stopBrowserAutoRefresh() {
    if (browserAutoRefreshTimer) {
        clearInterval(browserAutoRefreshTimer);
        browserAutoRefreshTimer = null;
    }
}

// password prompt modal for passworded servers
var _pendingPasswordJoinKey = null;

function showPasswordModal(key) {
    _pendingPasswordJoinKey = key;
    var input = document.getElementById('passwordModalInput');
    if (input) input.value = '';
    var backdrop = document.getElementById('passwordModalBackdrop');
    if (backdrop) backdrop.style.display = 'flex';
    if (input) input.focus();
}

function closePasswordModal() {
    _pendingPasswordJoinKey = null;
    var backdrop = document.getElementById('passwordModalBackdrop');
    if (backdrop) backdrop.style.display = 'none';
}

function submitPasswordModal() {
    var pw = document.getElementById('passwordModalInput');
    var field = document.getElementById('serverPassword');
    if (field && pw) field.value = pw.value;
    closePasswordModal();
    if (typeof joinServer === 'function') joinServer();
}

// vpn info modal
var _pendingVpnJoinKey = null;

function showVpnModal(key) {
    _pendingVpnJoinKey = key;
    var server = findBrowserServer(key);
    if (!server) return;
    var el = function(id) { return document.getElementById(id); };
    if (el('vpnModalType')) el('vpnModalType').textContent = server.vpnType || '--';
    if (el('vpnModalIP')) el('vpnModalIP').textContent = server.address || '--';
    if (el('vpnModalNetwork')) el('vpnModalNetwork').textContent = server.vpnNetwork || '--';
    if (el('vpnModalPassword')) el('vpnModalPassword').textContent = server.vpnPassword || 'None';
    var backdrop = el('vpnModalBackdrop');
    if (backdrop) backdrop.style.display = 'flex';
}

function closeVpnModal() {
    _pendingVpnJoinKey = null;
    var backdrop = document.getElementById('vpnModalBackdrop');
    if (backdrop) backdrop.style.display = 'none';
}

function confirmVpnJoin() {
    var key = _pendingVpnJoinKey;
    closeVpnModal();
    if (!key) return;
    var server = findBrowserServer(key);
    if (server && server.hasPassword) {
        showPasswordModal(key);
        return;
    }
    if (typeof joinServer === 'function') joinServer();
}

async function browserBanServer(key) {
    var server = findBrowserServer(key);
    var label = server ? (server.motd || key) : key;
    var reason = await cypressPrompt(t('browser.ban_prompt_title'), t('browser.ban_prompt_body', {name: label}));
    if (reason === null) return;
    send('modBanServerByKey', { key: key, reason: reason });
}

function browserTogglePin(key) {
    var server = findBrowserServer(key);
    if (!server) return;
    if (server.pinned) {
        send('modUnpinServer', { address: key });
    } else {
        send('modPinServer', { address: key });
    }
}

function onModPinServerResult(data) {
    if (data.ok) {
        refreshBrowser();
    } else {
        cypressAlert(t('browser.pin_failed'), data.error || 'unknown error');
    }
}

function onModUnpinServerResult(data) {
    if (data.ok) {
        refreshBrowser();
    } else {
        cypressAlert(t('browser.unpin_failed'), data.error || 'unknown error');
    }
}

function onModBanServerByKeyResult(data) {
    if (data.ok) {
        refreshBrowser();
    } else {
        cypressAlert(t('browser.ban_failed'), data.error || 'unknown error');
    }
}

// update player count + tooltip in-place without full rebuild
function updateBrowserEntryPlayers(key) {
    var entries = document.querySelectorAll('.browser-entry');
    for (var i = 0; i < entries.length; i++) {
        var onclick = entries[i].getAttribute('onclick') || '';
        if (onclick.indexOf(key) === -1) continue;

        var server = findBrowserServer(key);
        var liveCount = browserPlayerCache[key];
        var players = liveCount !== undefined ? liveCount : (server ? server.players : undefined);
        var maxPlayersRaw = server ? server.maxPlayers : undefined;
        var safeCount = (players !== undefined && players !== null && isFinite(+players)) ? Math.floor(+players) : '?';
        var safeMax = (maxPlayersRaw !== undefined && maxPlayersRaw !== null && isFinite(+maxPlayersRaw)) ? Math.floor(+maxPlayersRaw) : '?';
        var playerText = safeCount + '/' + safeMax;

        var countEl = entries[i].querySelector('.browser-player-count');
        if (countEl) countEl.textContent = playerText;

        var tooltip = entries[i].querySelector('.browser-player-tooltip');
        if (tooltip) {
            var names = browserPlayerNames[key] || (server && server.playerNames && server.playerNames.length ? server.playerNames : null);
            var tipHtml = '';
            if (names && names.length) {
                for (var n = 0; n < names.length; n++) {
                    tipHtml += '<div class="browser-player-tooltip-name">' + escapeHtml(names[n]) + '</div>';
                }
            } else {
                tipHtml = '<div class="browser-player-tooltip-name browser-player-tooltip-muted">' + t('browser.players_connected', { count: playerText }) + '</div>';
            }
            tooltip.innerHTML = tipHtml;
        }
        break;
    }
}
