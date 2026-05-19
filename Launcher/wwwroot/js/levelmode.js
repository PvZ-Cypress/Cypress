function populateLevelPicker(skipCompat) {
    const sel = document.getElementById('levelPicker');
    const game = getGame();
    const data = GAME_DATA[game];
    sel.innerHTML = '';
    const cats = {};
    data.levels.forEach(l => { if (!cats[l.cat]) cats[l.cat] = []; cats[l.cat].push(l); });
    for (const cat in cats) {
        const og = document.createElement('optgroup');
        og.label = cat;
        cats[cat].forEach(l => {
            const o = document.createElement('option');
            o.value = (game === 'GW1' && l.variant !== undefined) ? l.id + '#' + l.variant : l.id;
            o.textContent = l.name;
            og.appendChild(o);
        });
        sel.appendChild(og);
    }
    renderPickerOptions('levelPicker');
    if (!skipCompat) onLevelPickerChanged();
}

function populateModePicker() {
    const sel = document.getElementById('modePicker');
    const data = GAME_DATA[getGame()];
    sel.innerHTML = '';
    const cats = {};
    data.modes.forEach(m => { if (!cats[m.cat]) cats[m.cat] = []; cats[m.cat].push(m); });
    for (const cat in cats) {
        const og = document.createElement('optgroup');
        og.label = cat;
        cats[cat].forEach(m => {
            const o = document.createElement('option');
            o.value = m.id;
            o.textContent = m.name + (m.note ? ' - ' + m.note : '');
            og.appendChild(o);
        });
        sel.appendChild(og);
    }
    renderPickerOptions('modePicker');
    onModePickerChanged();
}

function populateStartPointPicker() {
    const sel = document.getElementById('startPointPicker');
    const data = GAME_DATA[getGame()];
    sel.innerHTML = '';
    (data.startPoints || []).forEach(s => {
        const o = document.createElement('option');
        o.value = s.id;
        o.textContent = s.name;
        sel.appendChild(o);
    });
    renderPickerOptions('startPointPicker');
}

function parseGW1LevelValue(val) {
    const idx = val.lastIndexOf('#');
    if (idx === -1) return { levelId: val, variant: '0' };
    return { levelId: val.substring(0, idx), variant: val.substring(idx + 1) };
}

function getSelectedLevelData() {
    const game = getGame();
    const data = GAME_DATA[game];
    const rawVal = document.getElementById('levelPicker').value;
    if (game === 'GW1') {
        const parsed = parseGW1LevelValue(rawVal);
        return data.levels.find(l => l.id === parsed.levelId && l.variant === parsed.variant) || null;
    }
    return data.levels.find(l => l.id === rawVal) || null;
}

function getSelectedModeId() {
    return document.getElementById('modePicker').value;
}

function getSupportedModesForLevel(levelData) {
    return levelData && levelData.modes ? new Set(levelData.modes) : null;
}

function getLevelsSupportingMode(modeId) {
    if (!modeId) return null;
    const game = getGame();
    const data = GAME_DATA[game];
    const ids = new Set();
    data.levels.forEach(l => {
        if (l.modes && l.modes.includes(modeId)) {
            ids.add(game === 'GW1' && l.variant !== undefined ? l.id + '#' + l.variant : l.id);
        }
    });
    return ids;
}

function syncPickerCompatibility() {
    const levelData = getSelectedLevelData();
    const supportedModes = getSupportedModesForLevel(levelData);
    const modeId = getSelectedModeId();

    document.querySelectorAll('#modePickerOptions .picker-option').forEach(btn => {
        const val = btn.getAttribute('data-value');
        const supported = !supportedModes || supportedModes.has(val);
        btn.classList.toggle('picker-option-unsupported', !supported);
    });

    const trigger = document.getElementById('modePickerTrigger');
    if (trigger) {
        const isUnsupported = supportedModes && modeId && !supportedModes.has(modeId);
        trigger.classList.toggle('picker-trigger-warn', isUnsupported);
    }
}

function onLevelPickerChanged() {
    const levelData = getSelectedLevelData();
    const modeId = getSelectedModeId();
    const supportedModes = getSupportedModesForLevel(levelData);

    if (supportedModes && modeId && !supportedModes.has(modeId)) {
        const firstSupported = Array.from(document.getElementById('modePicker').options).find(o => supportedModes.has(o.value));
        if (firstSupported) {
            selectPickerValue('modePicker', firstSupported.value);
            return; // selectpickervalue triggers onmodepickerchanged which calls syncpickercompatibility
        }
    }

    syncPickerCompatibility();
    onInclusionChanged();
}
function onModePickerChanged() {
    syncPickerCompatibility();
    onInclusionChanged();
}

function checkCompatibility() {
    const game = getGame();
    const warnings = [];
    const levelData = getSelectedLevelData();
    const modeId = getSelectedModeId();

    if (!levelData || !modeId) {
        setInclusionWarning('');
        return;
    }

    // check level-mode compatibility
    const supported = getSupportedModesForLevel(levelData);
    if (supported && !supported.has(modeId)) {
        const modeData = GAME_DATA[game].modes.find(m => m.id === modeId);
        warnings.push(t('mapmode.mode_incompatible', { mode: modeData ? modeData.name : modeId, level: levelData.name }));
    }

    // GW1: night TOD check
    if (game === 'GW1') {
        const tod = document.getElementById('todPicker').value;
        if (tod === 'Night' && !levelData.night) {
            warnings.push(t('mapmode.night_not_supported_gw1', { level: levelData.name }));
        }
    }

    // GW2: night TOD note
    if (game === 'GW2') {
        const tod = document.getElementById('todPicker').value;
        if (tod === 'Night' && !levelData.tod) {
            warnings.push(t('mapmode.night_partial_gw2', { level: levelData.name }));
        }
    }

    // BFN: start point compatibility
    if (game === 'BFN') {
        const spId = document.getElementById('startPointPicker').value;
        const spData = GAME_DATA.BFN.startPoints.find(s => s.id === spId);
        if (spData) {
            const levelId = document.getElementById('levelPicker').value;
            const levelInfo = levelData;
            let spLevelOk = true;
            if (spData.levels) {
                spLevelOk = spData.levels.includes(levelId);
            } else if (spData.cats) {
                spLevelOk = spData.cats.includes(levelInfo.cat);
                if (!spLevelOk && spData.extraLevels) spLevelOk = spData.extraLevels.includes(levelId);
            } else if (spData.excludeLevels) {
                spLevelOk = !spData.excludeLevels.includes(levelId);
            }
            if (!spLevelOk) {
                warnings.push(t('mapmode.start_not_supported', { sp: spData.name, level: levelData.name }));
            }
            if (spData.modes && !spData.modes.includes(modeId)) {
                const modeData = GAME_DATA.BFN.modes.find(m => m.id === modeId);
                warnings.push(t('mapmode.start_requires_mode', { sp: spData.name, modes: spData.modes.join(', '), mode: modeData ? modeData.name : modeId }));
            }
        }
    }

    setInclusionWarning(warnings.join(' '));
}

function setInclusionWarning(msg) {
    const el = document.getElementById('inclusionWarning');
    const textEl = document.getElementById('inclusionWarningText');
    if (!el || !textEl) return;
    if (msg) {
        textEl.textContent = msg;
        el.style.display = '';
    } else {
        el.style.display = 'none';
    }
}

function levelSupportsTod(levelId) {
    const game = getGame();
    // GW1 and GW2 always use TOD (day is universal). BFN does not use TOD.
    if (game === 'GW1' || game === 'GW2') return true;
    return false;
}

function levelSupportsNight(levelId) {
    const data = GAME_DATA[getGame()];
    if (!data) return false;
    const level = data.levels.find(l => l.id === levelId);
    if (!level) return false;
    // GW1 uses 'night', GW2 uses 'TOD' for night support
    return !!(level.night || level.tod);
}

function getGW1Variants(levelData) {
    if (!levelData || !levelData.variants) return null;
    return levelData.variants; // array of { v, name }
}

function updateVariantPicker() {
    const game = getGame();
    const group = document.getElementById('variantGroup');
    if (!group) return;
    if (game !== 'GW1') {
        group.style.display = 'none';
        return;
    }
    const levelData = getSelectedLevelData();
    const variants = getGW1Variants(levelData);
    if (!variants || variants.length <= 1) {
        group.style.display = 'none';
        // reset to 0
        const sel = document.getElementById('variantPicker');
        if (sel) sel.value = '0';
        return;
    }
    group.style.display = '';
    // update button labels
    const btns = group.querySelectorAll('.segmented-btn');
    btns.forEach(btn => {
        const val = btn.getAttribute('data-value');
        const vData = variants.find(vv => vv.v === val);
        btn.textContent = vData ? vData.name : ('Variant ' + val);
        btn.style.display = vData ? '' : 'none';
    });
    const hint = document.getElementById('variantHint');
    if (hint) hint.textContent = t('mapmode.variant_hint');
}

function onInclusionChanged() {
    if (manualMode) return;
    const game = getGame();
    const levelVal = document.getElementById('levelPicker').value;
    const modeVal = document.getElementById('modePicker').value;

    // TOD visibility
    const hasTod = levelSupportsTod(levelVal);
    const todGroup = document.getElementById('todGroup');
    if (todGroup) todGroup.style.display = hasTod ? '' : 'none';

    // night button state
    if (hasTod) {
        const nightSupported = levelSupportsNight(levelVal);
        const nightBtn = document.querySelector('[data-select-target="todPicker"] .segmented-btn[data-value="Night"]');
        if (nightBtn) {
            nightBtn.classList.toggle('segmented-btn-dimmed', !nightSupported);
            nightBtn.title = nightSupported ? t('mapmode.night_title') : t('mapmode.night_unsupported_title');
        }
    }

    // variant picker (GW1 only), no longer needed, hidden
    const variantGroup = document.getElementById('variantGroup');
    if (variantGroup) variantGroup.style.display = 'none';

    // for GW1, extract the real level ID from the composite value
    const realLevelId = (game === 'GW1') ? parseGW1LevelValue(levelVal).levelId : levelVal;
    document.getElementById('level').value = realLevelId;

    // build inclusion
    let modeStr = modeVal;
    if (game === 'GW1') {
        // GW1 modes require a variant suffix (0 or 1)
        const levelData = getSelectedLevelData();
        const variant = (levelData && levelData.variant) || '0';
        modeStr = modeVal + variant;
    }

    const isHub = game === 'GW2' && (levelVal === 'Level_FE_Hub' || levelVal === 'Level_Hub_TacoBandits');
    let inc = 'GameMode=' + modeStr;
    if (hasTod && !isHub) {
        inc += ';TOD=' + document.getElementById('todPicker').value;
    }
    if (game === 'GW2') inc += ';HostedMode=' + document.getElementById('hostedModePicker').value;
    document.getElementById('inclusion').value = inc;
    if (game === 'BFN') document.getElementById('startPoint').value = document.getElementById('startPointPicker').value;

    checkCompatibility();
    updateChangedSettingsIndicator();
    updateModifierArgsPreview();
}

function setManualMode(nextManualMode) {
    manualMode = nextManualMode;
    const dropdownBtn = document.getElementById('dropdownModeBtn');
    const manualBtn = document.getElementById('manualModeBtn');
    const fields = ['level', 'inclusion', 'startPoint', 'serverArgsPreview'];
    const modifierSection = document.getElementById('modifierToggles');
    if (manualMode) {
        fields.forEach(f => {
            const el = document.getElementById(f);
            el.removeAttribute('readonly');
            el.classList.remove('text-input-readonly');
        });
        dropdownBtn.classList.remove('active');
        manualBtn.classList.add('active');
        if (modifierSection) {
            modifierSection.classList.add('modifier-grid-locked');
        }
    } else {
        fields.forEach(f => {
            const el = document.getElementById(f);
            el.setAttribute('readonly', '');
            el.classList.add('text-input-readonly');
        });
        dropdownBtn.classList.add('active');
        manualBtn.classList.remove('active');
        if (modifierSection) {
            modifierSection.classList.remove('modifier-grid-locked');
        }
        onInclusionChanged();
        updateModifierArgsPreview();
    }
}
