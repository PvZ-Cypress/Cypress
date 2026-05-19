// ea auth login screen logic

window._eaLoggedIn = false;
window._identityOwnedGames = [];
window._identityEaRelinked = false;

function updateProfileIdentitySection() {
    var loggedIn = !!window._eaLoggedIn;
    var signedOutBlock = document.getElementById('profileSignedOutBlock');
    var signedInBlock = document.getElementById('profileSignedInBlock');
    var usernameField = document.getElementById('username');
    var usernameHint = document.getElementById('usernameHint');

    if (signedOutBlock) signedOutBlock.style.display = loggedIn ? 'none' : '';
    if (signedInBlock) signedInBlock.style.display = loggedIn ? '' : 'none';

    if (usernameField) {
        if (loggedIn) {
            usernameField.readOnly = true;
            usernameField.oninput = null;
            usernameField.placeholder = t('auth.not_registered_placeholder');
            if (usernameHint) usernameHint.textContent = t('auth.identity_username_hint');
        } else {
            usernameField.readOnly = false;
            usernameField.oninput = onProfileFieldChanged;
            usernameField.placeholder = t('profile.display_name_placeholder');
            if (usernameHint) usernameHint.textContent = t('auth.display_name_hint');
        }
    }
    updateGameLibrarySection();
}

var _GAME_LIBRARY_DEFS = [
    { name: 'Garden Warfare 1', iconId: 'iconDataGW1', label: 'GW1' },
    { name: 'Garden Warfare 2', iconId: 'iconDataGW2', label: 'GW2' },
    { name: 'Battle for Neighborville', iconId: 'iconDataBFN', label: 'BFN' }
];

function updateGameLibrarySection() {
    var list = document.getElementById('gameLibraryList');
    if (!list) return;
    var loggedIn = !!window._eaLoggedIn;
    if (!loggedIn) {
        list.innerHTML = '<div class="game-library-empty">' + (window._i18nStrings['profile.library_sign_in'] || 'Sign in to see your owned games') + '</div>';
        return;
    }
    var owned = window._identityOwnedGames || [];
    list.innerHTML = '';
    _GAME_LIBRARY_DEFS.forEach(function(def) {
        var isOwned = owned.indexOf(def.name) !== -1;
        var iconEl = document.getElementById(def.iconId);
        var iconSrc = iconEl ? iconEl.src : '';
        var card = document.createElement('div');
        card.className = 'game-library-card ' + (isOwned ? 'owned' : 'not-owned');
        card.innerHTML =
            (iconSrc ? '<img src="' + iconSrc + '" alt="' + def.label + '" draggable="false">' : '') +
            '<div class="game-library-card-name">' + def.label + '</div>' +
            (isOwned ? '<div class="game-library-badge">' + (window._i18nStrings['profile.library_owned'] || 'Owned') + '</div>' : '');
        list.appendChild(card);
    });
}

function _updateIdentityButtons() {
    var games = window._identityOwnedGames || [];
    var relinked = !!window._identityEaRelinked;

    var refreshGroup = document.getElementById('refreshLicensesGroup');
    var relinkGroup = document.getElementById('relinkEaGroup');

    // show refresh licenses only if they dont have all 3 games
    if (refreshGroup) refreshGroup.style.display = (games.length < 3) ? '' : 'none';
    // show relink ea only if not already relinked
    if (relinkGroup) relinkGroup.style.display = relinked ? 'none' : '';
}

function showAuthModal() {
    document.getElementById('authModalBackdrop').style.display = 'flex';
    document.getElementById('authStatus').textContent = '';
    document.getElementById('authStatus').className = 'auth-status';
    document.getElementById('authLoginBtn').disabled = false;
}

function hideAuthModal() {
    document.getElementById('authModalBackdrop').style.display = 'none';
}

function startEaLogin() {
    const btn = document.getElementById('authLoginBtn');
    const status = document.getElementById('authStatus');
    btn.disabled = true;
    status.textContent = t('auth.opening_ea');
    status.className = 'auth-status waiting';
    send('eaLogin');
}

function handleAuthStatus(data) {
    window._eaLoggedIn = !!data.loggedIn;
    updateProfileIdentitySection();
    if (data.loggedIn) {
        hideAuthModal();
        if (data.uid) setAvatarImage(data.uid);
    } else {
        showAuthModal(); // dismissable
    }
    send('init', {}); // load settings regardless of login state
}

function handleAuthLoginResult(data) {
    const btn = document.getElementById('authLoginBtn');
    const status = document.getElementById('authStatus');

    if (data.ok) {
        window._eaLoggedIn = true;
        updateProfileIdentitySection();
        status.textContent = t('auth.logged_in_as', { name: data.displayName });
        status.className = 'auth-status';
        if (data.uid) setAvatarImage(data.uid);
        hideAuthModal();
        send('init', {});
    } else {
        btn.disabled = false;
        status.textContent = data.error || t('auth.login_failed');
        status.className = 'auth-status error';
    }
}

function handleAuthLogoutResult(data) {
    if (data.ok) {
        window._eaLoggedIn = false;
        updateProfileIdentitySection();
        showAuthModal();
    }
}

// cypress identity registration

function handleIdentityStatus(data) {
    if (data.registered) {
        window._identityOwnedGames = data.ownedGames || [];
        window._identityEaRelinked = !!data.eaRelinked;
        _updateIdentityButtons();
        updateGameLibrarySection();
        hideIdentityModal();
        const usernameField = document.getElementById('username');
        if (usernameField) usernameField.value = data.username || '';
        const nicknameField = document.getElementById('nicknameInput');
        if (nicknameField) nicknameField.value = data.nickname || '';
        if (typeof syncProfileDisplay === 'function') syncProfileDisplay();
    } else if (window._eaLoggedIn) {
        // not registered: confirmation with EA name w owned games
        var eaName = data.eaDisplayName || '';
        var games = data.ownedGames || [];
        showIdentityModal(eaName, games);
    }
}

function showIdentityModal(eaDisplayName, ownedGames) {
    // reset to step 1
    document.getElementById('identityConfirmStep').style.display = '';
    document.getElementById('identityUsernameStep').style.display = 'none';
    document.getElementById('identityConfirmStatus').textContent = '';
    document.getElementById('identityConfirmStatus').className = 'auth-status';
    document.getElementById('identityConfirmBtn').disabled = false;

    var nameEl = document.getElementById('identityConfirmEaName');
    if (nameEl) nameEl.textContent = eaDisplayName || '(unknown)';

    var listEl = document.getElementById('identityConfirmGamesList');
    if (listEl) {
        listEl.innerHTML = '';
        if (ownedGames && ownedGames.length > 0) {
            ownedGames.forEach(function(g) {
                var li = document.createElement('li');
                li.textContent = g;
                listEl.appendChild(li);
            });
        } else {
            var li = document.createElement('li');
            li.textContent = t('auth.no_games_found');
            li.style.color = 'var(--text-muted, #999)';
            listEl.appendChild(li);
        }
    }

    document.getElementById('identityModalBackdrop').style.display = 'flex';
}

function hideIdentityModal() {
    document.getElementById('identityModalBackdrop').style.display = 'none';
}

function proceedToUsernameStep() {
    document.getElementById('identityConfirmStep').style.display = 'none';
    var usernameStep = document.getElementById('identityUsernameStep');
    usernameStep.style.display = '';
    document.getElementById('identityRegStatus').textContent = '';
    document.getElementById('identityRegStatus').className = 'auth-status';
    document.getElementById('identityRegisterBtn').disabled = false;
    var input = document.getElementById('identityUsernameInput');
    input.value = '';
    setTimeout(function() { input.focus(); }, 100);
}

function submitIdentityRegistration() {
    const input = document.getElementById('identityUsernameInput');
    const username = input.value.trim();
    const status = document.getElementById('identityRegStatus');
    const btn = document.getElementById('identityRegisterBtn');

    if (username.length < 3 || username.length > 32) {
        status.textContent = t('auth.username_length_error');
        status.className = 'auth-status error';
        return;
    }
    if (!/^[a-zA-Z0-9][a-zA-Z0-9 _-]*[a-zA-Z0-9]$/.test(username) && !/^[a-zA-Z0-9]$/.test(username)) {
        status.textContent = t('auth.username_chars_error');
        status.className = 'auth-status error';
        return;
    }

    btn.disabled = true;
    status.textContent = t('auth.registering');
    status.className = 'auth-status waiting';
    send('registerIdentity', { username: username });
}

function handleRegisterResult(data) {
    const status = document.getElementById('identityRegStatus');
    const btn = document.getElementById('identityRegisterBtn');

    if (data.ok) {
        status.textContent = '';
        window._identityOwnedGames = data.ownedGames || [];
        window._identityEaRelinked = false;
        _updateIdentityButtons();
        hideIdentityModal();
        const usernameField = document.getElementById('username');
        if (usernameField) usernameField.value = data.username || '';
        const nicknameField = document.getElementById('nicknameInput');
        if (nicknameField) nicknameField.value = data.nickname || '';
        updateProfileIdentitySection();
        if (typeof updateProfileWidget === 'function') updateProfileWidget();
    } else {
        btn.disabled = false;
        status.textContent = data.error || t('auth.registration_failed');
        status.className = 'auth-status error';
    }
}

// refresh licenses

function submitRefreshEntitlements() {
    var btn = document.getElementById('refreshLicensesBtn');
    var hint = document.getElementById('refreshLicensesHint');
    if (btn) btn.disabled = true;
    if (hint) hint.textContent = t('auth.checking_ea');
    send('refreshEntitlements');
}

function handleRefreshEntitlementsResult(data) {
    var btn = document.getElementById('refreshLicensesBtn');
    var hint = document.getElementById('refreshLicensesHint');
    if (data.ok) {
        window._identityOwnedGames = data.ownedGames || [];
        _updateIdentityButtons();
        updateGameLibrarySection();
        if (hint) hint.textContent = t('auth.licenses_updated');
        setTimeout(function() {
            if (hint) hint.textContent = t('auth.refresh_licenses_hint');
        }, 3000);
    } else {
        if (hint) hint.textContent = data.error || t('auth.refresh_failed');
        setTimeout(function() {
            if (hint) hint.textContent = t('auth.refresh_licenses_hint');
        }, 4000);
    }
    if (btn) btn.disabled = false;
}

// relink ea account

function showRelinkModal() {
    document.getElementById('relinkStatus').textContent = '';
    document.getElementById('relinkStatus').className = 'auth-status';
    document.getElementById('relinkConfirmBtn').disabled = false;
    document.getElementById('relinkModalBackdrop').style.display = 'flex';
}

function closeRelinkModal() {
    document.getElementById('relinkModalBackdrop').style.display = 'none';
}

function startRelinkEALogin() {
    var btn = document.getElementById('relinkConfirmBtn');
    var status = document.getElementById('relinkStatus');
    btn.disabled = true;
    status.textContent = t('auth.opening_ea');
    status.className = 'auth-status waiting';
    send('relinkEA');
}

function handleRelinkEAResult(data) {
    var btn = document.getElementById('relinkConfirmBtn');
    var status = document.getElementById('relinkStatus');
    if (data.ok) {
        window._identityEaRelinked = true;
        window._identityOwnedGames = data.ownedGames || [];
        _updateIdentityButtons();
        updateGameLibrarySection();
        closeRelinkModal();
        showStatus(t('auth.relink_success'), 'info');
    } else {
        btn.disabled = false;
        status.textContent = data.error || t('auth.relink_failed');
        status.className = 'auth-status error';
    }
}

function submitNickname() {
    var input = document.getElementById('nicknameInput');
    if (!input) return;
    var nickname = input.value.trim();
    var status = document.getElementById('nicknameStatus');
    if (nickname.length > 0 && (nickname.length < 3 || nickname.length > 32)) {
        if (status) { status.textContent = t('auth.nickname_length_error'); status.className = 'auth-status error'; }
        return;
    }
    if (nickname.length > 0 && !/^[a-zA-Z0-9][a-zA-Z0-9 _-]*[a-zA-Z0-9]$/.test(nickname) && !/^[a-zA-Z0-9]$/.test(nickname)) {
        if (status) { status.textContent = t('auth.nickname_chars_error'); status.className = 'auth-status error'; }
        return;
    }
    if (nickname.length > 0 && Math.random() < 1 / 10000) {
        nickname = 'Cypress Carl';
    }
    if (status) { status.textContent = t('auth.saving'); status.className = 'auth-status waiting'; }
    send('setNickname', { nickname: nickname });
}

function handleNicknameResult(data) {
    var status = document.getElementById('nicknameStatus');
    if (data.ok) {
        if (status) { status.textContent = data.nickname ? t('auth.nickname_set') : t('auth.nickname_cleared'); status.className = 'auth-status'; }
        setTimeout(function() { if (status) status.textContent = ''; }, 2000);
    } else {
        if (status) { status.textContent = data.error || 'Failed'; status.className = 'auth-status error'; }
    }
}

function clearIdentity() {
    send('clearIdentity');
}

function setAvatarImage(uid) {
    if (!uid) return;
    var url = 'https://eaavatarservice.akamaized.net/production/avatar/prod/userAvatar/' + uid + '/208x208.JPEG';
    var targets = [
        document.getElementById('profileAvatar'),
        document.getElementById('profileCardAvatar')
    ];
    targets.forEach(function(el) {
        if (!el) return;
        var img = el.querySelector('img');
        if (img) { img.src = url; return; }
        img = document.createElement('img');
        img.src = url;
        img.alt = '';
        img.onerror = function() { this.style.display = 'none'; };
        img.onload = function() { el.textContent = ''; el.appendChild(this); };
    });
}
