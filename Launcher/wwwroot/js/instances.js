const MAX_LOG_LINES = 2000;

var _pamCallback = null;
function openPlayerActionModal(action, playerNameHtml, callback) {
    _pamCallback = callback;
    var isBan = action === 'ban';
    document.getElementById('pamTitle').textContent = isBan ? t('instances.ban_player') : t('instances.kick_player');
    document.getElementById('pamPlayerName').textContent = playerNameHtml;
    var confirm = document.getElementById('pamConfirm');
    confirm.textContent = isBan ? t('instances.ban') : t('instances.kick');
    confirm.className = 'btn btn-sm ' + (isBan ? 'btn-danger' : 'btn-primary');
    document.getElementById('pamReason').value = '';
    document.getElementById('playerActionModal').style.display = 'flex';
    setTimeout(function() { document.getElementById('pamReason').focus(); }, 50);
}
function closePlayerActionModal() {
    document.getElementById('playerActionModal').style.display = 'none';
    _pamCallback = null;
}
function confirmPlayerAction() {
    if (!_pamCallback) return;
    var reason = document.getElementById('pamReason').value.trim();
    _pamCallback(reason || null);
    closePlayerActionModal();
}

const KNOWN_COMMANDS = [
    'Server.RestartLevel',
    'Server.LoadLevel',
    'Server.LoadNextPlaylistSetup',
    'Server.LoadNextRound',
    'Server.KickPlayer',
    'Server.KickPlayerById',
    'Server.BanPlayer',
    'Server.BanPlayerById',
    'Server.UnbanPlayer',
    'Server.AddBan',
    'Server.Say',
    'Server.SayToPlayer',
    'Cypress.SetLogLevel'
];

// state per instance: { pid, game, isServer, logs[], status, exited, exitCode, label }
const instances = {};
let selectedInstancePid = null;
const commandHistory = {};  // pid -> string[]
const commandHistoryIndex = {};  // pid -> number
const instanceLabels = {};  // pid -> string (persisted name)

const GAME_ICONS = {
    GW1: '<svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="1.8"><circle cx="12" cy="12" r="9"/><path d="M12 3c-2 3-3 6-3 9s1 6 3 9"/><path d="M12 3c2 3 3 6 3 9s-1 6-3 9"/><line x1="3" y1="12" x2="21" y2="12"/></svg>',
    GW2: '<svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="1.8"><circle cx="12" cy="12" r="9"/><path d="M12 3c-2 3-3 6-3 9s1 6 3 9"/><path d="M12 3c2 3 3 6 3 9s-1 6-3 9"/><line x1="3" y1="9" x2="21" y2="9"/><line x1="3" y1="15" x2="21" y2="15"/></svg>',
    BFN: '<svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="1.8"><path d="M12 2L2 7l10 5 10-5-10-5z"/><path d="M2 17l10 5 10-5"/><path d="M2 12l10 5 10-5"/></svg>'
};

const SVG_ICONS = {
    back: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="15 18 9 12 15 6"/></svg>',
    kill: '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><line x1="15" y1="9" x2="9" y2="15"/><line x1="9" y1="9" x2="15" y2="15"/></svg>',
    send: '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="22" y1="2" x2="11" y2="13"/><polygon points="22 2 15 22 11 13 2 9 22 2"/></svg>',
    clear: '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>',
    edit: '<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"/><path d="M18.5 2.5a2.12 2.12 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"/></svg>',
    server: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><rect x="2" y="2" width="20" height="8" rx="2"/><rect x="2" y="14" width="20" height="8" rx="2"/><circle cx="6" cy="6" r="1" fill="currentColor"/><circle cx="6" cy="18" r="1" fill="currentColor"/></svg>',
    client: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><rect x="2" y="3" width="20" height="14" rx="2"/><line x1="8" y1="21" x2="16" y2="21"/><line x1="12" y1="17" x2="12" y2="21"/></svg>',
    logs: '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/></svg>',
    console: '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="4 17 10 11 4 5"/><line x1="12" y1="19" x2="20" y2="19"/></svg>'
};

function getGameLabel(g) {
    return { GW1: 'GW1', GW2: 'GW2', BFN: 'BFN' }[g] || g;
}

function supportsFreecam(game) {
    return game === 'GW1' || game === 'GW2' || game === 'BFN';
}

// check if user has moderator access to a client instance (local mod or global mod override)
function hasModAccess(inst) {
    if (!inst || inst.isServer) return false;
    if (inst.isModerator) return true;
    return typeof modLoggedIn !== 'undefined' && modLoggedIn;
}

// called when global mod login/logout changes - refresh tab visibility for selected instance
function refreshModTabVisibility() {
    if (!selectedInstancePid) return;
    var inst = instances[selectedInstancePid];
    if (!inst) return;
    var isMod = hasModAccess(inst);
    document.getElementById('modPlayersTabBtn').style.display = isMod ? '' : 'none';
    document.getElementById('modServerTabBtn').style.display = inst.isModerator ? '' : 'none';
}

function getInstanceBgSrc(inst) {
    // prefer override bg (e.g. boss hunt custom art)
    if (inst.overrideBg && typeof MAP_BG_CACHE !== 'undefined') {
        if (MAP_BG_CACHE[inst.overrideBg]) {
            return MAP_BG_CACHE[inst.overrideBg];
        }
        if (MAP_BG_CACHE[inst.overrideBg] === undefined) {
            MAP_BG_CACHE[inst.overrideBg] = null;
            send('getMapBg', { key: inst.overrideBg });
        }
    }
    // try map background (from MAP_BG_CACHE in data.js)
    if (inst.level && typeof LEVEL_MAP_BG !== 'undefined') {
        const bgKey = LEVEL_MAP_BG[inst.level];
        if (bgKey && typeof MAP_BG_CACHE !== 'undefined' && MAP_BG_CACHE[bgKey]) {
            return MAP_BG_CACHE[bgKey];
        }
        if (bgKey && typeof MAP_BG_CACHE !== 'undefined' && MAP_BG_CACHE[bgKey] === undefined) {
            MAP_BG_CACHE[bgKey] = null;
            send('getMapBg', { key: bgKey });
        }
    }
    // fall back to game background
    const bgEl = document.getElementById('bgData' + inst.game);
    return bgEl ? bgEl.src : '';
}

function getInstanceDisplayName(pid) {
    const inst = instances[pid];
    if (!inst) return t('common.unknown');
    const label = instanceLabels[pid];
    if (label) return label;
    if (!inst.isServer && inst.username) return inst.username;
    return getGameLabel(inst.game) + ' ' + (inst.isServer ? t('instances.type_server') : t('instances.type_client'));
}

// called from core.js receiveMessage
function handleInstanceMessage(data) {
    switch (data.type) {
        case 'instanceStarted':
            onInstanceStarted(data);
            break;
        case 'instanceOutput':
            onInstanceOutput(data.pid, data.line);
            break;
        case 'instanceExited':
            onInstanceExited(data.pid, data.exitCode);
            break;
        case 'instances':
            onInstancesList(data.list);
            break;
    }
}

function onInstanceStarted(data) {
    const pid = data.pid;
    instances[pid] = {
        pid: pid,
        game: data.game,
        isServer: data.isServer,
        isExternal: data.isExternal || false,
        level: data.level || '',
        startTime: data.startTime,
        logs: [],
        players: {},  // ID -> { name, jointime }
        status: '',
        exited: false,
        exitCode: null,
        motd: data.motd || '',
        icon: data.icon || '',
        modded: data.modded || false,
        modpackUrl: data.modpackUrl || '',
        username: data.username || ''
    };
    commandHistory[pid] = [];
    commandHistoryIndex[pid] = -1;
    updateInstanceList();
    updateBadge();

    // auto-switch to instances tab and select this instance
    switchTab('instances');
    selectInstance(pid);

    // send initial anticheat settings for gw2 servers
    if (data.isServer && data.game === 'GW2') {
        document.querySelectorAll('.ac-host-toggle').forEach(function(cb) {
            var setting = cb.getAttribute('data-ac');
            var val = cb.checked ? 'true' : 'false';
            send('sendCommand', { pid: pid, cmd: 'Cypress.SetAnticheat ' + setting + ' ' + val });
        });
    }
}

function onInstanceOutput(pid, line) {
    const inst = instances[pid];
    if (!inst) return;

    // try parse JSON line from DLL
    let parsed = null;
    if (line.startsWith('{')) {
        try { parsed = JSON.parse(line); } catch (e) { }
    }

    let logEntry;
    if (parsed && parsed.t) {
        // handle player events
        if (parsed.t === 'playerJoin') {
            inst.players[parsed.id] = { name: parsed.name, joinTime: parsed.ts || Date.now() };
            if (inst.sideChannelPeers && inst.sideChannelPeers[parsed.name])
                applyPlayerMetadata(inst, parsed.name, inst.sideChannelPeers[parsed.name], parsed.id);
            if (!inst.playerHistory) inst.playerHistory = [];
            inst.playerHistory.push({ event: 'join', name: parsed.name, ts: parsed.ts || Date.now() });
            if (inst.playerHistory.length > 500) inst.playerHistory.shift();
            if (selectedInstancePid === pid) { updatePlayerList(); if (document.getElementById('stab-srv-history')?.classList.contains('active')) updateHistoryTab(); }
            return;
        }
        if (parsed.t === 'playerLeave') {
            // also clean up side-channel peer data for this player
            if (inst.sideChannelPeers) {
                var leaveName = parsed.name || (inst.players[parsed.id] ? inst.players[parsed.id].name : '');
                if (leaveName && inst.sideChannelPeers[leaveName])
                    delete inst.sideChannelPeers[leaveName];
            }
            var leftName = parsed.name || (inst.players[parsed.id] ? inst.players[parsed.id].name : '');
            if (!inst.playerHistory) inst.playerHistory = [];
            if (leftName) inst.playerHistory.push({ event: 'leave', name: leftName, ts: parsed.ts || Date.now(), reason: parsed.extra || '' });
            if (inst.playerHistory.length > 500) inst.playerHistory.shift();
            delete inst.players[parsed.id];
            if (selectedInstancePid === pid) { updatePlayerList(); if (document.getElementById('stab-srv-history')?.classList.contains('active')) updateHistoryTab(); }
            return;
        }

        // side-channel events
        if (parsed.t === 'sideChannelAuth') {
            if (!inst.sideChannelPeers) inst.sideChannelPeers = {};
            const extra = (parsed.extra || '').split('|');
            const role = extra[0] || 'player';
            const displayName = parsed.display_name || parsed.name;
            const accountId = parsed.account_id || '';
            inst.sideChannelPeers[parsed.name] = { name: parsed.name, displayName: displayName, accountId: accountId, isMod: role === 'mod', connected: true, authTime: parsed.ts || Date.now() };
            applyPlayerMetadata(inst, parsed.name, parsed, parsed.id);
            if (!inst.peerArchive) inst.peerArchive = {};
            inst.peerArchive[parsed.name] = Object.assign(inst.peerArchive[parsed.name] || {}, inst.sideChannelPeers[parsed.name]);
            if (selectedInstancePid === pid) updatePlayerList();
            return;
        }
        if (parsed.t === 'sideChannelDisconnect') {
            // keep peer data so promote button stays visible, just mark disconnected
            if (inst.sideChannelPeers && inst.sideChannelPeers[parsed.name])
                inst.sideChannelPeers[parsed.name].connected = false;
            if (selectedInstancePid === pid) updatePlayerList();
            return;
        }
        if (parsed.t === 'modListChanged') {
            if (selectedInstancePid === pid) updateModeratorTab();
            return;
        }

        // client-side: moderator status from side-channel
        if (parsed.t === 'modStatus') {
            inst.isModerator = (parsed.id === 1);
            updateInstanceList();
            if (selectedInstancePid === pid) {
                var showMod = hasModAccess(inst);
                document.getElementById('modPlayersTabBtn').style.display = showMod ? '' : 'none';
                document.getElementById('modServerTabBtn').style.display = inst.isModerator ? '' : 'none';
                document.getElementById('modBroadcastRow').style.display = (inst.isModerator && inst.game !== 'BFN') ? '' : 'none';
                if (showMod) {
                    updateModeratorTab();
                } else if (document.getElementById('itab-mod-players')?.classList.contains('active') ||
                           document.getElementById('itab-mod-server')?.classList.contains('active')) {
                    switchInstanceTab('logs');
                }
            }
            return;
        }

        // client-side: side-channel player list events (for moderator ui)
        if (parsed.t === 'scPlayerList') {
            if (!inst.scPlayers) inst.scPlayers = {};
            const players = parsed.players || [];
            inst.scPlayers = {};
            players.forEach(p => {
                inst.scPlayers[p.name] = { name: p.name, displayName: p.display_name || p.name };
                applyPlayerMetadata(inst, p.name, p, p.id);
                if (p.ea_pid || p.components || p.account_id) {
                    if (!inst.sideChannelPeers) inst.sideChannelPeers = {};
                    inst.sideChannelPeers[p.name] = {
                        name: p.name, displayName: p.display_name || p.name,
                        hwid: p.hwid || '', eaPid: p.ea_pid || '',
                        accountId: p.account_id || '', components: p.components || [],
                        username: p.username || '', nickname: p.nickname || '',
                        connected: true
                    };
                    if (p.components && p.components.length) {
                        if (!inst.knownComponents) inst.knownComponents = {};
                        if (!inst.knownComponents[p.name]) inst.knownComponents[p.name] = new Set();
                        p.components.forEach(c => inst.knownComponents[p.name].add(c));
                    }
                }
            });
            if (selectedInstancePid === pid) updateModeratorTab();
            return;
        }
        // per-player auth data from side-channel
        if (parsed.t === 'scPlayerAuth') {
            if (!inst.sideChannelPeers) inst.sideChannelPeers = {};
            inst.sideChannelPeers[parsed.name] = {
                name: parsed.name, displayName: parsed.display_name || parsed.name,
                hwid: parsed.hwid || '', eaPid: parsed.ea_pid || '',
                accountId: parsed.account_id || '', components: parsed.components || [],
                username: parsed.username || '', nickname: parsed.nickname || '',
                connected: true
            };
            applyPlayerMetadata(inst, parsed.name, parsed, parsed.id);
            if (!inst.peerArchive) inst.peerArchive = {};
            inst.peerArchive[parsed.name] = Object.assign(inst.peerArchive[parsed.name] || {}, inst.sideChannelPeers[parsed.name]);
            if (parsed.components && parsed.components.length) {
                if (!inst.knownComponents) inst.knownComponents = {};
                if (!inst.knownComponents[parsed.name]) inst.knownComponents[parsed.name] = new Set();
                parsed.components.forEach(c => inst.knownComponents[parsed.name].add(c));
            }
            if (selectedInstancePid === pid) updateModeratorTab();
            return;
        }
        if (parsed.t === 'scPlayerState') {
            applyPlayerMetadata(inst, parsed.name, parsed, parsed.id);
            if (selectedInstancePid === pid) {
                updatePlayerList();
                updateModeratorTab();
            }
            return;
        }
        if (parsed.t === 'scPlayerJoin') {
            if (!inst.scPlayers) inst.scPlayers = {};
            inst.scPlayers[parsed.name] = { name: parsed.name, id: parsed.id };
            applyPlayerMetadata(inst, parsed.name, parsed, parsed.id);
            if (!inst.playerHistory) inst.playerHistory = [];
            inst.playerHistory.push({ event: 'join', name: parsed.name, ts: Date.now() });
            if (inst.playerHistory.length > 500) inst.playerHistory.shift();
            if (selectedInstancePid === pid) {
                updateModeratorTab();
                if (document.getElementById('stab-mod-history')?.classList.contains('active')) updateModHistoryTab();
            }
            return;
        }
        if (parsed.t === 'scPlayerLeave') {
            if (inst.scPlayers) delete inst.scPlayers[parsed.name];
            if (inst.sideChannelPeers) delete inst.sideChannelPeers[parsed.name];
            if (!inst.playerHistory) inst.playerHistory = [];
            inst.playerHistory.push({ event: 'leave', name: parsed.name, ts: Date.now() });
            if (inst.playerHistory.length > 500) inst.playerHistory.shift();
            if (selectedInstancePid === pid) {
                updateModeratorTab();
                if (document.getElementById('stab-mod-history')?.classList.contains('active')) updateModHistoryTab();
            }
            return;
        }
        if (parsed.t === 'scModBans') {
            onModBansResult(parsed);
            return;
        }

        logEntry = {
            type: parsed.t,
            level: parsed.lvl || 'info',
            msg: parsed.msg || '',
            ts: parsed.ts || Date.now(),
            raw: line
        };
        // update status if it's a status message
        if (parsed.t === 'status') {
            inst.status = (parsed.col1 || '') + '\n' + (parsed.col2 || '');
            if (selectedInstancePid === pid) {
                updateDetailStatus();
            }
        }
    } else {
        // plain text line
        logEntry = {
            type: 'raw',
            level: 'info',
            msg: line,
            ts: Date.now(),
            raw: line
        };
    }

    inst.logs.push(logEntry);

    // trim if over max
    if (inst.logs.length > MAX_LOG_LINES) {
        var excess = inst.logs.length - MAX_LOG_LINES;
        inst.logs.splice(0, excess);
        if (selectedInstancePid === pid) {
            var viewer = document.getElementById('logViewer');
            while (excess-- > 0 && viewer.firstChild) viewer.removeChild(viewer.firstChild);
        }
    }

    if (selectedInstancePid === pid) {
        if (logEntry.type === 'srv') {
            // server output goes to console viewer only
            appendConsoleLogLine(logEntry);
        } else {
            appendLogLine(logEntry);
        }
    }
}

function onInstanceExited(pid, exitCode) {
    const inst = instances[pid];
    if (inst) {
        inst.exited = true;
        inst.exitCode = exitCode;
        inst.logs.push({
            type: 'log',
            level: 'Warning',
            msg: 'Process exited with code 0x' + (exitCode >>> 0).toString(16).toUpperCase(),
            ts: Date.now(),
            raw: ''
        });
        if (selectedInstancePid === pid) {
            appendLogLine(inst.logs[inst.logs.length - 1]);
            updateDetailStatus();
            document.getElementById('instanceKillBtn').style.display = 'none';
        }
    }
    updateInstanceList();
    updateBadge();
}

function onInstancesList(list) {
    list.forEach(item => {
        if (!instances[item.pid]) {
            instances[item.pid] = {
                pid: item.pid,
                game: item.game,
                isServer: item.isServer,
                startTime: item.startTime,
                logs: [],
                status: '',
                exited: false,
                exitCode: null
            };
        }
    });
    updateInstanceList();
    updateBadge();
}

function updateBadge() {
    const badge = document.getElementById('instanceBadge');
    const count = Object.values(instances).filter(i => !i.exited).length;
    if (count > 0) {
        badge.textContent = count;
        badge.style.display = 'inline-flex';
    } else {
        badge.style.display = 'none';
    }
}

function updateInstanceList() {
    const listEl = document.getElementById('instanceList');
    const emptyEl = document.getElementById('instanceEmpty');
    const pids = Object.keys(instances);

    if (pids.length === 0) {
        emptyEl.style.display = 'flex';
        var headerEl = document.getElementById('instanceListHeader');
        if (headerEl) headerEl.style.display = 'none';
        // remove any cards
        listEl.querySelectorAll('.instance-card').forEach(c => c.remove());
        return;
    }

    emptyEl.style.display = 'none';
    var headerEl2 = document.getElementById('instanceListHeader');
    if (headerEl2) headerEl2.style.display = 'flex';

    // remove cards for instances that no longer exist
    listEl.querySelectorAll('.instance-card').forEach(card => {
        if (!instances[card.dataset.pid]) card.remove();
    });

    pids.forEach(pid => {
        const inst = instances[pid];
        let card = listEl.querySelector('.instance-card[data-pid="' + pid + '"]');
        if (!card) {
            card = document.createElement('div');
            card.className = 'instance-card';
            card.dataset.pid = pid;
            card.onclick = () => selectInstance(parseInt(pid));
            listEl.appendChild(card);
        }

        const typeClass = inst.isServer ? 'server' : 'client';
        const typeIcon = inst.isServer ? SVG_ICONS.server : SVG_ICONS.client;
        const badgeClass = inst.exited ? 'exited' : (inst.isExternal ? 'external' : typeClass);
        const badgeLabel = inst.exited ? 'EXITED' : (inst.isExternal ? 'EXTERNAL' : (inst.isServer ? 'SERVER' : (inst.isModerator ? 'MOD' : 'CLIENT')));
        const gameIcon = GAME_ICONS[inst.game] || GAME_ICONS.GW2;
        const displayName = getInstanceDisplayName(parseInt(pid));

        // use server icon if available, otherwise map bg
        let iconHtml;
        if (inst.isServer && inst.icon && typeof isValidBase64 === 'function' && isValidBase64(inst.icon)) {
            if (!inst._iconBlobUrl) inst._iconBlobUrl = typeof base64ToBlobUrl === 'function' ? base64ToBlobUrl(inst.icon) : ('data:image/jpeg;base64,' + inst.icon);
            iconHtml = '<div class="instance-card-icon ' + typeClass + '"><img src="' + inst._iconBlobUrl + '" style="width:100%;height:100%;object-fit:cover;border-radius:inherit;"></div>';
        } else {
            iconHtml = '<div class="instance-card-icon ' + typeClass + '">' + typeIcon + '</div>';
        }

        // try to get a background image for this card
        const bgSrc = getInstanceBgSrc(inst);
        const bgHtml = bgSrc ? '<img class="instance-card-bg" src="' + bgSrc + '" alt="">' : '';

        // build meta line
        let metaParts = ['PID ' + pid];
        if (inst.isServer) {
            if (inst.modded) metaParts.push('Modded');
            const pCount = inst.players ? Object.keys(inst.players).length : 0;
            metaParts.push(pCount + ' player' + (pCount !== 1 ? 's' : ''));
        } else if (inst.username) {
            metaParts.push(escapeHtml(inst.username));
        }
        if (!inst.isServer && inst.isModerator) {
            metaParts.push('Local Mod');
        }

        // build subtitle (MOTD for server, empty for client)
        let subtitleHtml = '';
        if (inst.isServer && inst.motd) {
            subtitleHtml = '<div class="instance-card-motd motd-rendered">' + (typeof renderMotd === 'function' ? renderMotd(inst.motd) : escapeHtml(inst.motd)) + '</div>';
        }

        card.innerHTML = bgHtml +
            iconHtml +
            '<div class="instance-card-info">' +
                '<div class="instance-card-title">' +
                    '<span class="game-pill game-pill-' + inst.game.toLowerCase() + '">' + gameIcon + ' ' + getGameLabel(inst.game) + '</span> ' +
                    escapeHtml(displayName) +
                '</div>' +
                subtitleHtml +
                '<div class="instance-card-meta">' + metaParts.join(' · ') + '</div>' +
            '</div>' +
            '<span class="instance-card-badge ' + badgeClass + '">' + badgeLabel + '</span>';
    });
}

function selectInstance(pid) {
    const inst = instances[pid];
    if (!inst) return;

    selectedInstancePid = pid;

    // reset mod controls so they repopulate for this instance's game
    if (typeof modControlsPopulated !== 'undefined') modControlsPopulated = false;

    document.getElementById('instanceList').style.display = 'none';
    document.getElementById('instanceDetail').style.display = 'flex';

    const displayName = getInstanceDisplayName(pid);
    const titleEl = document.getElementById('instanceDetailTitle');
    titleEl.innerHTML = escapeHtml(displayName) + ' <span class="text-muted">(PID ' + pid + ')</span>';

    document.getElementById('instanceRenameBtn').style.display = '';

    // set detail header background
    const detailEl = document.getElementById('instanceDetail');
    const bgSrc = getInstanceBgSrc(inst);
    detailEl.style.setProperty('--instance-bg', bgSrc ? 'url(' + bgSrc + ')' : 'none');

    document.getElementById('instanceKillBtn').style.display = inst.exited ? 'none' : '';

    // show/hide console, players, controls tabs for servers only
    document.getElementById('consoleTabBtn').style.display = inst.isServer ? '' : 'none';
    document.getElementById('playersTabBtn').style.display = inst.isServer ? '' : 'none';
    document.getElementById('serverTabBtn').style.display = inst.isServer ? '' : 'none';
    // moderator tabs: only for client instances with mod status (local or global)
    var isMod = hasModAccess(inst);
    var isLocalMod = inst && inst.isModerator;
    document.getElementById('modPlayersTabBtn').style.display = isMod ? '' : 'none';
    document.getElementById('modServerTabBtn').style.display = isLocalMod ? '' : 'none';
    var showBroadcast = inst.game !== 'BFN';
    document.getElementById('broadcastRow').style.display = (inst.isServer && showBroadcast) ? '' : 'none';
    document.getElementById('sayToPlayerRow').style.display = (inst.isServer && showBroadcast) ? '' : 'none';
    document.getElementById('modBroadcastRow').style.display = (isLocalMod && showBroadcast) ? '' : 'none';

    if (inst.isServer) populateSrvControls(inst);

    // hide anticheat sub-tab for non-GW2
    var isGW2 = inst.game === 'GW2';
    var srvAcTab = document.getElementById('srvAnticheatSubTab');
    if (srvAcTab) srvAcTab.style.display = isGW2 ? '' : 'none';
    var modAcTab = document.getElementById('modAnticheatSubTab');
    if (modAcTab) modAcTab.style.display = isGW2 ? '' : 'none';

    updateDetailStatus();
    updatePlayerList();
    switchInstanceTab('logs');
    rebuildLogView();
}

function renameInstance() {
    const pid = selectedInstancePid;
    if (!pid) return;
    const inst = instances[pid];
    if (!inst) return;

    const titleEl = document.getElementById('instanceDetailTitle');
    const renameBtn = document.getElementById('instanceRenameBtn');
    const current = instanceLabels[pid] || '';

    // replace title with inline input
    const input = document.createElement('input');
    input.type = 'text';
    input.className = 'text-input instance-rename-input';
    input.value = current;
    input.placeholder = getGameLabel(inst.game) + ' ' + (inst.isServer ? t('instances.type_server') : t('instances.type_client'));
    input.maxLength = 40;

    function commit() {
        const name = input.value.trim();
        if (name) {
            instanceLabels[pid] = name;
        } else {
            delete instanceLabels[pid];
        }
        selectInstance(pid);
        updateInstanceList();
    }

    input.addEventListener('keydown', function(e) {
        if (e.key === 'Enter') { e.preventDefault(); commit(); }
        if (e.key === 'Escape') { e.preventDefault(); selectInstance(pid); }
    });
    input.addEventListener('blur', commit);

    titleEl.innerHTML = '';
    titleEl.appendChild(input);
    renameBtn.style.display = 'none';
    input.focus();
    input.select();
}

function closeInstanceDetail() {
    selectedInstancePid = null;
    document.getElementById('instanceDetail').style.display = 'none';
    document.getElementById('instanceList').style.display = 'flex';
    updateInstanceList();

    // clean up exited instances when going back
    Object.keys(instances).forEach(pid => {
        if (instances[pid].exited) {
            delete instances[pid];
            delete commandHistory[pid];
            delete commandHistoryIndex[pid];
        }
    });
    updateInstanceList();
    updateBadge();
}

function updateDetailStatus() {
    const el = document.getElementById('instanceDetailStatus');
    const inst = instances[selectedInstancePid];
    if (!inst) return;

    if (inst.exited) {
        el.textContent = t('instances.exited') + ' (0x' + (inst.exitCode >>> 0).toString(16).toUpperCase() + ')';
    } else if (inst.status) {
        // show a compact version of status
        const lines = inst.status.split('\n').filter(l => l.trim());
        el.textContent = lines.join(' | ').replace(/\t+/g, '  ');
    } else {
        el.textContent = t('instances.running');
    }
}

function switchInstanceTab(tab) {
    document.querySelectorAll('.instance-tab-content').forEach(el => el.classList.remove('active'));
    document.querySelectorAll('.instance-tab-btn').forEach(el => el.classList.remove('active'));
    document.getElementById('itab-' + tab).classList.add('active');
    document.querySelector('.instance-tab-btn[data-itab="' + tab + '"]').classList.add('active');

    if (tab === 'console') {
        rebuildConsoleView();
        document.getElementById('consoleInput').focus();
    }
    if (tab === 'mod-players' || tab === 'mod-server') {
        updateModeratorTab();
    }
}

function killSelectedInstance() {
    if (selectedInstancePid) {
        send('killInstance', { pid: selectedInstancePid });
    }
}

// log viewer

function formatTimestamp(ts) {
    const d = new Date(ts);
    const h = String(d.getHours()).padStart(2, '0');
    const m = String(d.getMinutes()).padStart(2, '0');
    const s = String(d.getSeconds()).padStart(2, '0');
    return h + ':' + m + ':' + s;
}

function logLevelClass(entry) {
    if (entry.type === 'srv') return 'log-srv';
    if (entry.type === 'status') return 'log-status';
    if (entry.type === 'cmd') return 'log-cmd';
    const lvl = (entry.level || 'info').toLowerCase();
    if (lvl === 'warning') return 'log-warning';
    if (lvl === 'error') return 'log-error';
    if (lvl === 'debug') return 'log-debug';
    return 'log-info';
}

function createLogLineEl(entry) {
    const div = document.createElement('div');
    div.className = 'log-line ' + logLevelClass(entry);

    const ts = document.createElement('span');
    ts.className = 'log-ts';
    ts.textContent = formatTimestamp(entry.ts);
    div.appendChild(ts);

    const prefix = entry.type === 'srv' ? '[Server] ' : '';
    div.appendChild(document.createTextNode(prefix + entry.msg));

    // check filter
    const filter = document.getElementById('logFilterInput').value.toLowerCase();
    if (filter && !(entry.msg || '').toLowerCase().includes(filter)) {
        div.classList.add('hidden');
    }

    return div;
}

function appendLogLine(entry) {
    if (entry.type === 'status' || entry.type === 'srv') return; // status/server lines go elsewhere

    const viewer = document.getElementById('logViewer');
    const el = createLogLineEl(entry);
    viewer.appendChild(el);

    // trim DOM nodes if too many
    while (viewer.children.length > MAX_LOG_LINES) {
        viewer.removeChild(viewer.firstChild);
    }

    if (document.getElementById('logAutoScroll').checked) {
        viewer.scrollTop = viewer.scrollHeight;
    }
}

function rebuildLogView() {
    const viewer = document.getElementById('logViewer');
    viewer.innerHTML = '';
    const inst = instances[selectedInstancePid];
    if (!inst) return;

    const frag = document.createDocumentFragment();
    inst.logs.forEach(entry => {
        if (entry.type === 'status' || entry.type === 'srv') return;
        frag.appendChild(createLogLineEl(entry));
    });
    viewer.appendChild(frag);

    if (document.getElementById('logAutoScroll').checked) {
        viewer.scrollTop = viewer.scrollHeight;
    }
}

function filterLogs() {
    const viewer = document.getElementById('logViewer');
    const filter = document.getElementById('logFilterInput').value.toLowerCase();
    viewer.querySelectorAll('.log-line').forEach(el => {
        if (!filter) {
            el.classList.remove('hidden');
        } else {
            const text = el.textContent.toLowerCase();
            el.classList.toggle('hidden', !text.includes(filter));
        }
    });
}

function clearLogs() {
    const inst = instances[selectedInstancePid];
    if (inst) inst.logs = [];
    document.getElementById('logViewer').innerHTML = '';
}

// server console

function appendConsoleLogLine(entry) {
    const viewer = document.getElementById('consoleViewer');
    const el = createLogLineEl(entry);
    viewer.appendChild(el);
    while (viewer.children.length > MAX_LOG_LINES) {
        viewer.removeChild(viewer.firstChild);
    }
    viewer.scrollTop = viewer.scrollHeight;
}

function rebuildConsoleView() {
    const viewer = document.getElementById('consoleViewer');
    viewer.innerHTML = '';
    const inst = instances[selectedInstancePid];
    if (!inst) return;
    const frag = document.createDocumentFragment();
    inst.logs.forEach(entry => {
        if (entry.type === 'srv' || entry.type === 'cmd') {
            frag.appendChild(createLogLineEl(entry));
        }
    });
    viewer.appendChild(frag);
    viewer.scrollTop = viewer.scrollHeight;
}

function sendConsoleCommand() {
    const input = document.getElementById('consoleInput');
    const cmd = input.value.trim();
    if (!cmd || !selectedInstancePid) return;

    // add to history
    if (!commandHistory[selectedInstancePid]) commandHistory[selectedInstancePid] = [];
    commandHistory[selectedInstancePid].push(cmd);
    commandHistoryIndex[selectedInstancePid] = commandHistory[selectedInstancePid].length;

    // show in console viewer
    const viewer = document.getElementById('consoleViewer');
    const div = document.createElement('div');
    div.className = 'log-line log-cmd';
    const ts = document.createElement('span');
    ts.className = 'log-ts';
    ts.textContent = formatTimestamp(Date.now());
    div.appendChild(ts);
    div.appendChild(document.createTextNode('> ' + cmd));
    viewer.appendChild(div);
    viewer.scrollTop = viewer.scrollHeight;

    // send to backend
    send('sendCommand', { pid: selectedInstancePid, cmd: cmd });
    input.value = '';
    hideAutocomplete();
}

function onConsoleKeyDown(e) {
    const pid = selectedInstancePid;
    if (!pid) return;

    if (e.key === 'Enter') {
        e.preventDefault();
        sendConsoleCommand();
        return;
    }

    if (e.key === 'ArrowUp') {
        e.preventDefault();
        const hist = commandHistory[pid] || [];
        let idx = commandHistoryIndex[pid];
        if (idx === undefined || idx === null) idx = hist.length;
        idx = Math.max(0, idx - 1);
        commandHistoryIndex[pid] = idx;
        if (hist[idx]) document.getElementById('consoleInput').value = hist[idx];
        return;
    }

    if (e.key === 'ArrowDown') {
        e.preventDefault();
        const hist = commandHistory[pid] || [];
        let idx = commandHistoryIndex[pid];
        if (idx === undefined || idx === null) idx = hist.length;
        idx = Math.min(hist.length, idx + 1);
        commandHistoryIndex[pid] = idx;
        document.getElementById('consoleInput').value = idx < hist.length ? hist[idx] : '';
        return;
    }

    if (e.key === 'Tab') {
        e.preventDefault();
        const input = document.getElementById('consoleInput');
        const val = input.value;
        const match = KNOWN_COMMANDS.filter(c => c.toLowerCase().startsWith(val.toLowerCase()));
        if (match.length === 1) {
            input.value = match[0] + ' ';
            hideAutocomplete();
        } else if (match.length > 1) {
            showAutocomplete(match);
        }
        return;
    }

    // trigger autocomplete on typing
    setTimeout(() => {
        const val = document.getElementById('consoleInput').value;
        if (val.length >= 2) {
            const match = KNOWN_COMMANDS.filter(c => c.toLowerCase().startsWith(val.toLowerCase()));
            if (match.length > 0 && match.length <= 10) {
                showAutocomplete(match);
            } else {
                hideAutocomplete();
            }
        } else {
            hideAutocomplete();
        }
    }, 0);
}

function showAutocomplete(items) {
    const el = document.getElementById('consoleAutocomplete');
    el.innerHTML = '';
    items.forEach(item => {
        const div = document.createElement('div');
        div.className = 'console-autocomplete-item';
        div.textContent = item;
        div.onclick = () => {
            document.getElementById('consoleInput').value = item + ' ';
            document.getElementById('consoleInput').focus();
            hideAutocomplete();
        };
        el.appendChild(div);
    });
    el.style.display = 'block';
}

function hideAutocomplete() {
    document.getElementById('consoleAutocomplete').style.display = 'none';
}

// player list

function normalizePlayerTeam(team, teamId) {
    const value = (team || '').toString().toLowerCase();
    if (value === 'plants' || teamId === 2) return 'plants';
    if (value === 'zombies' || teamId === 1) return 'zombies';
    return 'unknown';
}

function mergePlayerMetadata(target, source) {
    if (!target || !source) return;
    if (source.team !== undefined || source.team_id !== undefined || source.teamId !== undefined) {
        const nextTeamId = source.team_id !== undefined ? source.team_id : source.teamId;
        target.teamId = nextTeamId !== undefined ? nextTeamId : (target.teamId !== undefined ? target.teamId : -1);
        target.team = normalizePlayerTeam(source.team, target.teamId);
    }
    if (source.class_name !== undefined) target.className = source.class_name || '';
    if (source.className !== undefined) target.className = source.className || '';
    if (source.weapon_name !== undefined) target.weaponName = source.weapon_name || '';
    if (source.weaponName !== undefined) target.weaponName = source.weaponName || '';
    if (source.updated_at !== undefined) target.updatedAt = source.updated_at;
    if (source.updatedAt !== undefined) target.updatedAt = source.updatedAt;
}

function applyPlayerMetadata(inst, playerName, source, playerId) {
    if (!inst || !source) return;

    if (playerName) {
        if (!inst.sideChannelPeers) inst.sideChannelPeers = {};
        if (!inst.sideChannelPeers[playerName]) inst.sideChannelPeers[playerName] = { name: playerName };
        mergePlayerMetadata(inst.sideChannelPeers[playerName], source);

        if (inst.peerArchive && inst.peerArchive[playerName])
            mergePlayerMetadata(inst.peerArchive[playerName], source);

        if (inst.scPlayers && inst.scPlayers[playerName])
            mergePlayerMetadata(inst.scPlayers[playerName], source);
    }

    if (!inst.players) return;

    if (playerId !== undefined && playerId !== null && inst.players[playerId]) {
        mergePlayerMetadata(inst.players[playerId], source);
        return;
    }

    Object.keys(inst.players).forEach(function(id) {
        const player = inst.players[id];
        if (player && player.name === playerName)
            mergePlayerMetadata(player, source);
    });
}

function buildPlayerMetadataText(player, scPeer, game) {
    const className = player.className || (scPeer && scPeer.className) || '';
    const weaponName = player.weaponName || (scPeer && scPeer.weaponName) || '';

    if (game === 'GW1') {
        var label = (typeof getPlayerVariantLabel === 'function')
            ? getPlayerVariantLabel('GW1', className, weaponName)
            : null;
        return label || '';
    }

    if (game === 'GW2') {
        var label = (typeof getPlayerVariantLabel === 'function')
            ? getPlayerVariantLabel('GW2', className, weaponName)
            : null;
        return label || className || '';
    }

    if (game === 'BFN') {
        var isKnownClass = (typeof BFN_CLASS_ICON !== 'undefined') && !!BFN_CLASS_ICON[className];
        return isKnownClass ? '' : (className || '');
    }

    const team = normalizePlayerTeam(player.team || (scPeer && scPeer.team), player.teamId !== undefined ? player.teamId : (scPeer ? scPeer.teamId : undefined));
    return team + ' \u00b7 class: ' + (className || '?') + ' \u00b7 weapon: ' + (weaponName || '?');
}

function createPlayerCard(inst, id, player) {
    const scPeers = inst.sideChannelPeers || {};
    const scPeer = scPeers[player.name];
    const joinTime = new Date(player.joinTime);
    const timeStr = String(joinTime.getHours()).padStart(2, '0') + ':' +
        String(joinTime.getMinutes()).padStart(2, '0') + ':' +
        String(joinTime.getSeconds()).padStart(2, '0');

    const isMod = scPeer && scPeer.isMod;
    const hasAccount = scPeer && scPeer.accountId;
    const eaPid = scPeer && scPeer.eaPid ? scPeer.eaPid : '';
    const modBadge = isMod ? ' <span class="mod-badge">MOD</span>' : '';
    const metadataText = buildPlayerMetadataText(player, scPeer, inst ? inst.game : undefined);

    let modBtn = '';
    if (hasAccount) {
        const safeName = escapeHtml(player.name).replace(/'/g, "\\'");
        if (isMod) {
            modBtn = '<button class="icon-btn icon-btn-small icon-btn-warning" onclick="demotePlayer(\'' + safeName + '\')" title="' + escapeHtml(t('instances.btn_remove_mod')) + '">' +
                '<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/><line x1="9" y1="9" x2="15" y2="15"/><line x1="15" y1="9" x2="9" y2="15"/></svg>' +
            '</button>';
        } else {
            modBtn = '<button class="icon-btn icon-btn-small" onclick="promotePlayer(\'' + safeName + '\')" title="' + escapeHtml(t('instances.btn_make_mod')) + '">' +
                '<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/></svg>' +
            '</button>';
        }
    }

    let copyPidBtn = '';
    if (eaPid) {
        copyPidBtn = '<button class="icon-btn icon-btn-small" onclick="navigator.clipboard.writeText(\'' + escapeAttr(eaPid) + '\')" title="Copy PID: ' + escapeAttr(eaPid) + '">' +
            '<svg fill="currentColor" width="14" height="14" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg"><path d="m16.635 6.162-5.928 9.377H4.24l1.508-2.3h4.024l1.474-2.335H2.264L.79 13.239h2.156L0 17.84h12.072l4.563-7.259 1.652 2.66h-1.401l-1.473 2.299h4.347l1.473 2.3H24zm-11.461.107L3.7 8.604l9.52-.035 1.474-2.3z"/></svg>' +
        '</button>';
    }

    let copyHwidBtn = '';
    if (typeof modLoggedIn !== 'undefined' && modLoggedIn && inst.knownComponents && inst.knownComponents[player.name] && inst.knownComponents[player.name].size) {
        const comps = JSON.stringify(Array.from(inst.knownComponents[player.name]));
        copyHwidBtn = '<button class="icon-btn icon-btn-small" onclick="navigator.clipboard.writeText(\'' + escapeAttr(comps) + '\')" title="' + escapeHtml(t('instances.btn_copy_hwid')) + '">' +
            '<svg fill="currentColor" width="12" height="12" viewBox="0 0 293 293" xmlns="http://www.w3.org/2000/svg"><path d="M271.5,25c0-13.807-11.193-25-25-25h-200c-13.807,0-25,11.193-25,25v243c0,13.807,11.193,25,25,25h200c13.807,0,25-11.193,25-25V25z M53.011,20.816c8.951,0,16.208,7.257,16.208,16.208s-7.257,16.208-16.208,16.208c-8.952,0-16.208-7.257-16.208-16.208S44.059,20.816,53.011,20.816z M53.011,278.496c-8.952,0-16.208-7.257-16.208-16.208c0-8.951,7.257-16.208,16.208-16.208c8.951,0,16.208,7.257,16.208,16.208C69.219,271.239,61.963,278.496,53.011,278.496z M163.624,193.807l3.574-30.99c0.266-2.298-0.328-4.393-1.672-5.899c-2.088-2.344-5.626-2.813-8.777-1.035l-49.005,27.652c-22.588-13.885-37.656-38.818-37.656-67.276c0-43.587,35.334-78.922,78.922-78.922s78.922,35.335,78.922,78.922C227.931,154.85,200.225,186.951,163.624,193.807z M240.655,278.496c-8.952,0-16.208-7.257-16.208-16.208c0-8.951,7.257-16.208,16.208-16.208s16.208,7.257,16.208,16.208C256.864,271.239,249.607,278.496,240.655,278.496z M240.655,53.232c-8.952,0-16.208-7.257-16.208-16.208s7.257-16.208,16.208-16.208s16.208,7.257,16.208,16.208S249.607,53.232,240.655,53.232z"/><circle cx="149.01" cy="116.258" r="28.452"/></svg>' +
        '</button>';
    }

    let freecamBtn = '';
    if (inst.isServer && supportsFreecam(inst.game)) {
        const safeName = escapeHtml(player.name).replace(/'/g, "\\'");
        freecamBtn = '<button class="icon-btn icon-btn-small icon-btn-primary" onclick="srvFreecamPlayer(\'' + safeName + '\')" title="' + escapeHtml(t('instances.btn_freecam')) + '">' +
            '<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"/><circle cx="12" cy="12" r="3"/></svg>' +
        '</button>';
    }

    const card = document.createElement('div');
    card.className = 'player-card';
    card.dataset.playerId = id;
    var _playerIconKey = (inst && typeof getPlayerCharIconKey === 'function')
        ? getPlayerCharIconKey(inst.game, player.className || '', player.weaponName || '')
        : null;
    var _playerAvatarInner = _playerIconKey && typeof charIconImg === 'function'
        ? charIconImg(_playerIconKey)
        : '<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"/><circle cx="12" cy="7" r="4"/></svg>';
    card.innerHTML =
        '<div class="player-card-avatar">' + _playerAvatarInner + '</div>' +
        '<div class="player-card-info">' +
            '<div class="player-card-name">' + escapeHtml(player.name) + modBadge + '</div>' +
            '<div class="player-card-meta player-card-meta-line">ID: ' + id + ' &middot; Joined ' + timeStr + '</div>' +
            '<div class="player-card-meta player-card-meta-line">' + escapeHtml(metadataText) + '</div>' +
        '</div>' +
        '<div class="player-card-actions">' +
            copyPidBtn +
            copyHwidBtn +
            modBtn +
            freecamBtn +
            '<button class="icon-btn icon-btn-small" onclick="kickPlayer(' + id + ')" title="' + escapeHtml(t('instances.kick')) + '">' +
                '<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/><polyline points="16 17 21 12 16 7"/><line x1="21" y1="12" x2="9" y2="12"/></svg>' +
            '</button>' +
            '<button class="icon-btn icon-btn-small icon-btn-danger" onclick="banPlayer(' + id + ')" title="' + escapeHtml(t('instances.ban')) + '">' +
                '<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><line x1="4.93" y1="4.93" x2="19.07" y2="19.07"/></svg>' +
            '</button>' +
            (hasAccount && typeof modLoggedIn !== 'undefined' && modLoggedIn ?
            '<button class="icon-btn icon-btn-small icon-btn-danger" onclick="globalBanPlayer(\'' + escapeJs(player.name) + '\')" title="' + escapeHtml(t('instances.global_ban')) + '" style="background:var(--danger,#e53935);color:#fff;">' +
                '<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/><line x1="9" y1="9" x2="15" y2="15"/><line x1="15" y1="9" x2="9" y2="15"/></svg>' +
            '</button>' : '') +
        '</div>';
    return card;
}

function updatePlayerList() {
    const inst = instances[selectedInstancePid];
    const listEl = document.getElementById('playerList');
    const emptyEl = document.getElementById('playerListEmpty');
    const columnsEl = document.getElementById('playerListColumns');
    const plantsListEl = document.getElementById('plantsPlayerList');
    const zombiesListEl = document.getElementById('zombiesPlayerList');
    const unknownSectionEl = document.getElementById('unknownPlayerSection');
    const unknownListEl = document.getElementById('unknownPlayerList');
    const countEl = document.getElementById('playerListCount');
    const badgeEl = document.getElementById('playerCountBadge');
    const plantsCountEl = document.getElementById('plantsPlayerCount');
    const zombiesCountEl = document.getElementById('zombiesPlayerCount');
    const unknownCountEl = document.getElementById('unknownPlayerCount');

    if (!inst || !inst.isServer) return;

    const playerIds = Object.keys(inst.players);
    const count = playerIds.length;

    countEl.textContent = count + ' player' + (count !== 1 ? 's' : '');

    if (badgeEl) {
        if (count > 0) {
            badgeEl.textContent = count;
            badgeEl.style.display = 'inline-flex';
        } else {
            badgeEl.style.display = 'none';
        }
    }

    // keep SayToPlayer dropdown in sync
    updateSrvPlayerDropdown();

    if (count === 0) {
        emptyEl.style.display = 'flex';
        columnsEl.style.display = 'none';
        unknownSectionEl.style.display = 'none';
        plantsListEl.innerHTML = '';
        zombiesListEl.innerHTML = '';
        unknownListEl.innerHTML = '';
        return;
    }
    emptyEl.style.display = 'none';
    columnsEl.style.display = 'grid';
    plantsListEl.innerHTML = '';
    zombiesListEl.innerHTML = '';
    unknownListEl.innerHTML = '';

    let plantsCount = 0;
    let zombiesCount = 0;
    let unknownCount = 0;

    playerIds.forEach(id => {
        const player = inst.players[id];
        const team = normalizePlayerTeam(player.team, player.teamId);
        const card = createPlayerCard(inst, id, player);

        if (team === 'plants') {
            plantsListEl.appendChild(card);
            plantsCount++;
        } else if (team === 'zombies') {
            zombiesListEl.appendChild(card);
            zombiesCount++;
        } else {
            unknownListEl.appendChild(card);
            unknownCount++;
        }
    });

    plantsCountEl.textContent = plantsCount;
    zombiesCountEl.textContent = zombiesCount;
    unknownCountEl.textContent = unknownCount;
    unknownSectionEl.style.display = unknownCount > 0 ? 'flex' : 'none';
}

function kickPlayer(playerId) {
    if (!selectedInstancePid) return;
    const inst = instances[selectedInstancePid];
    const name = (inst && inst.players && inst.players[playerId]) ? inst.players[playerId].name : 'ID ' + playerId;
    openPlayerActionModal('kick', escapeHtml(name), function(reason) {
        var reasonArg = '';
        if (reason) {
            // bfn uses ParseCommandString which supports quoted strings; gw1/gw2 reads remainder of stream
            reasonArg = ' ' + (inst && inst.game === 'BFN' ? '"' + reason.replace(/"/g, '') + '"' : reason);
        }
        send('sendCommand', { pid: selectedInstancePid, cmd: 'Server.KickPlayerById ' + playerId + reasonArg });
    });
}

function banPlayer(playerId) {
    if (!selectedInstancePid) return;
    const inst = instances[selectedInstancePid];
    if (!inst) return;
    const player = inst.players && inst.players[playerId];
    const name = player ? player.name : 'ID ' + playerId;
    openPlayerActionModal('ban', escapeHtml(name), function(reason) {
        var reasonArg = '';
        if (reason) {
            reasonArg = ' ' + (inst.game === 'BFN' ? '"' + reason.replace(/"/g, '') + '"' : reason);
        }
        send('sendCommand', { pid: selectedInstancePid, cmd: 'Server.BanPlayerById ' + playerId + reasonArg });
    });
}

// global ban from server host view - bans locally + submits to GCBDB
async function globalBanPlayer(playerName) {
    if (!selectedInstancePid) return;
    if (!await cypressConfirm('Global Ban', 'Globally ban ' + playerName + '? This bans them from ALL Cypress servers using GCBDB.')) return;
    var reason = await cypressPrompt('Ban Reason', 'Enter ban reason:', 'Banned by global moderator');
    if (reason === null) return;
    // ban locally first
    send('sendCommand', { pid: selectedInstancePid, cmd: 'Server.BanPlayer "' + playerName + '"' });
    // grab hwid + components from tracked side-channel data
    var inst = instances[selectedInstancePid];
    var scPeers = inst ? (inst.sideChannelPeers || {}) : {};
    var peer = scPeers[playerName];
    send('modGlobalBanPlayer', {
        pid: selectedInstancePid,
        player: playerName,
        reason: reason || '',
        hwid: peer ? peer.hwid : '',
        components: peer ? (peer.components || []) : [],
        ea_pid: peer ? (peer.eaPid || '') : '',
        account_id: peer ? (peer.accountId || '') : ''
    });
}

function srvFreecamPlayer(playerName) {
    if (!selectedInstancePid) return;
    send('sendCommand', { pid: selectedInstancePid, cmd: 'Cypress.Freecam "' + playerName + '"' });
}

function srvPreBan() {
    var name = (document.getElementById('srvPrebanName').value || '').trim();
    var reason = (document.getElementById('srvPrebanReason').value || '').trim();
    if (!name) return;
    if (!selectedInstancePid) return;
    send('sendCommand', { pid: selectedInstancePid, cmd: 'Server.AddBan "' + name + '"' + (reason ? ' ' + reason : '') });
    document.getElementById('srvPrebanName').value = '';
    document.getElementById('srvPrebanReason').value = '';
    setTimeout(fetchLocalBans, 400);
}

function srvUnbanPlayer(name) {
    if (!selectedInstancePid) return;
    send('sendCommand', { pid: selectedInstancePid, cmd: 'Server.UnbanPlayer "' + name + '"' });
    setTimeout(fetchLocalBans, 400);
}

function fetchLocalBans() {
    send('getLocalBans', {});
}

function onLocalBansResult(data) {
    var list = document.getElementById('srvBanList');
    var empty = document.getElementById('srvBanListEmpty');
    if (!list) return;
    var bans = data.bans || [];
    list.querySelectorAll('.player-card').forEach(function(el) { el.remove(); });
    if (bans.length === 0) {
        if (empty) empty.style.display = '';
        return;
    }
    if (empty) empty.style.display = 'none';
    bans.forEach(function(ban) {
        var names = (ban.Names || [ban.Name] || []).filter(Boolean);
        var primaryName = names[0] || '(unknown)';
        var allNames = names.join(', ') || '(unknown)';
        var reason = ban.BanReason || '';
        var hwid = ban.MachineId || '';
        var hasHw = hwid && hwid !== 'UNIQUEID' && hwid.length > 4;
        var card = document.createElement('div');
        card.className = 'player-card';
        card.innerHTML =
            '<div class="player-card-avatar"><svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><line x1="4.93" y1="4.93" x2="19.07" y2="19.07"/></svg></div>' +
            '<div class="player-card-info">' +
                '<div class="player-card-name" title="' + escapeHtml(allNames) + '">' + escapeHtml(primaryName) + (names.length > 1 ? ' <span style="color:var(--text-muted);font-weight:400;font-size:11px;">+' + (names.length - 1) + ' alias</span>' : '') + '</div>' +
                '<div class="player-card-meta">' +
                    (reason ? escapeHtml(reason) : '<span style="opacity:0.4">' + escapeHtml(t('instances.no_reason')) + '</span>') +
                    (hasHw ? ' &nbsp;·&nbsp; <span title="' + escapeHtml(hwid) + '">' + escapeHtml(hwid.substring(0, 8)) + '…</span>' : ' &nbsp;·&nbsp; <span style="opacity:0.4">' + escapeHtml(t('instances.no_hw')) + '</span>') +
                '</div>' +
            '</div>' +
            '<div class="player-card-actions">' +
                '<button class="icon-btn icon-btn-small icon-btn-danger" data-unban="' + escapeHtml(primaryName) + '" title="' + escapeHtml(t('instances.btn_unban')) + '">' +
                '<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="11" width="18" height="11" rx="2" ry="2"/><path d="M7 11V7a5 5 0 0 1 9.9-1"/></svg></button>' +
            '</div>';
        card.querySelector('[data-unban]').addEventListener('click', function() {
            srvUnbanPlayer(this.getAttribute('data-unban'));
        });
        list.appendChild(card);
    });
}

function updateHistoryTab() {
    var inst = selectedInstancePid ? instances[selectedInstancePid] : null;
    var list = document.getElementById('srvHistoryList');
    var empty = document.getElementById('srvHistoryEmpty');
    if (!list) return;
    list.querySelectorAll('.history-row').forEach(function(el) { el.remove(); });
    var history = (inst && inst.playerHistory) ? inst.playerHistory : [];
    if (history.length === 0) {
        if (empty) empty.style.display = '';
        return;
    }
    if (empty) empty.style.display = 'none';
    var knownComponents = (inst && inst.knownComponents) ? inst.knownComponents : {};
    // newest first
    var sorted = history.slice().reverse();
    sorted.forEach(function(entry) {
        var d = new Date(entry.ts);
        var time = d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
        var isJoin = entry.event === 'join';
        var hashes = knownComponents[entry.name] ? Array.from(knownComponents[entry.name]) : [];
        var row = document.createElement('div');
        row.className = 'history-row';
        row.style.cssText = 'display:flex;flex-direction:column;gap:2px;padding:3px 8px;border-radius:4px;font-size:12px;';

        var mainRow = document.createElement('div');
        mainRow.style.cssText = 'display:flex;align-items:center;gap:8px;';
        mainRow.innerHTML =
            '<span style="color:var(--text-muted);flex-shrink:0;font-family:monospace;">' + escapeHtml(time) + '</span>' +
            '<svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="' + (isJoin ? '#4caf50' : 'var(--text-muted)') + '" stroke-width="2.5">' +
                (isJoin ? '<polyline points="5 12 19 12"/><polyline points="13 6 19 12 13 18"/>' : '<polyline points="19 12 5 12"/><polyline points="11 18 5 12 11 6"/>') +
            '</svg>' +
            '<span class="history-name" style="flex:1;font-weight:500;cursor:' + (hashes.length ? 'pointer' : 'default') + ';">' + escapeHtml(entry.name) + (hashes.length ? ' <span style="color:var(--text-muted);font-size:10px;font-weight:400;">[' + hashes.length + ' hash' + (hashes.length !== 1 ? 'es' : '') + ']</span>' : '') + '</span>' +
            (entry.reason ? '<span style="color:var(--text-muted);font-size:11px;">' + escapeHtml(entry.reason) + '</span>' : '');

        var hashList = null;
        if (hashes.length) {
            hashList = document.createElement('div');
            hashList.style.cssText = 'display:none;padding:4px 18px;display:none;flex-direction:column;gap:2px;';
            hashes.forEach(function(h) {
                var span = document.createElement('div');
                span.style.cssText = 'font-family:monospace;font-size:10px;color:var(--text-muted);user-select:all;';
                span.textContent = h;
                hashList.appendChild(span);
            });
            mainRow.querySelector('.history-name').addEventListener('click', function() {
                hashList.style.display = hashList.style.display === 'none' ? 'flex' : 'none';
            });
        }

        row.appendChild(mainRow);
        if (hashList) row.appendChild(hashList);
        list.appendChild(row);
    });
}

// server controls

function populateSrvControls(inst) {
    const game = inst.game;
    const gd = typeof GAME_DATA !== 'undefined' ? GAME_DATA[game] : null;
    if (!gd) return;

    // populate level dropdown
    const levelSel = document.getElementById('srvLevel');
    levelSel.innerHTML = '';
    let lastCat = '';
    let optgroup = null;
    gd.levels.forEach(lv => {
        if (lv.cat !== lastCat) {
            optgroup = document.createElement('optgroup');
            optgroup.label = lv.cat;
            levelSel.appendChild(optgroup);
            lastCat = lv.cat;
        }
        const opt = document.createElement('option');
        opt.value = (game === 'GW1' && lv.variant !== undefined) ? lv.id + '#' + lv.variant : lv.id;
        opt.textContent = lv.name;
        (optgroup || levelSel).appendChild(opt);
    });

    // show/hide game-specific fields
    document.getElementById('srvTodGroup').style.display = (game === 'GW2') ? '' : 'none';
    document.getElementById('srvHostedModeGroup').style.display = (game === 'GW2') ? '' : 'none';
    document.getElementById('srvStartPointGroup').style.display = (game === 'BFN') ? '' : 'none';

    // render smart picker for level
    if (typeof renderPickerOptions === 'function') renderPickerOptions('srvLevel');

    // populate instance modifiers
    if (typeof populateInstanceModifiers === 'function') populateInstanceModifiers('srvModifiers', game);

    onSrvLevelChanged();
}

function onSrvLevelChanged() {
    const inst = instances[selectedInstancePid];
    if (!inst) return;
    const gd = GAME_DATA[inst.game];
    if (!gd) return;

    const rawVal = document.getElementById('srvLevel').value;
    let levelId = rawVal;
    let level;
    if (inst.game === 'GW1' && rawVal.includes('#')) {
        const parsed = typeof parseGW1LevelValue === 'function' ? parseGW1LevelValue(rawVal) : { levelId: rawVal, variant: '0' };
        levelId = parsed.levelId;
        level = gd.levels.find(l => l.id === parsed.levelId && l.variant === parsed.variant);
    } else {
        level = gd.levels.find(l => l.id === levelId);
    }

    // populate mode dropdown with all modes, mark unsupported
    const modeSel = document.getElementById('srvMode');
    modeSel.innerHTML = '';
    const supportedModes = level && level.modes ? new Set(level.modes) : null;
    const modeCats = {};
    gd.modes.forEach(m => { if (!modeCats[m.cat]) modeCats[m.cat] = []; modeCats[m.cat].push(m); });
    for (const cat in modeCats) {
        const optgroup = document.createElement('optgroup');
        optgroup.label = cat;
        modeCats[cat].forEach(m => {
            const opt = document.createElement('option');
            opt.value = m.id;
            opt.textContent = m.name;
            optgroup.appendChild(opt);
        });
        modeSel.appendChild(optgroup);
    }

    // populate start points for BFN
    if (inst.game === 'BFN' && gd.startPoints && gd.startPoints.length > 0) {
        const spSel = document.getElementById('srvStartPoint');
        spSel.innerHTML = '';
        gd.startPoints.forEach(sp => {
            if (!level) return;
            const levelCat = level.cat;
            const byLevel = sp.levels && sp.levels.includes(levelId);
            const byCat = sp.cats && sp.cats.includes(levelCat);
            const byExtra = sp.extraLevels && sp.extraLevels.includes(levelId);
            const byExclude = sp.excludeLevels && sp.excludeLevels.includes(levelId);
            if (byExclude) return;
            if (byLevel || byCat || byExtra) {
                const opt = document.createElement('option');
                opt.value = sp.id;
                opt.textContent = sp.name;
                spSel.appendChild(opt);
            }
        });
    }

    updateSrvPlayerDropdown();
    updateSrvOverride();
    if (typeof renderPickerOptions === 'function') renderPickerOptions('srvMode');
    if (typeof renderPickerOptions === 'function') renderPickerOptions('srvStartPoint');
    if (typeof updatePickerTrigger === 'function') updatePickerTrigger('srvStartPoint');
    // mark unsupported modes (dimmed, not hidden)
    if (supportedModes) {
        document.querySelectorAll('#srvModeOptions .picker-option').forEach(btn => {
            btn.classList.toggle('picker-option-unsupported', !supportedModes.has(btn.getAttribute('data-value')));
        });
    }
    if (typeof updatePickerTrigger === 'function') updatePickerTrigger('srvLevel');
}

function onSrvModeChanged() {
    updateSrvOverride();
}

function updateSrvOverride() {
    const inst = instances[selectedInstancePid];
    if (!inst) return;
    const banner = document.getElementById('srvOverrideBanner');
    const nameEl = document.getElementById('srvOverrideName');

    if (inst.game !== 'GW2' || typeof GW2_LOADSCREEN_OVERRIDES === 'undefined') {
        if (inst.overrideBg) delete inst.overrideBg;
        banner.style.display = 'none';
        return;
    }

    const levelId = document.getElementById('srvLevel').value;
    const modeId = document.getElementById('srvMode').value;
    const key = levelId + '+' + modeId;
    const override = GW2_LOADSCREEN_OVERRIDES[key];

    if (override && override.displayName) {
        nameEl.textContent = override.displayName;
        banner.style.display = '';

        // store override bg on instance for card rendering + detail header
        if (override.bg) {
            inst.overrideBg = override.bg;
            const detailEl = document.getElementById('instanceDetail');
            if (typeof MAP_BG_CACHE !== 'undefined' && MAP_BG_CACHE[override.bg]) {
                detailEl.style.setProperty('--instance-bg', 'url(' + MAP_BG_CACHE[override.bg] + ')');
            } else if (typeof MAP_BG_CACHE !== 'undefined' && MAP_BG_CACHE[override.bg] === undefined) {
                MAP_BG_CACHE[override.bg] = null;
                send('getMapBg', { key: override.bg });
            }
            updateInstanceList();
        }
    } else {
        if (inst.overrideBg) {
            delete inst.overrideBg;
            // revert detail header bg to level default
            const detailEl = document.getElementById('instanceDetail');
            const bgSrc = getInstanceBgSrc(inst);
            detailEl.style.setProperty('--instance-bg', bgSrc ? 'url(' + bgSrc + ')' : 'none');
            updateInstanceList();
        }
        banner.style.display = 'none';
    }
}

function updateSrvPlayerDropdown() {
    const inst = instances[selectedInstancePid];
    if (!inst) return;
    const sel = document.getElementById('srvSayToPlayerSelect');
    sel.innerHTML = '';
    Object.keys(inst.players).forEach(id => {
        const p = inst.players[id];
        const opt = document.createElement('option');
        opt.value = p.name;
        opt.textContent = p.name;
        sel.appendChild(opt);
    });
}

function srvLoadMap() {
    const inst = instances[selectedInstancePid];
    if (!inst || inst.exited) return;

    const rawLevelVal = document.getElementById('srvLevel').value;
    const modeId = document.getElementById('srvMode').value;
    if (!rawLevelVal || !modeId) return;

    let levelId = rawLevelVal;
    let variantSuffix = '';
    if (inst.game === 'GW1' && rawLevelVal.includes('#')) {
        const parsed = typeof parseGW1LevelValue === 'function' ? parseGW1LevelValue(rawLevelVal) : { levelId: rawLevelVal, variant: '0' };
        levelId = parsed.levelId;
        variantSuffix = parsed.variant;
    }

    let cmd;
    if (inst.game === 'BFN') {
        const startPoint = document.getElementById('srvStartPoint').value || '';
        cmd = 'Server.LoadLevel ' + levelId + ' GameMode=' + modeId + ' ' + startPoint;
    } else {
        let modeStr = (inst.game === 'GW1') ? modeId + variantSuffix : modeId;
        let inclusion = 'GameMode=' + modeStr;
        if (inst.game === 'GW2') {
            const tod = document.getElementById('srvTod').value;
            const hosted = document.getElementById('srvHostedMode').value;
            const isHub = levelId === 'Level_FE_Hub' || levelId === 'Level_Hub_TacoBandits';
            if (!isHub) inclusion += ';TOD=' + tod;
            inclusion += ';HostedMode=' + hosted;
        }
        cmd = 'Server.LoadLevel ' + levelId + ' ' + inclusion;

        const overrideKey = levelId + '+' + modeId;
        const lsOverride = (inst.game === 'GW2' && typeof GW2_LOADSCREEN_OVERRIDES !== 'undefined') ? (GW2_LOADSCREEN_OVERRIDES[overrideKey] || null) : null;
        if (lsOverride) {
            // all 4 positional args must be provided together
            cmd += ' ' + (lsOverride.mode || '') + ' ' + (lsOverride.name || '') + ' ' + (lsOverride.desc || '') + ' ' + (lsOverride.asset || '');
        }
    }

    send('sendCommand', { pid: selectedInstancePid, cmd: cmd.trim() });

    if (inst.game === 'GW2') {
        const tod = document.getElementById('srvTod').value;
        const isHub = levelId === 'Level_FE_Hub' || levelId === 'Level_Hub_TacoBandits';
        if (isHub) send('sendCommand', { pid: selectedInstancePid, cmd: 'GameMode.ForceHUBTimeOfDay ' + tod.toUpperCase() });
    }

    // update instance level for card/header bg
    inst.level = levelId;
    updateInstanceList();
    const detailEl = document.getElementById('instanceDetail');
    const bgSrc = getInstanceBgSrc(inst);
    detailEl.style.setProperty('--instance-bg', bgSrc ? 'url(' + bgSrc + ')' : 'none');
}

function srvRestartLevel() {
    if (!selectedInstancePid) return;
    send('sendCommand', { pid: selectedInstancePid, cmd: 'Server.RestartLevel' });
}

function srvApplyModifiers() {
    if (!selectedInstancePid) return;
    const cmds = typeof getInstanceModifierCommands === 'function' ? getInstanceModifierCommands('srvModifiers') : [];
    cmds.forEach(cmd => {
        send('sendCommand', { pid: selectedInstancePid, cmd: 'Cypress.SetSetting ' + cmd });
    });
    send('sendCommand', { pid: selectedInstancePid, cmd: 'Server.RestartLevel' });
}

function srvNextPlaylist() {
    if (!selectedInstancePid) return;
    send('sendCommand', { pid: selectedInstancePid, cmd: 'Server.LoadNextPlaylistSetup' });
}

function srvSayToPlayer() {
    const input = document.getElementById('srvSayToPlayerInput');
    const msg = input.value.trim();
    const playerName = document.getElementById('srvSayToPlayerSelect').value;
    if (!msg || !playerName || !selectedInstancePid) return;
    const duration = document.getElementById('broadcastDuration').value;
    send('sendCommand', { pid: selectedInstancePid, cmd: 'Server.SayToPlayer "' + playerName + '" ' + msg + ' ' + duration });
    input.value = '';
}

function sendBroadcast() {
    const input = document.getElementById('broadcastInput');
    const msg = input.value.trim();
    if (!msg || !selectedInstancePid) return;
    const duration = document.getElementById('broadcastDuration').value;
    send('sendCommand', { pid: selectedInstancePid, cmd: 'Server.Say ' + msg + ' ' + duration });
    input.value = '';
}

// moderator tab

function updateModeratorTab() {
    const inst = instances[selectedInstancePid];
    if (!inst || !hasModAccess(inst)) return;
    populateModControls(inst);
    updateModClientView(inst);
}

// refresh player cards when side-channel auth events arrive (to show mod badges)
function onSideChannelPeersChanged() {
    if (selectedInstancePid && instances[selectedInstancePid]) {
        updatePlayerList();
    }
}

function promotePlayer(playerName) {
    const inst = instances[selectedInstancePid];
    if (!inst || !inst.sideChannelPeers) return;
    const peer = inst.sideChannelPeers[playerName];
    if (!peer || !peer.accountId) return;
    send('sendCommand', { pid: selectedInstancePid, cmd: 'Cypress.AddMod ' + peer.accountId });
    peer.isMod = true;
    updatePlayerList();
}

function demotePlayer(playerName) {
    const inst = instances[selectedInstancePid];
    if (!inst || !inst.sideChannelPeers) return;
    const peer = inst.sideChannelPeers[playerName];
    if (!peer || !peer.accountId) return;
    send('sendCommand', { pid: selectedInstancePid, cmd: 'Cypress.RemoveMod ' + peer.accountId });
    peer.isMod = false;
    updatePlayerList();
}

// client mod view

function updateModClientView(inst) {
    const players = inst.scPlayers || {};
    const names = Object.keys(players);
    const listEl = document.getElementById('modClientPlayerList');
    const emptyEl = document.getElementById('modClientPlayerListEmpty');
    const countEl = document.getElementById('modPlayerListCount');
    const badgeEl = document.getElementById('modPlayerCountBadge');

    if (countEl) countEl.textContent = names.length + ' player' + (names.length !== 1 ? 's' : '');
    if (badgeEl) {
        if (names.length > 0) {
            badgeEl.textContent = names.length;
            badgeEl.style.display = 'inline-flex';
        } else {
            badgeEl.style.display = 'none';
        }
    }

    if (names.length === 0) {
        emptyEl.style.display = 'flex';
        listEl.querySelectorAll('.mod-player-card').forEach(c => c.remove());
        return;
    }
    emptyEl.style.display = 'none';

    // remove stale cards
    listEl.querySelectorAll('.mod-player-card').forEach(card => {
        if (!players[card.dataset.playerName]) card.remove();
    });

    names.forEach(name => {
        let card = listEl.querySelector('.mod-player-card[data-player-name="' + CSS.escape(name) + '"]');
        if (!card) {
            card = document.createElement('div');
            card.className = 'mod-player-card player-card';
            card.dataset.playerName = name;
            listEl.appendChild(card);
        }

        var safeName = escapeHtml(name).replace(/'/g, "\\'");
        var canFreecam = supportsFreecam(inst.game);
        var playerData = inst.scPlayers ? inst.scPlayers[name] : null;
        var peerData = inst.sideChannelPeers ? inst.sideChannelPeers[name] : null;
        var playerEaPid = peerData ? (peerData.eaPid || '') : '';
        var playerComps = inst.knownComponents && inst.knownComponents[name] && inst.knownComponents[name].size ? inst.knownComponents[name] : null;

        var copyPidBtn = '';
        if (playerEaPid) {
            copyPidBtn = '<button class="icon-btn icon-btn-small" onclick="navigator.clipboard.writeText(\'' + escapeAttr(playerEaPid) + '\')" title="Copy PID: ' + escapeAttr(playerEaPid) + '">' +
                '<svg fill="currentColor" width="14" height="14" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg"><path d="m16.635 6.162-5.928 9.377H4.24l1.508-2.3h4.024l1.474-2.335H2.264L.79 13.239h2.156L0 17.84h12.072l4.563-7.259 1.652 2.66h-1.401l-1.473 2.299h4.347l1.473 2.3H24zm-11.461.107L3.7 8.604l9.52-.035 1.474-2.3z"/></svg>' +
            '</button>';
        }
        var copyHwidBtn = '';
        if (playerComps) {
            var compsJson = JSON.stringify(Array.from(playerComps));
            copyHwidBtn = '<button class="icon-btn icon-btn-small" onclick="navigator.clipboard.writeText(\'' + escapeAttr(compsJson) + '\')" title="' + escapeHtml(t('instances.btn_copy_hwid')) + '">' +
                '<svg fill="currentColor" width="12" height="12" viewBox="0 0 293 293" xmlns="http://www.w3.org/2000/svg"><path d="M271.5,25c0-13.807-11.193-25-25-25h-200c-13.807,0-25,11.193-25,25v243c0,13.807,11.193,25,25,25h200c13.807,0,25-11.193,25-25V25z M53.011,20.816c8.951,0,16.208,7.257,16.208,16.208s-7.257,16.208-16.208,16.208c-8.952,0-16.208-7.257-16.208-16.208S44.059,20.816,53.011,20.816z M53.011,278.496c-8.952,0-16.208-7.257-16.208-16.208c0-8.951,7.257-16.208,16.208-16.208c8.951,0,16.208,7.257,16.208,16.208C69.219,271.239,61.963,278.496,53.011,278.496z M163.624,193.807l3.574-30.99c0.266-2.298-0.328-4.393-1.672-5.899c-2.088-2.344-5.626-2.813-8.777-1.035l-49.005,27.652c-22.588-13.885-37.656-38.818-37.656-67.276c0-43.587,35.334-78.922,78.922-78.922s78.922,35.335,78.922,78.922C227.931,154.85,200.225,186.951,163.624,193.807z M240.655,278.496c-8.952,0-16.208-7.257-16.208-16.208c0-8.951,7.257-16.208,16.208-16.208s16.208,7.257,16.208,16.208C256.864,271.239,249.607,278.496,240.655,278.496z M240.655,53.232c-8.952,0-16.208-7.257-16.208-16.208s7.257-16.208,16.208-16.208s16.208,7.257,16.208,16.208S249.607,53.232,240.655,53.232z"/><circle cx="149.01" cy="116.258" r="28.452"/></svg>' +
            '</button>';
        }

        var _modIconKey = (inst && typeof getPlayerCharIconKey === 'function')
            ? getPlayerCharIconKey(inst.game, (playerData && playerData.className) || (peerData && peerData.className) || '', (playerData && playerData.weaponName) || (peerData && peerData.weaponName) || '')
            : null;
        var _modAvatarInner = _modIconKey && typeof charIconImg === 'function'
            ? charIconImg(_modIconKey)
            : '<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"/><circle cx="12" cy="7" r="4"/></svg>';
        card.innerHTML =
            '<div class="player-card-avatar">' + _modAvatarInner + '</div>' +
            '<div class="player-card-info">' +
                '<div class="player-card-name">' + escapeHtml(name) + '</div>' +
            '</div>' +
            '<div class="player-card-actions">' +
                copyPidBtn +
                copyHwidBtn +
                (canFreecam ? '<button class="icon-btn icon-btn-small icon-btn-primary" onclick="modFreecamPlayer(\'' + safeName + '\')" title="' + escapeHtml(t('instances.btn_freecam')) + '">' +
                    '<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"/><circle cx="12" cy="12" r="3"/></svg>' +
                '</button>' : '') +
                '<button class="icon-btn icon-btn-small" onclick="modKickPlayer(\'' + safeName + '\')" title="' + escapeHtml(t('instances.kick')) + '">' +
                    '<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/><polyline points="16 17 21 12 16 7"/><line x1="21" y1="12" x2="9" y2="12"/></svg>' +
                '</button>' +
                '<button class="icon-btn icon-btn-small icon-btn-danger" onclick="modBanPlayer(\'' + safeName + '\')" title="' + escapeHtml(t('instances.ban')) + '">' +
                    '<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><line x1="4.93" y1="4.93" x2="19.07" y2="19.07"/></svg>' +
                '</button>' +
                (typeof modLoggedIn !== 'undefined' && modLoggedIn ?
                '<button class="icon-btn icon-btn-small icon-btn-danger" onclick="modGlobalBanPlayer(\'' + safeName + '\')" title="' + escapeHtml(t('instances.global_ban')) + '" style="background:var(--danger,#e53935);color:#fff;">' +
                    '<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/><line x1="9" y1="9" x2="15" y2="15"/><line x1="15" y1="9" x2="9" y2="15"/></svg>' +
                '</button>' : '') +
            '</div>';
    });
}

function modKickPlayer(playerName) {
    if (!selectedInstancePid) return;
    openPlayerActionModal('kick', escapeHtml(playerName), function(reason) {
        var cmd = 'Cypress.ModKick "' + playerName + '"';
        if (reason) cmd += ' ' + reason;
        send('sendCommand', { pid: selectedInstancePid, cmd: cmd });
    });
}

function modBanPlayer(playerName) {
    if (!selectedInstancePid) return;
    openPlayerActionModal('ban', escapeHtml(playerName), function(reason) {
        var cmd = 'Cypress.ModBan "' + playerName + '"';
        if (reason) cmd += ' ' + reason;
        send('sendCommand', { pid: selectedInstancePid, cmd: cmd });
    });
}

// global ban from client mod view - bans locally + submits to GCBDB
async function modGlobalBanPlayer(playerName) {
    if (!selectedInstancePid) return;
    if (!await cypressConfirm('Global Ban', 'Globally ban ' + playerName + '? This bans them from ALL Cypress servers using GCBDB.')) return;
    var reason = await cypressPrompt('Ban Reason', 'Enter ban reason:', 'Banned by global moderator');
    if (reason === null) return;
    // ban locally first
    send('sendCommand', { pid: selectedInstancePid, cmd: 'Cypress.ModBan "' + playerName + '"' });
    // grab hwid + components - client mod view uses scPlayers, no hwid stored there
    // but sideChannelPeers may have it if this is also a server instance
    var inst = instances[selectedInstancePid];
    var scPeers = inst ? (inst.sideChannelPeers || {}) : {};
    var peer = scPeers[playerName];
    send('modGlobalBanPlayer', {
        pid: selectedInstancePid,
        player: playerName,
        reason: reason || '',
        hwid: peer ? peer.hwid : '',
        components: peer ? (peer.components || []) : [],
        ea_pid: peer ? (peer.eaPid || '') : '',
        account_id: peer ? (peer.accountId || '') : ''
    });
}

function modFreecamPlayer(playerName) {
    if (!selectedInstancePid) return;
    send('sendCommand', { pid: selectedInstancePid, cmd: 'Cypress.ModFreecam "' + playerName + '"' });
}

// add instance / auto-detect

let detectedInstancesList = [];

function showAddInstanceDialog() {
    const overlay = document.getElementById('addInstanceOverlay');
    if (overlay) {
        overlay.style.display = 'flex';
        detectedInstancesList = [];
        updateDetectedList();
        // trigger auto-detect
        send('detectInstances', {});
    }
}

function closeAddInstanceDialog() {
    const overlay = document.getElementById('addInstanceOverlay');
    if (overlay) overlay.style.display = 'none';
}

function manualAttachInstance() {
    const input = document.getElementById('addInstanceAddress');
    if (!input) return;
    const val = input.value.trim();
    if (!val) return;

    // parse address:port
    let address = '127.0.0.1';
    let port = 14638;
    let pid = 0;
    const parts = val.split(':');
    if (parts.length >= 2) {
        address = parts[0] || '127.0.0.1';
        port = parseInt(parts[1]) || 14638;
    } else if (/^\d+$/.test(val)) {
        port = parseInt(val);
    } else {
        address = val;
    }

    send('attachInstance', { address, port, pid });
    closeAddInstanceDialog();
}

function attachDetectedInstance(idx) {
    const inst = detectedInstancesList[idx];
    if (!inst) return;
    send('attachInstance', { address: '127.0.0.1', port: inst.port, pid: inst.pid });
    closeAddInstanceDialog();
}

function refreshDetectedInstances() {
    send('detectInstances', {});
}

function onDetectedInstances(data) {
    detectedInstancesList = data.instances || [];
    updateDetectedList();
}

function updateDetectedList() {
    const container = document.getElementById('detectedInstancesList');
    if (!container) return;

    if (detectedInstancesList.length === 0) {
        container.innerHTML = '<div class="text-muted" style="text-align:center;padding:12px 0;">No untracked Cypress instances found</div>';
        return;
    }

    container.innerHTML = detectedInstancesList.map(function(inst, idx) {
        const typeLabel = inst.isServer ? t('instances.type_server') : t('instances.type_client');
        const gameLabel = { GW1: 'GW1', GW2: 'GW2', BFN: 'BFN' }[inst.game] || escapeHtml(String(inst.game || ''));
        const safeGame = { GW1: 'gw1', GW2: 'gw2', BFN: 'bfn' }[inst.game] || 'gw2';
        return '<div class="detected-instance-entry" onclick="attachDetectedInstance(' + idx + ')">' +
            '<span class="game-pill game-pill-' + safeGame + '">' + gameLabel + '</span> ' +
            '<span>' + typeLabel + '</span>' +
            '<span class="text-muted">PID ' + parseInt(inst.pid, 10) + ' · Port ' + parseInt(inst.port, 10) + '</span>' +
        '</div>';
    }).join('');
}

// mod server controls

let modControlsPopulated = false;

function populateModControls(inst) {
    if (modControlsPopulated && document.getElementById('modLevel').options.length > 0) return;
    modControlsPopulated = true;

    const game = inst.game;
    const gd = typeof GAME_DATA !== 'undefined' ? GAME_DATA[game] : null;
    if (!gd) return;

    // populate level dropdown
    const levelSel = document.getElementById('modLevel');
    levelSel.innerHTML = '';
    let lastCat = '';
    let optgroup = null;
    gd.levels.forEach(lv => {
        if (lv.cat !== lastCat) {
            optgroup = document.createElement('optgroup');
            optgroup.label = lv.cat;
            levelSel.appendChild(optgroup);
            lastCat = lv.cat;
        }
        const opt = document.createElement('option');
        opt.value = (game === 'GW1' && lv.variant !== undefined) ? lv.id + '#' + lv.variant : lv.id;
        opt.textContent = lv.name;
        (optgroup || levelSel).appendChild(opt);
    });

    // show/hide game-specific fields
    const todGroup = document.getElementById('modTodGroup');
    const hostedGroup = document.getElementById('modHostedModeGroup');
    const spGroup = document.getElementById('modStartPointGroup');
    if (todGroup) todGroup.style.display = (game === 'GW2') ? '' : 'none';
    if (hostedGroup) hostedGroup.style.display = (game === 'GW2') ? '' : 'none';
    if (spGroup) spGroup.style.display = (game === 'BFN') ? '' : 'none';

    // render smart picker
    if (typeof renderPickerOptions === 'function') renderPickerOptions('modLevel');

    // populate instance modifiers
    if (typeof populateInstanceModifiers === 'function') populateInstanceModifiers('modModifiers', game);

    onModLevelChanged();
}

function onModLevelChanged() {
    const inst = instances[selectedInstancePid];
    if (!inst) return;
    const gd = typeof GAME_DATA !== 'undefined' ? GAME_DATA[inst.game] : null;
    if (!gd) return;

    const rawVal = document.getElementById('modLevel').value;
    let levelId = rawVal;
    let level;
    if (inst.game === 'GW1' && rawVal.includes('#')) {
        const parsed = typeof parseGW1LevelValue === 'function' ? parseGW1LevelValue(rawVal) : { levelId: rawVal, variant: '0' };
        levelId = parsed.levelId;
        level = gd.levels.find(l => l.id === parsed.levelId && l.variant === parsed.variant);
    } else {
        level = gd.levels.find(l => l.id === levelId);
    }

    // populate mode dropdown with all modes, mark unsupported
    const modeSel = document.getElementById('modMode');
    modeSel.innerHTML = '';
    const supportedModes = level && level.modes ? new Set(level.modes) : null;
    const modeCats = {};
    gd.modes.forEach(m => { if (!modeCats[m.cat]) modeCats[m.cat] = []; modeCats[m.cat].push(m); });
    for (const cat in modeCats) {
        const optgroup = document.createElement('optgroup');
        optgroup.label = cat;
        modeCats[cat].forEach(m => {
            const opt = document.createElement('option');
            opt.value = m.id;
            opt.textContent = m.name;
            optgroup.appendChild(opt);
        });
        modeSel.appendChild(optgroup);
    }

    // populate start points for BFN
    if (inst.game === 'BFN' && gd.startPoints && gd.startPoints.length > 0) {
        const spSel = document.getElementById('modStartPoint');
        spSel.innerHTML = '';
        gd.startPoints.forEach(sp => {
            if (!level) return;
            const levelCat = level.cat;
            const byLevel = sp.levels && sp.levels.includes(levelId);
            const byCat = sp.cats && sp.cats.includes(levelCat);
            const byExtra = sp.extraLevels && sp.extraLevels.includes(levelId);
            const byExclude = sp.excludeLevels && sp.excludeLevels.includes(levelId);
            if (byExclude) return;
            if (byLevel || byCat || byExtra) {
                const opt = document.createElement('option');
                opt.value = sp.id;
                opt.textContent = sp.name;
                spSel.appendChild(opt);
            }
        });
    }

    if (typeof renderPickerOptions === 'function') renderPickerOptions('modMode');
    if (typeof renderPickerOptions === 'function') renderPickerOptions('modStartPoint');
    if (typeof updatePickerTrigger === 'function') updatePickerTrigger('modStartPoint');
    // mark unsupported modes (dimmed, not hidden)
    if (supportedModes) {
        document.querySelectorAll('#modModeOptions .picker-option').forEach(btn => {
            btn.classList.toggle('picker-option-unsupported', !supportedModes.has(btn.getAttribute('data-value')));
        });
    }
    if (typeof updatePickerTrigger === 'function') updatePickerTrigger('modLevel');
}

function onModModeChanged() {
}

function modLoadMap() {
    const inst = instances[selectedInstancePid];
    if (!inst || inst.exited || !inst.isModerator) return;

    const rawLevelVal = document.getElementById('modLevel').value;
    const modeId = document.getElementById('modMode').value;
    if (!rawLevelVal || !modeId) return;

    let levelId = rawLevelVal;
    let variantSuffix = '';
    if (inst.game === 'GW1' && rawLevelVal.includes('#')) {
        const parsed = typeof parseGW1LevelValue === 'function' ? parseGW1LevelValue(rawLevelVal) : { levelId: rawLevelVal, variant: '0' };
        levelId = parsed.levelId;
        variantSuffix = parsed.variant;
    }

    let cmd;
    if (inst.game === 'BFN') {
        const startPoint = document.getElementById('modStartPoint').value || '';
        cmd = 'Server.LoadLevel ' + levelId + ' GameMode=' + modeId + ' ' + startPoint;
    } else {
        let modeStr = (inst.game === 'GW1') ? modeId + variantSuffix : modeId;
        let inclusion = 'GameMode=' + modeStr;
        if (inst.game === 'GW2') {
            const tod = document.getElementById('modTod').value;
            const hosted = document.getElementById('modHostedMode').value;
            inclusion += ';TOD=' + tod + ';HostedMode=' + hosted;
        }
        cmd = 'Server.LoadLevel ' + levelId + ' ' + inclusion;
    }

    send('sendCommand', { pid: selectedInstancePid, cmd: 'Cypress.ModCommand ' + cmd.trim() });
}

function modRestartLevel() {
    var inst = instances[selectedInstancePid];
    if (!inst || !inst.isModerator) return;
    send('sendCommand', { pid: selectedInstancePid, cmd: 'Cypress.ModCommand Server.RestartLevel' });
}

function modApplyModifiers() {
    var inst = instances[selectedInstancePid];
    if (!inst || !inst.isModerator) return;
    const cmds = typeof getInstanceModifierCommands === 'function' ? getInstanceModifierCommands('modModifiers') : [];
    cmds.forEach(cmd => {
        send('sendCommand', { pid: selectedInstancePid, cmd: 'Cypress.ModSetting ' + cmd });
    });
    send('sendCommand', { pid: selectedInstancePid, cmd: 'Cypress.ModCommand Server.RestartLevel' });
}

function modNextPlaylist() {
    var inst = instances[selectedInstancePid];
    if (!inst || !inst.isModerator) return;
    send('sendCommand', { pid: selectedInstancePid, cmd: 'Cypress.ModCommand Server.LoadNextPlaylistSetup' });
}

function modSay() {
    const input = document.getElementById('modSayInput');
    const msg = input.value.trim();
    if (!msg || !selectedInstancePid) return;
    const duration = document.getElementById('modSayDuration').value;
    send('sendCommand', { pid: selectedInstancePid, cmd: 'Cypress.ModCommand Server.Say ' + msg + ' ' + duration });
    input.value = '';
}

// anticheat toggles - send command on change
document.addEventListener('change', function(e) {
    if (!selectedInstancePid) return;
    const cb = e.target;
    if (cb.classList.contains('ac-srv-toggle')) {
        const setting = cb.getAttribute('data-ac');
        const val = cb.checked ? 'true' : 'false';
        send('sendCommand', { pid: selectedInstancePid, cmd: 'Cypress.SetAnticheat ' + setting + ' ' + val });
    }
    if (cb.classList.contains('ac-mod-toggle')) {
        const modInst = instances[selectedInstancePid];
        if (!modInst || !modInst.isModerator) return;
        const setting = cb.getAttribute('data-ac');
        const val = cb.checked ? 'true' : 'false';
        send('sendCommand', { pid: selectedInstancePid, cmd: 'Cypress.ModCommand Cypress.SetAnticheat ' + setting + ' ' + val });
    }
});

// mod bans
function modPreBan() {
    var name = (document.getElementById('modPrebanName').value || '').trim();
    var reason = (document.getElementById('modPrebanReason').value || '').trim();
    if (!name || !selectedInstancePid) return;
    send('sendCommand', { pid: selectedInstancePid, cmd: 'Cypress.ModCommand Server.AddBan "' + name + '"' + (reason ? ' ' + reason : '') });
    document.getElementById('modPrebanName').value = '';
    document.getElementById('modPrebanReason').value = '';
    setTimeout(modFetchBans, 400);
}

function modUnbanPlayer(name) {
    if (!selectedInstancePid) return;
    send('sendCommand', { pid: selectedInstancePid, cmd: 'Cypress.ModCommand Server.UnbanPlayer "' + name + '"' });
    setTimeout(modFetchBans, 400);
}

function modFetchBans() {
    if (!selectedInstancePid) return;
    send('sendCommand', { pid: selectedInstancePid, cmd: 'Cypress.ModCommand Cypress.GetBans' });
}

function onModBansResult(data) {
    var list = document.getElementById('modBanList');
    var empty = document.getElementById('modBanListEmpty');
    if (!list) return;
    var bans = data.bans || [];
    list.querySelectorAll('.player-card').forEach(function(el) { el.remove(); });
    if (bans.length === 0) {
        if (empty) empty.style.display = '';
        return;
    }
    if (empty) empty.style.display = 'none';
    bans.forEach(function(ban) {
        var names = (ban.Names || []).filter(Boolean);
        var primaryName = names[0] || '(unknown)';
        var allNames = names.join(', ') || '(unknown)';
        var reason = ban.BanReason || '';
        var hwid = ban.MachineId || '';
        var hasHw = hwid && hwid !== 'UNIQUEID' && hwid.length > 4;
        var card = document.createElement('div');
        card.className = 'player-card';
        card.innerHTML =
            '<div class="player-card-avatar"><svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><line x1="4.93" y1="4.93" x2="19.07" y2="19.07"/></svg></div>' +
            '<div class="player-card-info">' +
                '<div class="player-card-name" title="' + escapeHtml(allNames) + '">' + escapeHtml(primaryName) + (names.length > 1 ? ' <span style="color:var(--text-muted);font-weight:400;font-size:11px;">+' + (names.length - 1) + ' alias</span>' : '') + '</div>' +
                '<div class="player-card-meta">' +
                    (reason ? escapeHtml(reason) : '<span style="opacity:0.4">' + escapeHtml(t('instances.no_reason')) + '</span>') +
                    (hasHw ? ' &nbsp;·&nbsp; <span title="' + escapeHtml(hwid) + '">' + escapeHtml(hwid.substring(0, 8)) + '…</span>' : ' &nbsp;·&nbsp; <span style="opacity:0.4">' + escapeHtml(t('instances.no_hw')) + '</span>') +
                '</div>' +
            '</div>' +
            '<div class="player-card-actions">' +
                '<button class="icon-btn icon-btn-small icon-btn-danger" data-unban="' + escapeHtml(primaryName) + '" title="' + escapeHtml(t('instances.btn_unban')) + '">' +
                '<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="11" width="18" height="11" rx="2" ry="2"/><path d="M7 11V7a5 5 0 0 1 9.9-1"/></svg></button>' +
            '</div>';
        card.querySelector('[data-unban]').addEventListener('click', function() {
            modUnbanPlayer(this.getAttribute('data-unban'));
        });
        list.appendChild(card);
    });
}

// mod history (reuses client-side playerHistory from scPlayerJoin/scPlayerLeave)
function updateModHistoryTab() {
    var inst = selectedInstancePid ? instances[selectedInstancePid] : null;
    var list = document.getElementById('modHistoryList');
    var empty = document.getElementById('modHistoryEmpty');
    if (!list) return;
    list.querySelectorAll('.history-row').forEach(function(el) { el.remove(); });
    var history = (inst && inst.playerHistory) ? inst.playerHistory : [];
    if (history.length === 0) {
        if (empty) empty.style.display = '';
        return;
    }
    if (empty) empty.style.display = 'none';
    var archive = inst.peerArchive || {};
    var isGlobalMod = typeof modLoggedIn !== 'undefined' && modLoggedIn;
    var sorted = history.slice().reverse();
    sorted.forEach(function(entry) {
        var d = new Date(entry.ts);
        var time = d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
        var isJoin = entry.event === 'join';
        var peer = archive[entry.name];

        var row = document.createElement('div');
        row.className = 'history-row';
        row.style.cssText = 'display:flex;flex-direction:column;padding:3px 8px;border-radius:4px;font-size:12px;';

        var header = document.createElement('div');
        header.style.cssText = 'display:flex;align-items:center;gap:8px;cursor:' + (peer ? 'pointer' : 'default') + ';';
        header.innerHTML =
            (peer ? '<svg class="history-chevron" width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="var(--text-muted)" stroke-width="2.5" style="transition:transform .15s;flex-shrink:0;"><polyline points="9 6 15 12 9 18"/></svg>' : '<span style="width:10px;flex-shrink:0;"></span>') +
            '<span style="color:var(--text-muted);flex-shrink:0;font-family:monospace;">' + escapeHtml(time) + '</span>' +
            '<svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="' + (isJoin ? '#4caf50' : 'var(--text-muted)') + '" stroke-width="2.5">' +
                (isJoin ? '<polyline points="5 12 19 12"/><polyline points="13 6 19 12 13 18"/>' : '<polyline points="19 12 5 12"/><polyline points="11 18 5 12 11 6"/>') +
            '</svg>' +
            '<span style="flex:1;font-weight:500;">' + escapeHtml(entry.name) + '</span>' +
            (entry.reason ? '<span style="color:var(--text-muted);font-size:11px;">' + escapeHtml(entry.reason) + '</span>' : '');
        row.appendChild(header);

        if (peer) {
            var details = document.createElement('div');
            details.className = 'history-details';
            details.style.cssText = 'display:none;margin:4px 0 2px 18px;padding:6px 10px;background:var(--bg-tertiary);border-radius:4px;font-size:11px;line-height:1.6;color:var(--text-secondary);';

            var lines = [];
            if (peer.displayName && peer.displayName !== entry.name)
                lines.push('<b>display</b>: ' + escapeHtml(peer.displayName));
            if (peer.nickname)
                lines.push('<b>nickname</b>: ' + escapeHtml(peer.nickname));
            if (peer.username)
                lines.push('<b>username</b>: ' + escapeHtml(peer.username));
            if (peer.accountId)
                lines.push('<b>account</b>: <span style="user-select:all;">' + escapeHtml(peer.accountId) + '</span>');
            if (isGlobalMod && peer.eaPid)
                lines.push('<b>pid</b>: <span style="user-select:all;">' + escapeHtml(peer.eaPid) + '</span>');
            if (peer.hwid)
                lines.push('<b>hwid</b>: <span style="user-select:all;">' + escapeHtml(peer.hwid) + '</span>');
            if (peer.components && peer.components.length)
                lines.push('<b>components</b>: <span style="user-select:all;word-break:break-all;">' + escapeHtml(peer.components.join(', ')) + '</span>');

            if (lines.length) {
                details.innerHTML = lines.join('<br>');
                row.appendChild(details);

                header.addEventListener('click', function() {
                    var open = details.style.display !== 'none';
                    details.style.display = open ? 'none' : 'block';
                    var chev = header.querySelector('.history-chevron');
                    if (chev) chev.style.transform = open ? '' : 'rotate(90deg)';
                });
            }
        }

        list.appendChild(row);
    });
}

// mod remote console
function modSendConsole() {
    var inst = instances[selectedInstancePid];
    if (!inst || !inst.isModerator) return;
    var input = document.getElementById('modConsoleInput');
    var cmd = (input.value || '').trim();
    if (!cmd) return;
    send('sendCommand', { pid: selectedInstancePid, cmd: 'Cypress.ModCommand ' + cmd });
    input.value = '';
}
