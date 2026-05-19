function switchTab(tab) {
    document.querySelectorAll('.tab-content').forEach(el => el.classList.remove('active'));
    document.querySelectorAll('.nav-btn').forEach(el => el.classList.remove('active'));
    document.getElementById('tab-' + tab).classList.add('active');
    var navBtn = document.querySelector('[data-tab="' + tab + '"]');
    if (navBtn) navBtn.classList.add('active');
    // profile widget highlight
    var pw = document.getElementById('profileWidget');
    if (pw) pw.classList.toggle('active', tab === 'profile');
    if (tab === 'profile' && typeof syncProfileDisplay === 'function') syncProfileDisplay();
    if (tab === 'profile') send('getTranslationsList', {});
    if (tab === 'docs') switchDoc('quickstart');
    if (tab === 'instances') send('getInstances');
    if (tab === 'join' && typeof pingAllServers === 'function') pingAllServers();
    if (tab === 'playlists') send('getPlaylists', {});
    if (tab === 'browser') {
        refreshBrowser();
        if (typeof startBrowserAutoRefresh === 'function') startBrowserAutoRefresh();
    } else {
        if (typeof stopBrowserAutoRefresh === 'function') stopBrowserAutoRefresh();
    }
    syncFloatingFooter(tab);
}

// host sub-tabs (network, map & mode, modifiers, anticheat, playlist)
function switchHostTab(tab) {
    document.querySelectorAll('.host-sub-content').forEach(el => el.classList.remove('active'));
    document.querySelectorAll('.host-sub-tab').forEach(el => el.classList.remove('active'));
    document.getElementById('htab-' + tab).classList.add('active');
    document.querySelector('.host-sub-tab[data-htab="' + tab + '"]').classList.add('active');
}

// instance server sub-tabs (map, modifiers, anticheat), shared by srv and mod scopes
function switchSrvSubTab(scope, tab) {
    document.querySelectorAll('.srv-sub-content[data-stab-scope="' + scope + '"]').forEach(el => el.classList.remove('active'));
    var parent = document.getElementById('stab-' + scope + '-' + tab);
    if (parent) parent.classList.add('active');
    var tabBar = parent ? parent.closest('.srv-ctrl-scroll')?.previousElementSibling : null;
    if (tabBar && tabBar.classList.contains('srv-sub-tabs')) {
        tabBar.querySelectorAll('.srv-sub-tab').forEach(el => el.classList.remove('active'));
        tabBar.querySelector('.srv-sub-tab[data-stab="' + tab + '"]')?.classList.add('active');
    }
    if (tab === 'bans' && typeof fetchLocalBans === 'function') fetchLocalBans();
    if (tab === 'history' && typeof updateHistoryTab === 'function') updateHistoryTab();
}

function syncFloatingFooter(tab) {
    const currentTab = tab || document.querySelector('.tab-content.active')?.id?.replace('tab-', '') || 'join';
    const footer = document.getElementById('floatingActionFooter');
    const joinBtn = document.getElementById('floatingJoinButton');
    const hostBtn = document.getElementById('floatingHostButton');
    const playlistBtn = document.getElementById('floatingPlaylistButton');
    const playlistCopyBtn = document.getElementById('floatingPlaylistCopyButton');
    const playlistSaveBtn = document.getElementById('floatingPlaylistSaveButton');

    if (!footer) {
        return;
    }

    [joinBtn, hostBtn, playlistBtn, playlistCopyBtn, playlistSaveBtn].forEach(button => {
        if (button) {
            button.style.display = 'none';
        }
    });

    if (currentTab === 'join') {
        footer.classList.remove('hidden');
        joinBtn.style.display = 'inline-flex';
        return;
    }

    if (currentTab === 'host') {
        footer.classList.remove('hidden');
        hostBtn.style.display = 'inline-flex';
        return;
    }

    if (currentTab === 'playlists') {
        footer.classList.remove('hidden');
        playlistBtn.style.display = 'inline-flex';
        playlistCopyBtn.style.display = 'inline-flex';
        playlistSaveBtn.style.display = 'inline-flex';
        return;
    }

    footer.classList.add('hidden');
}

function getGame() { return document.getElementById('gameSelector').value; }

function getGameLabels() {
    return {
        'GW1': t('game.gw1'),
        'GW2': t('game.gw2'),
        'BFN': t('game.bfn')
    };
}

function toggleGamePicker() {
    document.getElementById('gamePicker').classList.toggle('open');
}

function selectGame(game) {
    document.getElementById('gameSelector').value = game;
    document.getElementById('gamePickerLabel').textContent = getGameLabels()[game] || game;
    const selectedOption = document.querySelector('.game-picker-option[data-game="' + game + '"]');
    if (selectedOption) {
        const iconSrc = selectedOption.querySelector('img').src;
        document.getElementById('gamePickerIcon').src = iconSrc;
    }
    document.querySelectorAll('.game-picker-option').forEach(opt => {
        opt.classList.toggle('selected', opt.dataset.game === game);
    });
    document.getElementById('gamePicker').classList.remove('open');
    onGameChanged();
}

document.addEventListener('click', function(e) {
    const picker = document.getElementById('gamePicker');
    if (picker && !picker.contains(e.target)) {
        picker.classList.remove('open');
    }
});

function getGameInfo() {
    return {
        'GW1': { title: t('game.gw1') },
        'GW2': { title: t('game.gw2') },
        'BFN': { title: t('game.bfn') }
    };
}

function updateGameBranding() {
    const game = getGame();
    const bgName = document.getElementById('gameBgName');
    const bgIcon = document.getElementById('gameBgIcon');
    if (bgName) bgName.textContent = game;

    const selectedOption = document.querySelector('.game-picker-option[data-game="' + game + '"]');
    const iconSrc = selectedOption ? selectedOption.querySelector('img').src : '';
    if (bgIcon && iconSrc) bgIcon.src = iconSrc;

    const bgData = document.getElementById('bgData' + game);
    const bgSrc = bgData ? bgData.src : '';

    const gameInfo = getGameInfo();
    const info = gameInfo[game] || gameInfo['GW2'];
    ['join', 'host'].forEach(function(tab) {
        var bgEl = document.getElementById(tab + 'GameInfoBg');
        var iconEl = document.getElementById(tab + 'GameInfoIcon');
        var titleEl = document.getElementById(tab + 'GameInfoTitle');
        if (bgEl && bgSrc) bgEl.src = bgSrc;
        if (iconEl && iconSrc) iconEl.src = iconSrc;
        if (titleEl) titleEl.textContent = info.title;
    });
}

function onGameChanged() {
    const game = getGame();
    const isBFN = game === 'BFN';
    const isGW1 = game === 'GW1';
    var fovGroup = document.getElementById('fovGroup');
    if (fovGroup) fovGroup.style.display = isGW1 ? 'none' : '';
    document.getElementById('username').maxLength = isBFN ? 16 : 32;
    var usernameHint = document.getElementById('usernameHint');
    if (usernameHint) usernameHint.textContent = isBFN ? t('tabs.username_hint_bfn') : t('tabs.username_hint_default');
    document.getElementById('aiBackfillGroup').style.display = isBFN ? '' : 'none';
    document.getElementById('hostedModeGroup').style.display = (game === 'GW2') ? '' : 'none';
    document.getElementById('hostAnticheatTab').style.display = (game === 'GW2') ? '' : 'none';
    // if switching away from GW2 while on anticheat tab, go back to network
    if (game !== 'GW2') {
        var activeHtab = document.querySelector('.host-sub-tab.active');
        if (activeHtab && activeHtab.dataset.htab === 'anticheat') switchHostTab('network');
    }
    document.getElementById('startPointGroup').style.display = isBFN ? '' : 'none';
    document.getElementById('startPointValueGroup').style.display = isBFN ? '' : 'none';
    document.getElementById('variantGroup').style.display = isGW1 ? '' : 'none';
    document.getElementById('levelPickerLabel').textContent = isBFN ? t('tabs.dsub_label') : t('mapmode.level_label');
    populateLevelPicker(true);
    populateModePicker();
    if (isBFN) populateStartPointPicker();
    syncPickerCompatibility();
    populateModifierToggles();
    onInclusionChanged();
    syncSegmentedGroup('todPicker');
    syncSegmentedGroup('hostedModePicker');
    syncRelayUi('join');
    syncRelayUi('host');
    syncFloatingFooter();
    updateGameBranding();
    if (!isApplyingBackendState) {
        send('gameChanged', { game });
    }
}
