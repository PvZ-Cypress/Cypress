// server icon upload & preview

const SERVER_ICON_MAX_SIZE = 64;

function pickServerIcon() {
    document.getElementById('serverIconInput').click();
}

function onServerIconSelected(input) {
    if (!input.files || !input.files[0]) return;
    const file = input.files[0];
    const reader = new FileReader();
    reader.onload = function (e) {
        resizeServerIcon(e.target.result, function (b64) {
            document.getElementById('serverIconData').value = b64;
            const preview = document.getElementById('serverIconPreview');
            preview.innerHTML = '<img src="data:image/jpeg;base64,' + b64 + '" alt="Server Icon" draggable="false">';
            document.getElementById('clearIconBtn').style.display = '';
        });
    };
    reader.readAsDataURL(file);
    input.value = '';
}

function resizeServerIcon(dataUrl, callback) {
    var img = new Image();
    img.onload = function () {
        var canvas = document.createElement('canvas');
        canvas.width = SERVER_ICON_MAX_SIZE;
        canvas.height = SERVER_ICON_MAX_SIZE;
        var ctx = canvas.getContext('2d');
        var scale = Math.max(SERVER_ICON_MAX_SIZE / img.width, SERVER_ICON_MAX_SIZE / img.height);
        var w = img.width * scale;
        var h = img.height * scale;
        var x = (SERVER_ICON_MAX_SIZE - w) / 2;
        var y = (SERVER_ICON_MAX_SIZE - h) / 2;
        ctx.drawImage(img, x, y, w, h);
        var jpegUrl = canvas.toDataURL('image/jpeg', 0.85);
        callback(jpegUrl.split(',')[1]);
    };
    img.src = dataUrl;
}

function clearServerIcon() {
    document.getElementById('serverIconData').value = '';
    document.getElementById('serverIconPreview').innerHTML = '<svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" opacity="0.3"><rect x="3" y="3" width="18" height="18" rx="2" ry="2"/><circle cx="8.5" cy="8.5" r="1.5"/><polyline points="21 15 16 10 5 21"/></svg>';
    document.getElementById('clearIconBtn').style.display = 'none';
}

// minecraft-style server list

var serverList = []; // [{name, address}]
var selectedServerIndex = -1;
var serverStatusCache = {}; // address -> {ok, name, game, players, icon, ...}
var serverCheckTimer = null;
var pendingChecks = {};

function getServerList() { return serverList; }

function setServerList(list) {
    serverList = list || [];
    renderServerList();
}

function addServerPrompt() {
    document.getElementById('serverModalTitle').textContent = t('server_modal.add_title');
    document.getElementById('serverModalName').value = '';
    document.getElementById('serverModalAddress').value = '';
    document.getElementById('serverModalEditIndex').value = '-1';
    document.getElementById('serverModalBackdrop').style.display = 'flex';
    document.getElementById('serverModalName').focus();
}

function editServerEntry(idx) {
    var entry = serverList[idx];
    if (!entry) return;
    document.getElementById('serverModalTitle').textContent = t('server_modal.edit_title');
    document.getElementById('serverModalName').value = entry.name || '';
    document.getElementById('serverModalAddress').value = entry.address || '';
    document.getElementById('serverModalEditIndex').value = idx;
    document.getElementById('serverModalBackdrop').style.display = 'flex';
    document.getElementById('serverModalName').focus();
}

function closeServerModal() {
    document.getElementById('serverModalBackdrop').style.display = 'none';
}

function saveServerEntry() {
    var name = document.getElementById('serverModalName').value.trim();
    var address = document.getElementById('serverModalAddress').value.trim();
    if (!address) return;
    if (!name) name = address;
    var idx = parseInt(document.getElementById('serverModalEditIndex').value);
    if (idx >= 0 && idx < serverList.length) {
        serverList[idx].name = name;
        serverList[idx].address = address;
    } else {
        serverList.push({ name: name, address: address });
        idx = serverList.length - 1;
    }
    closeServerModal();
    renderServerList();
    selectServer(idx);
    pingServer(address);
    saveServerListToBackend();
}

function removeServerEntry(idx) {
    serverList.splice(idx, 1);
    if (selectedServerIndex === idx) {
        selectedServerIndex = -1;
        document.getElementById('serverIP').value = '';
    } else if (selectedServerIndex > idx) {
        selectedServerIndex--;
    }
    renderServerList();
    saveServerListToBackend();
}

function moveServerEntry(idx, dir) {
    var newIdx = idx + dir;
    if (newIdx < 0 || newIdx >= serverList.length) return;
    var tmp = serverList[idx];
    serverList[idx] = serverList[newIdx];
    serverList[newIdx] = tmp;
    if (selectedServerIndex === idx) selectedServerIndex = newIdx;
    else if (selectedServerIndex === newIdx) selectedServerIndex = idx;
    renderServerList();
    saveServerListToBackend();
}

function selectServer(idx) {
    selectedServerIndex = idx;
    var entry = serverList[idx];
    if (entry) {
        document.getElementById('serverIP').value = entry.address;
    }
    // update selection visuals
    document.querySelectorAll('.server-entry').forEach(function (el, i) {
        el.classList.toggle('selected', i === idx);
    });
}

function renderServerList() {
    var container = document.getElementById('serverList');
    var emptyMsg = document.getElementById('serverListEmpty');
    if (!serverList.length) {
        container.innerHTML = '';
        emptyMsg.style.display = 'flex';
        return;
    }
    emptyMsg.style.display = 'none';
    var html = '';
    for (var i = 0; i < serverList.length; i++) {
        var s = serverList[i];
        var status = serverStatusCache[s.address];
        var stateClass = !status ? '' : (status.ok ? 'online' : 'offline');
        var isSelected = i === selectedServerIndex;
        html += '<div class="server-entry ' + stateClass + (isSelected ? ' selected' : '') + '" onclick="selectServer(' + i + ')" ondblclick="selectServer(' + i + '); joinServer();">';
        html += '<div class="server-entry-icon">';
        if (status && status.ok && status.icon && isValidBase64(status.icon)) {
            if (!status._iconBlobUrl) status._iconBlobUrl = typeof base64ToBlobUrl === 'function' ? base64ToBlobUrl(status.icon) : ('data:image/jpeg;base64,' + status.icon);
            html += '<img src="' + status._iconBlobUrl + '" alt="" draggable="false">';
        } else if (status && status.ok) {
            html += '<svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="var(--accent)" stroke-width="1.5" opacity="0.6"><rect x="2" y="2" width="20" height="20" rx="3" ry="3"/><path d="M8 12h8M12 8v8"/></svg>';
        } else {
            html += '<svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" opacity="0.3"><rect x="2" y="2" width="20" height="20" rx="3" ry="3"/><path d="M12 8v4m0 4h.01"/></svg>';
        }
        html += '</div>';
        html += '<div class="server-entry-info">';
        html += '<span class="server-entry-name">' + escapeHtml(s.name) + '</span>';
        var motd = status && status.ok ? (status.motd || t('browser.default_server_name')) : '';
        if (motd) {
            html += '<span class="server-entry-motd motd-rendered">' + renderMotd(motd) + '</span>';
        }
        html += '<span class="server-entry-address">' + escapeHtml(s.address);
        if (status && status.ok) {
            var meta = [];
            if (status.game) meta.push(status.game);
            if (status.modded) meta.push('Modded');
            if (status.map) meta.push(status.map);
            if (status.mode) meta.push(status.mode);
            if (meta.length) html += ' &middot; ' + escapeHtml(meta.join(' · '));
        }
        html += '</span>';
        if (status && status.ok && status.modpackUrl) {
            html += '<a class="server-entry-modpack" href="#" onclick="event.stopPropagation(); openModpackLink(\'' + escapeJs(status.modpackUrl) + '\'); return false;" title="' + escapeHtml(t('server_entry.download_modpack')) + '">';
            html += '<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>';
            html += ' Modpack';
            html += '</a>';
        }
        html += '</div>';
        html += '<div class="server-entry-status">';
        if (!status) {
            html += '<span class="server-entry-ping unknown">?</span>';
        } else if (status.ok) {
            var pText = (status.players !== undefined && status.players !== null && isFinite(+status.players)) ? Math.floor(+status.players) : '?';
            html += '<span class="server-entry-players">' + pText + '</span>';
            html += '<span class="server-entry-ping online-dot"></span>';
        } else {
            html += '<span class="server-entry-ping offline-x">✕</span>';
        }
        html += '</div>';
        html += '<div class="server-entry-actions">';
        html += '<button class="server-entry-btn" onclick="event.stopPropagation(); moveServerEntry(' + i + ', -1)" title="' + escapeHtml(t('server_entry.move_up')) + '"><svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="18 15 12 9 6 15"/></svg></button>';
        html += '<button class="server-entry-btn" onclick="event.stopPropagation(); moveServerEntry(' + i + ', 1)" title="' + escapeHtml(t('server_entry.move_down')) + '"><svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="6 9 12 15 18 9"/></svg></button>';
        html += '<button class="server-entry-btn" onclick="event.stopPropagation(); editServerEntry(' + i + ')" title="' + escapeHtml(t('server_entry.edit')) + '"><svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"/><path d="M18.5 2.5a2.12 2.12 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"/></svg></button>';
        html += '<button class="server-entry-btn danger" onclick="event.stopPropagation(); removeServerEntry(' + i + ')" title="' + escapeHtml(t('server_entry.remove')) + '"><svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg></button>';
        html += '</div>';
        html += '</div>';
    }
    container.innerHTML = html;
}

function isValidBase64(str) {
    return typeof str === 'string' && str.length > 0 && str.length < 500000 && /^[A-Za-z0-9+/=]+$/.test(str);
}

function escapeHtml(str) {
    var d = document.createElement('div');
    d.textContent = str;
    return d.innerHTML;
}

function escapeAttr(str) {
    return str.replace(/&/g, '&amp;').replace(/'/g, '&#39;').replace(/"/g, '&quot;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
}

function openModpackLink(url) {
    try {
        var parsed = new URL(url);
        if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') return;
        send('openExternal', { url: parsed.href });
    } catch (_) {}
}

// ping a single server
function pingServer(address) {
    if (pendingChecks[address]) return;
    pendingChecks[address] = true;
    send('checkServer', { address: address });
}

// ping all servers in the list
function pingAllServers() {
    for (var i = 0; i < serverList.length; i++) {
        pingServer(serverList[i].address);
    }
}

function onServerInfoResult(data) {
    if (!data || !data.address) return;
    delete pendingChecks[data.address];
    var prev = serverStatusCache[data.address];
    serverStatusCache[data.address] = data;
    // only rebuild sidebar if this address is actually in the sidebar server list
    var isSidebarServer = serverList.some(function(s) { return s.address === data.address; });
    if (isSidebarServer) renderServerList();
    // update browser player count + ping if applicable
    if (typeof browserPlayerCache !== 'undefined') {
        if (data.players !== undefined) browserPlayerCache[data.address] = data.players;
        if (data.playerNames) {
            if (typeof browserPlayerNames === 'undefined') window.browserPlayerNames = {};
            browserPlayerNames[data.address] = data.playerNames;
        }
        if (data.ping !== undefined) {
            if (typeof browserPingCache === 'undefined') window.browserPingCache = {};
            browserPingCache[data.address] = data.ping;
        }
        // patch in-place instead of full rebuild
        if (typeof updateBrowserEntryPlayers === 'function') updateBrowserEntryPlayers(data.address);
        if (typeof updateBrowserEntryPing === 'function') updateBrowserEntryPing(data.address);
        // only full rebuild if level/mode actually changed from what we had before
        var levelChanged = data.level && (!prev || prev.level !== data.level);
        var modeChanged = data.mode && (!prev || prev.mode !== data.mode);
        if ((levelChanged || modeChanged) && typeof filterBrowserList === 'function') filterBrowserList();
    }
}

// also support the legacy single-ip check from typing in the address field
function onServerIPChanged() {
    var ip = document.getElementById('serverIP').value.trim();
    if (serverCheckTimer) clearTimeout(serverCheckTimer);
    if (!ip) return;
    // clear stashed browser gamePort since user is typing manually
    window._selectedBrowserGamePort = 0;
    // deselect list entry if user typed a different address
    if (selectedServerIndex >= 0 && serverList[selectedServerIndex] && serverList[selectedServerIndex].address !== ip) {
        selectedServerIndex = -1;
        document.querySelectorAll('.server-entry').forEach(function (el) { el.classList.remove('selected'); });
    }
    serverCheckTimer = setTimeout(function () {
        pingServer(ip);
    }, 800);
}

function saveServerListToBackend() {
    send('saveServerList', { servers: serverList });
}

// collapsible sidebar

var sidebarCollapsed = false;

function toggleSidebar() {
    sidebarCollapsed = !sidebarCollapsed;
    var sidebar = document.getElementById('sidebar');
    var btn = document.getElementById('sidebarCollapseBtn');
    sidebar.classList.toggle('collapsed', sidebarCollapsed);
    btn.title = sidebarCollapsed ? t('sidebar.expand_title') : t('sidebar.collapse_title');
}
