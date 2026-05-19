let manualMode = false;
let isApplyingBackendState = false;

const RELAY_PRESETS = {
    custom: {
        placeholder: 'relay.your-vps.example:25200',
        hint: 'Point this to your UDP relay host. Hostnames and host:port are supported.'
    },
    template: {
        placeholder: 'relay.example.com:25200',
        hint: 'Template only. Replace it with your VPS host or domain before launching.'
    }
};

let detectedDeviceIP = '';

// convert base64 to blob object URL so we can discard the raw string
function base64ToBlobUrl(base64, mime) {
    try {
        var bin = atob(base64);
        var arr = new Uint8Array(bin.length);
        for (var i = 0; i < bin.length; i++) arr[i] = bin.charCodeAt(i);
        return URL.createObjectURL(new Blob([arr], { type: mime || 'image/jpeg' }));
    } catch (e) { return ''; }
}

// custom dialog system (replaces native alert/confirm/prompt)
var _cypressDialogResolve = null;
var _cypressDialogMode = 'alert'; // alert | confirm | prompt

function _showCypressDialog(title, body, mode, defaultVal) {
    _cypressDialogMode = mode;
    var overlay = document.getElementById('cypressDialogOverlay');
    document.getElementById('cypressDialogTitle').textContent = title;
    document.getElementById('cypressDialogBody').textContent = body || '';
    var inputWrap = document.getElementById('cypressDialogInputWrap');
    var input = document.getElementById('cypressDialogInput');
    var cancelBtn = document.getElementById('cypressDialogCancelBtn');

    if (mode === 'prompt') {
        inputWrap.style.display = '';
        input.value = defaultVal || '';
        cancelBtn.style.display = '';
    } else if (mode === 'confirm') {
        inputWrap.style.display = 'none';
        cancelBtn.style.display = '';
    } else {
        inputWrap.style.display = 'none';
        cancelBtn.style.display = 'none';
    }

    overlay.style.display = 'flex';
    if (mode === 'prompt') input.focus();

    return new Promise(function(resolve) { _cypressDialogResolve = resolve; });
}

function cypressDialogOk() {
    var overlay = document.getElementById('cypressDialogOverlay');
    overlay.style.display = 'none';
    if (_cypressDialogResolve) {
        if (_cypressDialogMode === 'prompt')
            _cypressDialogResolve(document.getElementById('cypressDialogInput').value);
        else
            _cypressDialogResolve(true);
        _cypressDialogResolve = null;
    }
}

function cypressDialogCancel() {
    var overlay = document.getElementById('cypressDialogOverlay');
    overlay.style.display = 'none';
    if (_cypressDialogResolve) {
        _cypressDialogResolve(_cypressDialogMode === 'prompt' ? null : false);
        _cypressDialogResolve = null;
    }
}

// enter to confirm in dialog
document.addEventListener('keydown', function(e) {
    var overlay = document.getElementById('cypressDialogOverlay');
    if (!overlay || overlay.style.display === 'none') return;
    if (e.key === 'Enter') { e.preventDefault(); cypressDialogOk(); }
    if (e.key === 'Escape') { e.preventDefault(); cypressDialogCancel(); }
});

function cypressAlert(title, body) { return _showCypressDialog(title, body, 'alert'); }
function cypressConfirm(title, body) { return _showCypressDialog(title, body, 'confirm'); }
function cypressPrompt(title, body, defaultVal) { return _showCypressDialog(title, body, 'prompt', defaultVal); }


function send(type, data) {
    window.external.sendMessage(JSON.stringify({ type, ...data }));
}
window.external.receiveMessage(function (msg) {
    try {
    const data = JSON.parse(msg);
    switch (data.type) {
        case 'translations':
            window._i18nStrings = data.strings || {};
            (function() {
                var meta = window._i18nStrings['_meta'];
                document.documentElement.dir = (meta && meta.rtl) ? 'rtl' : 'ltr';
                if (data.lang) document.documentElement.lang = data.lang;
                if (meta && typeof meta === 'object' && meta.name && data.lang) {
                    var sel = document.getElementById('languageSelect');
                    if (sel) {
                        for (var i = 0; i < sel.options.length; i++) {
                            if (sel.options[i].value === data.lang) {
                                sel.options[i].textContent = meta.name + ' (' + data.lang + ')';
                                break;
                            }
                        }
                    }
                }
            })();
            if (typeof applyDomTranslations === 'function') applyDomTranslations();
            break;
        case 'translationsList':
            if (typeof onTranslationsList === 'function') onTranslationsList(data);
            break;
        case 'tosStatus':
            if (data.accepted) {
                send('checkAuth', {});
            } else {
                document.getElementById('tosModalBackdrop').style.display = 'flex';
            }
            break;
        case 'authStatus': handleAuthStatus(data); break;
        case 'authLoginResult': handleAuthLoginResult(data); break;
        case 'authLogoutResult': handleAuthLogoutResult(data); break;
        case 'identityStatus':
            if (typeof handleIdentityStatus === 'function') handleIdentityStatus(data);
            break;
        case 'registerResult':
            if (typeof handleRegisterResult === 'function') handleRegisterResult(data);
            break;
        case 'nicknameResult':
            if (typeof handleNicknameResult === 'function') handleNicknameResult(data);
            break;
        case 'refreshEntitlementsResult':
            if (typeof handleRefreshEntitlementsResult === 'function') handleRefreshEntitlementsResult(data);
            break;
        case 'relinkEAResult':
            if (typeof handleRelinkEAResult === 'function') handleRelinkEAResult(data);
            break;
        case 'status': showStatus(data.text, data.level || 'info'); break;
        case 'gameDir': setGameDir(data.path); break;
        case 'loadUserData': loadUserData(data); break;
        case 'modPacks': populateSelect('modPackSelect', data.packs); break;
        case 'playlists':
            populateSelect('playlistSelect', data.files);
            if (typeof populatePlaylistFileSelect === 'function') populatePlaylistFileSelect(data.files);
            break;
        case 'relayLease': applyRelayLease(data); break;
        case 'relayResolved': onRelayResolved(data); break;
        case 'windowDragStart': if (window.onWindowDragStart) window.onWindowDragStart(data); break;
        case 'mapBg':
            if (data.key && data.data) {
                MAP_BG_CACHE[data.key] = base64ToBlobUrl(data.data);
                updateChangedSettingsIndicator();
                if (typeof updateInstanceList === 'function') updateInstanceList();
                if (typeof updateSrvOverride === 'function') updateSrvOverride();
                if (typeof updatePickerOptionBgs === 'function') updatePickerOptionBgs(data.key);
                if (typeof filterBrowserList === 'function') filterBrowserList();
            }
            break;
        case 'modeBg':
            if (data.key && data.data) {
                MODE_BG_CACHE[data.key] = base64ToBlobUrl(data.data);
                if (typeof updateModePickerOptionBgs === 'function') updateModePickerOptionBgs(data.key);
                if (typeof filterBrowserList === 'function') filterBrowserList();
            }
            break;
        case 'aiSetBg':
            if (data.key && data.data) {
                AI_SET_BG_CACHE[data.key] = base64ToBlobUrl(data.data);
                if (typeof updateAiSetPickerBgs === 'function') updateAiSetPickerBgs(data.key);
            }
            break;
        case 'charIcon':
            if (data.key && data.data) {
                CHAR_ICON_CACHE[data.key] = base64ToBlobUrl(data.data, 'image/png');
                document.querySelectorAll('img[data-icon-key="' + CSS.escape(data.key) + '"').forEach(function(img) {
                    img.src = CHAR_ICON_CACHE[data.key];
                    img.classList.remove('char-icon-pending');
                });
            }
            break;
        case 'instanceStarted':
        case 'instanceOutput':
        case 'instanceExited':
        case 'instances':
            handleInstanceMessage(data);
            break;
        case 'serverInfo':
            if (typeof onServerInfoResult === 'function') onServerInfoResult(data);
            break;
        case 'browserList':
            if (typeof onBrowserList === 'function') onBrowserList(data);
            break;
        case 'browserIcon':
            if (typeof onBrowserIcon === 'function') onBrowserIcon(data);
            break;
        case 'detectedInstances':
            if (typeof onDetectedInstances === 'function') onDetectedInstances(data);
            break;
        case 'modLoginResult':
            if (typeof onModLoginResult === 'function') onModLoginResult(data);
            break;
        case 'modLogoutResult':
            if (typeof onModLogoutResult === 'function') onModLogoutResult(data);
            break;
        case 'modGlobalBanResult':
            if (typeof onModGlobalBanResult === 'function') onModGlobalBanResult(data);
            break;
        case 'modGlobalUnbanResult':
            if (typeof onModGlobalUnbanResult === 'function') onModGlobalUnbanResult(data);
            break;
        case 'modGlobalBansList':
            if (typeof onModGlobalBansList === 'function') onModGlobalBansList(data);
            break;
        case 'modBanByPidResult':
            if (typeof onModBanByPidResult === 'function') onModBanByPidResult(data);
            break;
        case 'modBanServerResult':
            if (typeof onModBanServerResult === 'function') onModBanServerResult(data);
            break;
        case 'modBanServerByKeyResult':
            if (typeof onModBanServerByKeyResult === 'function') onModBanServerByKeyResult(data);
            break;
        case 'modUnbanServerResult':
            if (typeof onModUnbanServerResult === 'function') onModUnbanServerResult(data);
            break;
        case 'modBannedServersList':
            if (typeof onModBannedServersList === 'function') onModBannedServersList(data);
            break;
        case 'modPinServerResult':
            if (typeof onModPinServerResult === 'function') onModPinServerResult(data);
            break;
        case 'modUnpinServerResult':
            if (typeof onModUnpinServerResult === 'function') onModUnpinServerResult(data);
            break;
        case 'localBansResult':
            if (typeof onLocalBansResult === 'function') onLocalBansResult(data);
            break;
        case 'updateCheckResult':
            if (typeof onUpdateCheckResult === 'function') onUpdateCheckResult(data);
            break;
        case 'updateProgress':
            if (typeof onUpdateProgress === 'function') onUpdateProgress(data);
            break;
        case 'updateComplete':
            if (typeof onUpdateComplete === 'function') onUpdateComplete(data);
            break;
        case 'updateError':
            if (typeof onUpdateError === 'function') onUpdateError(data);
            break;
        case 'playlistSaved':
            if (typeof onPlaylistSaved === 'function') onPlaylistSaved(data);
            break;
        case 'playlistLoaded':
            if (typeof onPlaylistLoaded === 'function') onPlaylistLoaded(data);
            break;
        case 'playlistDeleted':
            if (typeof onPlaylistDeleted === 'function') onPlaylistDeleted(data);
            break;
    }
    } catch (e) {
        console.error('receiveMessage error:', e);
        showStatus('JS Error: ' + e.message + ' | type=' + (data && data.type) + ' | ' + (e.stack || '').split('\n').slice(0,3).join(' >> '), 'error');
    }
});

(function() {
    var dragging = false;
    var startScreenX = 0, startScreenY = 0;
    var startWinX = 0, startWinY = 0;

    var dragEl = document.getElementById('titlebar');
    if (!dragEl) return;

    dragEl.addEventListener('mousedown', function(e) {
        if (e.button !== 0) return;
        if (e.target.closest('.titlebar-controls')) return;
        dragging = true;
        startScreenX = e.screenX;
        startScreenY = e.screenY;
        send('windowDragStart');
        e.preventDefault();
    });

    window.onWindowDragStart = function(data) {
        startWinX = data.windowX;
        startWinY = data.windowY;
    };

    document.addEventListener('mousemove', function(e) {
        if (!dragging) return;
        var dx = e.screenX - startScreenX;
        var dy = e.screenY - startScreenY;
        send('windowDragMove', { x: startWinX + dx, y: startWinY + dy });
    });

    document.addEventListener('mouseup', function() {
        dragging = false;
    });
})();

function populateSelect(id, items) {
    const sel = document.getElementById(id);
    const first = sel.options[0]?.text || '';
    sel.innerHTML = '<option value="">' + first + '</option>';
    (items || []).forEach(p => { const o = document.createElement('option'); o.value = p; o.textContent = p; sel.appendChild(o); });
    if (typeof renderPickerOptions === 'function' && typeof PICKER_REGISTRY !== 'undefined' && PICKER_REGISTRY[id]) {
        renderPickerOptions(id);
        updatePickerTrigger(id);
    }
}

function acceptTos() {
    document.getElementById('tosModalBackdrop').style.display = 'none';
    send('acceptTos', {});
    send('checkAuth', {});
}
