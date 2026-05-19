function getChangedSettingsList() {
    const changes = [];
    document.querySelectorAll('.mod-toggle').forEach(el => {
        const key = el.getAttribute('data-key');
        const card = el.closest('.mod-card');
        let name = '';
        if (card) {
            const btn = card.querySelector('.mod-bool-btn');
            const nameEl = card.querySelector('.mod-card-name');
            if (btn) {
                const titleSpan = btn.querySelector('.mod-title-text');
                name = titleSpan ? titleSpan.textContent.trim() : btn.firstChild.textContent.trim();
            }
            else if (nameEl) {
                const titleSpan = nameEl.querySelector('.mod-title-text');
                name = titleSpan ? titleSpan.textContent.trim() : nameEl.firstChild?.textContent?.trim() || '';
            }
        }
        if (!name) name = key;
        if (el.type === 'checkbox') {
            if (el.checked) changes.push({ name: name, value: t('settings.enabled') });
        } else if (el.type === 'range') {
            const def = el.getAttribute('data-default');
            if (def !== null && el.value !== def) {
                const labels = el.getAttribute('data-labels');
                let val = el.value;
                if (labels) { try { const l = JSON.parse(decodeURIComponent(labels)); val = l[parseInt(el.value)] || val; } catch(e){} }
                changes.push({ name: name, value: val });
            }
        } else if (el.tagName === 'SELECT') {
            if (el.value) {
                const opt = el.options[el.selectedIndex];
                changes.push({ name: name, value: opt ? opt.text : el.value });
            }
        } else if (el.type === 'text' || el.type === 'number') {
            if (el.value && el.id !== 'gw2CostumeIds') changes.push({ name: name, value: el.value });
        }
    });
    const costumeInput = document.getElementById('gw2CostumeIds');
    if (costumeInput && costumeInput.value) {
        const count = costumeInput.value.split(';').filter(Boolean).length;
        changes.push({ name: t('settings.char_restrictions'), value: count + ' variants' });
    }
    const killOut = document.getElementById('bfnKillOutput');
    if (killOut && killOut.textContent !== 'None' && killOut.textContent !== '') {
        const killed = document.querySelectorAll('.bfn-class-btn[data-section="kill"]:not(.active)');
        changes.push({ name: t('settings.class_kill_switches'), value: killed.length + ' disabled' });
    }
    const pm = document.getElementById('bfnPlantMask');
    const zm = document.getElementById('bfnZombieMask');
    if ((pm && parseInt(pm.textContent) > 0) || (zm && parseInt(zm.textContent) > 0)) {
        const disabled = document.querySelectorAll('.bfn-class-btn[data-section="ai"]:not(.active)');
        changes.push({ name: t('settings.ai_spawn_restrictions'), value: disabled.length + ' disabled' });
    }
    return changes;
}

function updateChangedSettingsIndicator() {
    const panel = document.getElementById('changedSettingsPanel');
    if (!panel) return;
    const changes = getChangedSettingsList();
    const levelData = getSelectedLevelData();
    const modeId = getSelectedModeId();
    const game = getGame();
    const gameData = GAME_DATA[game];

    const hasContent = levelData || changes.length > 0;
    if (!hasContent) {
        panel.classList.remove('visible');
        return;
    }
    panel.classList.add('visible');

    // check for GW2 loadscreen override (boss hunt etc.)
    const overrideKey = levelData && modeId ? (levelData.id + '+' + modeId) : '';
    const override = (game === 'GW2' && typeof GW2_LOADSCREEN_OVERRIDES !== 'undefined') ? GW2_LOADSCREEN_OVERRIDES[overrideKey] : null;

    const bgEl = document.getElementById('changedSettingsBg');
    if (bgEl && levelData) {
        const bgKey = (override && override.bg) ? override.bg : LEVEL_MAP_BG[levelData.id];
        if (bgKey && MAP_BG_CACHE[bgKey]) {
            bgEl.src = MAP_BG_CACHE[bgKey];
            bgEl.style.display = '';
        } else if (bgKey && MAP_BG_CACHE[bgKey] === undefined) {
            bgEl.style.display = 'none';
            MAP_BG_CACHE[bgKey] = null; // mark as loading
            send('getMapBg', { key: bgKey });
        } else {
            bgEl.style.display = 'none';
        }
    } else if (bgEl) {
        bgEl.style.display = 'none';
    }

    const mapEl = document.getElementById('changedSettingsMap');
    const modeEl = document.getElementById('changedSettingsMode');
    if (mapEl) mapEl.textContent = levelData ? levelData.name : '';
    if (modeEl) {
        if (override && override.displayName) {
            modeEl.textContent = override.displayName;
        } else {
            const modeData = gameData && modeId ? gameData.modes.find(m => m.id === modeId) : null;
            modeEl.textContent = modeData ? modeData.name : (modeId || '');
        }
    }

    const listEl = document.getElementById('changedSettingsList');
    if (listEl) {
        if (changes.length === 0) {
            listEl.innerHTML = '<span class="changed-settings-label">' + escapeHtml(t('settings.default_settings')) + '</span>';
        } else {
            let html = '<span class="changed-settings-label">' + escapeHtml(changes.length === 1 ? t('settings.n_setting_changed', { n: 1 }) : t('settings.n_settings_changed', { n: changes.length })) + '</span>';
            changes.forEach(c => {
                html += '<span class="changed-setting-pill">' + c.name + ' <span class="pill-value">' + c.value + '</span></span>';
            });
            listEl.innerHTML = html;
        }
    }
}

document.addEventListener('change', function(e) {
    updateModifierArgsPreview();
});

function toggleSection(id) {
    const el = document.getElementById(id);
    const chev = document.getElementById(id + '_chevron');
    if (el.style.display === 'none') { el.style.display = ''; chev.innerHTML = '&#9660;'; }
    else { el.style.display = 'none'; chev.innerHTML = '&#9654;'; }
}
