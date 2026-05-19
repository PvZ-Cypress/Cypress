// returns an <img> for a character icon, requesting from backend on first use
function charIconImg(key, extraClass) {
    extraClass = extraClass ? ' ' + extraClass : '';
    if (typeof CHAR_ICON_CACHE === 'undefined') return '';
    if (CHAR_ICON_CACHE[key]) {
        return '<img class="char-icon' + extraClass + '" src="' + CHAR_ICON_CACHE[key] + '" data-icon-key="' + escapeHtml(key) + '" alt="" draggable="false">';
    }
    if (CHAR_ICON_CACHE[key] === undefined) {
        CHAR_ICON_CACHE[key] = null;
        send('getCharIcon', { key: key });
    }
    return '<img class="char-icon char-icon-pending' + extraClass + '" data-icon-key="' + escapeHtml(key) + '" alt="" draggable="false">';
}

function populateModifierToggles() {
    const container = document.getElementById('modifierToggles');
    const game = getGame();
    const data = GAME_DATA[game];
    container.innerHTML = '';

    data.modifierCategories.forEach(cat => {
        const catDiv = document.createElement('div');
        catDiv.className = 'mod-category';

        if (cat.special === 'gw2_costumes') {
            catDiv.innerHTML = buildGW2CostumeSection();
            container.appendChild(catDiv);
            return;
        }
        if (cat.special === 'bfn_classes') {
            catDiv.innerHTML = buildBFNClassSection();
            container.appendChild(catDiv);
            return;
        }

        let header = '<div class="mod-category-header"><div><strong>' + cat.name + '</strong><span class="mod-category-desc">' + cat.desc + '</span></div></div>';
        let modsHtml = '<div class="mod-category-body">';
        cat.mods.forEach(mod => {
            modsHtml += '<div class="mod-card mod-card-' + mod.type + (mod.type === 'bool' ? ' mod-card-bool' : '') + '">';
            modsHtml += '<div class="mod-card-info">';
            if (mod.type !== 'bool') {
                modsHtml += '<span class="mod-card-name">' + buildModifierTitle(mod.name, mod.desc) + '</span>';
            }
            modsHtml += '</div>';
            modsHtml += '<div class="mod-card-control' + (mod.type === 'bool' ? '' : ' mod-card-control-stretch') + '">';
            if (mod.type === 'bool') {
                modsHtml += '<input type="checkbox" class="mod-toggle mod-toggle-hidden" data-key="' + mod.key + '" tabindex="-1" aria-hidden="true">';
                modsHtml += '<button type="button" class="mod-bool-btn" aria-pressed="false" onclick="toggleModifierBool(this)">' + buildModifierTitle(mod.name, mod.desc) + '</button>';
            } else if (mod.type === 'slider') {
                modsHtml += '<div class="slider-control">';
                modsHtml += '<div class="slider-value"><span>' + mod.name + '</span><strong class="slider-badge" data-slider-label>' + mod.labels[mod.defaultValue] + '</strong></div>';
                modsHtml += '<input type="range" class="modifier-slider mod-toggle" data-key="' + mod.key + '" data-default="' + mod.defaultValue + '" data-labels="' + encodeURIComponent(JSON.stringify(mod.labels || [])) + '" min="' + mod.min + '" max="' + mod.max + '" step="' + mod.step + '" value="' + mod.defaultValue + '" oninput="updateModifierSlider(this)">';
                modsHtml += '<div class="slider-scale">' + mod.labels.map((label, index) => '<span data-slider-step="' + index + '">' + label + '</span>').join('') + '</div>';
                modsHtml += '</div>';
            } else if (mod.type === 'select') {
                const pickerId = 'modpk_' + mod.key.replace(/[^a-z0-9]/gi, '_');
                const pickerType = mod.key.includes('AICharacterSet') ? 'aiset' : null;
                modsHtml += '<div class="smart-picker" data-picker-id="' + pickerId + '">';
                modsHtml += '<button type="button" class="smart-picker-trigger" id="' + pickerId + 'Trigger" onclick="togglePicker(\'' + pickerId + '\')">';
                modsHtml += '<span id="' + pickerId + 'TriggerText">' + escapeHtml(mod.options[0] ? mod.options[0].n : 'Select') + '</span>';
                modsHtml += '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="6 9 12 15 18 9"/></svg></button>';
                modsHtml += '<div class="smart-picker-panel" id="' + pickerId + 'Panel">';
                modsHtml += '<div class="smart-picker-options" id="' + pickerId + 'Options"></div></div></div>';
                modsHtml += '<select class="select-input ui-hidden mod-toggle" data-key="' + mod.key + '" id="' + pickerId + '">';
                mod.options.forEach(o => { modsHtml += '<option value="' + o.v + '">' + o.n + '</option>'; });
                modsHtml += '</select>';
            } else if (mod.type === 'number') {
                modsHtml += '<input type="number" class="text-input text-input-sm mod-toggle" data-key="' + mod.key + '" placeholder="' + (mod.placeholder||'default') + '">';
            }
            modsHtml += '</div></div>';
        });
        modsHtml += '</div>';
        catDiv.innerHTML = header + modsHtml;
        container.appendChild(catDiv);
        // render smart pickers for select-type mods
        catDiv.querySelectorAll('.smart-picker').forEach(sp => {
            const id = sp.getAttribute('data-picker-id');
            const type = id && id.includes('AICharacterSet') ? 'aiset' : null;
            if (id && !PICKER_REGISTRY[id]) registerPicker(id, { type: type, onChange: () => { if (typeof updateModifierArgsPreview === 'function') updateModifierArgsPreview(); } });
            if (id) { renderPickerOptions(id); updatePickerTrigger(id); }
        });
    });

    container.querySelectorAll('.modifier-slider').forEach(updateModifierSlider);
    if (game === 'GW2') {
        syncGW2CostumeUi();
    }
    if (game === 'BFN') {
        updateBFNKillOutput();
        updateBFNMasks();
    }
    updateModifierArgsPreview();
}

function normalizeGW2CostumeIds(ids) {
    const rawIds = Array.isArray(ids) ? ids.map(id => String(id).trim()).filter(Boolean) : [];
    const seen = new Set();
    const normalized = [];

    GW2_ALL_COSTUME_IDS.forEach(id => {
        if (rawIds.includes(id) && !seen.has(id)) {
            seen.add(id);
            normalized.push(id);
        }
    });

    rawIds.forEach(id => {
        if (!seen.has(id)) {
            seen.add(id);
            normalized.push(id);
        }
    });

    return normalized;
}

function getGW2CostumeInput() {
    return document.getElementById('gw2CostumeIds');
}

function getGW2RawCostumeIds() {
    const input = getGW2CostumeInput();
    if (!input || !input.value.trim()) {
        return [];
    }

    return normalizeGW2CostumeIds(input.value.split(';'));
}

function getGW2EffectiveCostumeIds() {
    const rawIds = getGW2RawCostumeIds();
    return rawIds.length ? rawIds : [...GW2_ALL_COSTUME_IDS];
}

function arraysMatch(left, right) {
    if (left.length !== right.length) {
        return false;
    }

    return left.every((value, index) => value === right[index]);
}

function setGW2CostumeSelection(ids) {
    const input = getGW2CostumeInput();
    if (!input) {
        return;
    }

    const normalized = normalizeGW2CostumeIds(ids);
    input.value = normalized.length === 0 || normalized.length === GW2_ALL_COSTUME_IDS.length
        ? ''
        : normalized.join(';');
    syncGW2CostumeUi();
}

function syncGW2PresetButtons(rawIds) {
    const normalizedRawIds = normalizeGW2CostumeIds(rawIds);

    document.querySelectorAll('.gw2-preset-btn').forEach(button => {
        const presetName = button.getAttribute('data-preset') || '';
        if (!presetName) {
            button.classList.toggle('active', normalizedRawIds.length === 0 || normalizedRawIds.length === GW2_ALL_COSTUME_IDS.length);
            return;
        }

        const presetIds = normalizeGW2CostumeIds((GW2_COSTUME_PRESETS[presetName] || '').split(';'));
        button.classList.toggle('active', arraysMatch(normalizedRawIds, presetIds));
    });

    document.querySelectorAll('.gw2-quick-btn').forEach(button => {
        const mode = button.getAttribute('data-mode');
        if (mode === 'plants') {
            button.classList.toggle('active', arraysMatch(normalizedRawIds, GW2_COSTUME_TEAM_IDS.Plants));
        } else if (mode === 'zombies') {
            button.classList.toggle('active', arraysMatch(normalizedRawIds, GW2_COSTUME_TEAM_IDS.Zombies));
        }
    });
}

function syncGW2CostumeUi() {
    const rawIds = getGW2RawCostumeIds();
    const effectiveIds = rawIds.length ? rawIds : GW2_ALL_COSTUME_IDS;
    const selected = new Set(effectiveIds);
    updateModifierArgsPreview();

    document.querySelectorAll('.gw2-costume-btn').forEach(button => {
        button.classList.toggle('active', selected.has(button.getAttribute('data-id')));
    });

    document.querySelectorAll('.gw2-costume-class-card').forEach(card => {
        const classIds = normalizeGW2CostumeIds((card.getAttribute('data-ids') || '').split(';'));
        const selectedCount = classIds.filter(id => selected.has(id)).length;
        card.classList.toggle('active', selectedCount === classIds.length);

        const countEl = card.querySelector('[data-class-count]');
        if (countEl) {
            countEl.textContent = selectedCount + '/' + classIds.length;
        }
    });

    Object.entries(GW2_COSTUME_TEAM_IDS).forEach(([team, ids]) => {
        const selectedCount = ids.filter(id => selected.has(id)).length;
        const summary = document.querySelector('[data-gw2-team-summary="' + team + '"]');
        if (summary) {
            summary.textContent = t('modifiers.n_of_n_enabled', {n: selectedCount, total: ids.length});
        }
    });

    const summaryEl = document.getElementById('gw2CostumeSummary');
    if (summaryEl) {
        if (rawIds.length === 0 || effectiveIds.length === GW2_ALL_COSTUME_IDS.length) {
            summaryEl.textContent = t('modifiers.all_chars_playable');
        } else {
            summaryEl.textContent = t('modifiers.n_char_variants', {n: effectiveIds.length});
        }
    }

    syncGW2PresetButtons(rawIds);
}

function buildGW2CostumeSection() {
    let html = '<div class="mod-category-header"><div><strong>' + t('modifiers.char_restrictions_title') + '</strong><span class="mod-category-desc">' + t('modifiers.char_restrictions_desc') + '</span></div></div>';
    html += '<div class="mod-category-body">';
    html += '<div class="mod-card mod-card-section gw2-costume-shell">';
    html += '<div class="gw2-costume-toolbar">';
    html += '<button type="button" class="preset-btn gw2-preset-btn is-clear" data-preset="" onclick="applyGW2CostumePreset(\'\')">'+t('modifiers.all_characters')+'</button>';
    html += '<button type="button" class="preset-btn gw2-quick-btn" data-mode="plants" onclick="applyGW2CostumeTeamSelection(\'Plants\')">' + t('modifiers.plants_only') + '</button>';
    html += '<button type="button" class="preset-btn gw2-quick-btn" data-mode="zombies" onclick="applyGW2CostumeTeamSelection(\'Zombies\')">' + t('modifiers.zombies_only') + '</button>';
    html += '</div>';
    html += '<div class="mod-card-info"><span class="mod-card-name">' + t('modifiers.weekly_presets') + '</span></div>';
    html += '<div class="preset-grid gw2-preset-grid">';
    Object.keys(GW2_COSTUME_PRESETS).forEach(name => {
        html += '<button type="button" class="preset-btn gw2-preset-btn" data-preset="' + name + '" onclick="applyGW2CostumePreset(' + JSON.stringify(name).replace(/"/g, '&quot;') + ')">' + name + '</button>';
    });
    html += '</div>';
    html += '<div class="gw2-costume-summary" id="gw2CostumeSummary">' + t('modifiers.all_chars_playable') + '</div>';
    html += '<div class="gw2-costume-team-grid">';
    Object.entries(GW2_COSTUME_CLASSES).forEach(([team, classes]) => {
        html += '<section class="gw2-costume-team">';
        html += '<div class="gw2-costume-team-header"><div><strong>' + team + '</strong><span class="mod-card-desc" data-gw2-team-summary="' + team + '"></span></div><button type="button" class="preset-btn" onclick="applyGW2CostumeTeamSelection(\'' + team + '\')">' + t('modifiers.only_team', {team: team}) + '</button></div>';
        html += '<div class="gw2-costume-class-grid">';
        classes.forEach(characterClass => {
            const classIds = characterClass.variants.map(variant => variant.id).join(';');
            const classIconKey = (typeof GW2_CLASS_ICON !== 'undefined') ? (GW2_CLASS_ICON[characterClass.name] || null) : null;
            html += '<article class="gw2-costume-class-card" data-ids="' + classIds + '">';
            html += '<div class="gw2-costume-class-head">';
            html += '<div class="gw2-costume-class-head-left">';
            if (classIconKey) html += charIconImg(classIconKey, 'class-icon');
            html += '<span class="gw2-costume-class-name">' + characterClass.name + '</span>';
            html += '</div>';
            html += '<span class="gw2-class-count" data-class-count>0/' + characterClass.variants.length + '</span></div>';
            html += '<div class="gw2-costume-variant-grid">';
            characterClass.variants.forEach(variant => {
                const varIconKey = (typeof GW2_VARIANT_ICON !== 'undefined') ? (GW2_VARIANT_ICON[variant.id] || null) : null;
                html += '<button type="button" class="gw2-costume-btn" data-id="' + variant.id + '" onclick="toggleGW2CostumeButton(this)">';
                if (varIconKey) html += charIconImg(varIconKey);
                html += '<span>' + variant.name + '</span></button>';
            });
            html += '</div></article>';
        });
        html += '</div></section>';
    });
    html += '</div>';
    html += '<div class="gw2-costume-raw"><div class="mod-card-info"><span class="mod-card-name">' + t('modifiers.raw_costume_ids') + '</span><span class="mod-card-desc">' + t('modifiers.raw_costume_desc') + '</span></div>';
    html += '<input type="text" class="text-input text-input-sm mod-toggle gw2-costume-input" data-key="GameMode.AvailableCostumes" id="gw2CostumeIds" placeholder="' + t('modifiers.costume_placeholder') + '" oninput="syncGW2CostumeUi()"></div>';
    html += '</div>';
    html += '</div>';
    return html;
}

function applyGW2CostumePreset(name) {
    if (!name) {
        setGW2CostumeSelection(GW2_ALL_COSTUME_IDS);
        return;
    }

    if (GW2_COSTUME_PRESETS[name]) {
        setGW2CostumeSelection(GW2_COSTUME_PRESETS[name].split(';'));
    }
}

function applyGW2CostumeTeamSelection(team) {
    if (GW2_COSTUME_TEAM_IDS[team]) {
        setGW2CostumeSelection(GW2_COSTUME_TEAM_IDS[team]);
    }
}

function toggleGW2CostumeButton(button) {
    const costumeId = button.getAttribute('data-id');
    if (!costumeId) {
        return;
    }

    const selectedIds = new Set(getGW2EffectiveCostumeIds());
    if (selectedIds.has(costumeId)) {
        if (selectedIds.size === 1) {
            return;
        }
        selectedIds.delete(costumeId);
    } else {
        selectedIds.add(costumeId);
    }

    setGW2CostumeSelection([...selectedIds]);
}

function buildBFNClassSection() {
    let html = '<div class="mod-category-header"><div><strong>' + t('modifiers.char_restrictions_title') + '</strong><span class="mod-category-desc">' + t('modifiers.restrict_classes_desc') + '</span></div></div>';
    html += '<div class="mod-category-body">';
    html += '<div class="mod-card mod-card-section bfn-class-shell">';

    html += '<div class="mod-card-info"><span class="mod-card-name">' + t('modifiers.class_kill_switches_title') + '</span><span class="mod-card-desc">' + t('modifiers.class_kill_switches_desc') + '</span></div>';
    html += '<div class="bfn-class-toolbar">';
    html += '<button type="button" class="preset-btn bfn-preset-btn is-clear" data-preset="" onclick="applyBFNKillPreset(\'\')">'+t('modifiers.all_playable')+'</button>';
    html += '</div>';
    html += '<div class="mod-card-info"><span class="mod-card-name">' + t('modifiers.weekly_presets') + '</span></div>';
    html += '<div class="preset-grid bfn-preset-grid">';
    Object.keys(BFN_KILLSWITCH_PRESETS).forEach(name => {
        html += '<button type="button" class="preset-btn bfn-preset-btn" data-preset="' + name + '" onclick="applyBFNKillPreset(' + JSON.stringify(name).replace(/"/g, '&quot;') + ')">' + name + '</button>';
    });
    html += '</div>';
    html += '<div class="bfn-class-summary" id="bfnKillSummary">' + t('modifiers.all_classes_playable') + '</div>';

    html += '<div class="bfn-class-team-grid">';
    Object.entries(BFN_CLASSES).forEach(([team, classes]) => {
        html += '<section class="bfn-class-team">';
        html += '<div class="bfn-class-team-header"><div><strong>' + team + '</strong><span class="mod-card-desc" data-bfn-kill-team-summary="' + team + '"></span></div></div>';
        html += '<div class="bfn-class-btn-grid">';
        classes.forEach(c => {
            const iconKey = (typeof BFN_CLASS_ICON !== 'undefined') ? (BFN_CLASS_ICON[c.name] || null) : null;
            html += '<button type="button" class="bfn-class-btn active" data-kit="' + c.kit + '" data-section="kill" onclick="toggleBFNClassButton(this)">';
            if (iconKey) html += charIconImg(iconKey);
            html += '<span>' + c.name + '</span></button>';
        });
        html += '</div></section>';
    });
    html += '</div>';

    html += '<div class="bfn-class-divider"></div>';
    html += '<div class="mod-card-info"><span class="mod-card-name">' + t('modifiers.ai_spawn_restrictions_title') + '</span><span class="mod-card-desc">' + t('modifiers.ai_spawn_restrictions_desc') + '</span></div>';
    html += '<div class="bfn-class-summary" id="bfnAiSummary">' + t('modifiers.all_ai_spawn') + '</div>';
    html += '<div class="bfn-class-team-grid">';
    Object.entries(BFN_CLASSES).forEach(([team, classes]) => {
        html += '<section class="bfn-class-team">';
        html += '<div class="bfn-class-team-header"><div><strong>' + team + ' AI</strong><span class="mod-card-desc" data-bfn-ai-team-summary="' + team + '"></span></div></div>';
        html += '<div class="bfn-class-btn-grid">';
        classes.forEach(c => {
            const iconKey = (typeof BFN_CLASS_ICON !== 'undefined') ? (BFN_CLASS_ICON[c.name] || null) : null;
            html += '<button type="button" class="bfn-class-btn active" data-mask="' + c.mask + '" data-team="' + team + '" data-section="ai" onclick="toggleBFNClassButton(this)">';
            if (iconKey) html += charIconImg(iconKey);
            html += '<span>' + c.name + '</span></button>';
        });
        html += '</div></section>';
    });
    html += '</div>';

    html += '<code class="kill-output" id="bfnKillOutput" style="display:none;">None</code>';
    html += '<code id="bfnPlantMask" style="display:none;">0</code>';
    html += '<code id="bfnZombieMask" style="display:none;">0</code>';

    html += '</div></div>';
    return html;
}

function applyBFNKillPreset(name) {
    const preset = BFN_KILLSWITCH_PRESETS[name] || [];
    document.querySelectorAll('.bfn-preset-btn').forEach(btn => {
        btn.classList.toggle('active', (btn.getAttribute('data-preset') || '') === (name || ''));
    });
    document.querySelectorAll('.bfn-class-btn[data-section="kill"]').forEach(btn => {
        btn.classList.toggle('active', !preset.includes(btn.getAttribute('data-kit')));
    });
    updateBFNKillOutput();
    syncBFNKillSummary();
}

function toggleBFNClassButton(button) {
    button.classList.toggle('active');
    if (button.getAttribute('data-section') === 'kill') {
        updateBFNKillOutput();
        syncBFNKillSummary();
        syncBFNPresetButtons();
    } else {
        updateBFNMasks();
        syncBFNAiSummary();
    }
    updateModifierArgsPreview();
}

function syncBFNKillSummary() {
    const allBtns = document.querySelectorAll('.bfn-class-btn[data-section="kill"]');
    const activeBtns = document.querySelectorAll('.bfn-class-btn[data-section="kill"].active');
    const summaryEl = document.getElementById('bfnKillSummary');
    if (summaryEl) {
        if (activeBtns.length === allBtns.length) {
            summaryEl.textContent = t('modifiers.all_classes_playable');
        } else {
            summaryEl.textContent = t('modifiers.n_of_n_classes_playable', {n: activeBtns.length, total: allBtns.length});
        }
    }
    Object.entries(BFN_CLASSES).forEach(([team, classes]) => {
        const teamBtns = document.querySelectorAll('.bfn-class-btn[data-section="kill"][data-kit]');
        const teamKits = classes.map(c => c.kit);
        let activeCount = 0;
        teamBtns.forEach(btn => { if (teamKits.includes(btn.getAttribute('data-kit')) && btn.classList.contains('active')) activeCount++; });
        const el = document.querySelector('[data-bfn-kill-team-summary="' + team + '"]');
        if (el) el.textContent = t('modifiers.n_of_n_playable', {n: activeCount, total: classes.length});
    });
}

function syncBFNAiSummary() {
    const allBtns = document.querySelectorAll('.bfn-class-btn[data-section="ai"]');
    const activeBtns = document.querySelectorAll('.bfn-class-btn[data-section="ai"].active');
    const summaryEl = document.getElementById('bfnAiSummary');
    if (summaryEl) {
        if (activeBtns.length === allBtns.length) {
            summaryEl.textContent = t('modifiers.all_ai_spawn');
        } else {
            summaryEl.textContent = t('modifiers.n_of_n_ai_enabled', {n: activeBtns.length, total: allBtns.length});
        }
    }
    Object.entries(BFN_CLASSES).forEach(([team, classes]) => {
        const teamBtns = document.querySelectorAll('.bfn-class-btn[data-section="ai"][data-team="' + team + '"]');
        let activeCount = 0;
        teamBtns.forEach(btn => { if (btn.classList.contains('active')) activeCount++; });
        const el = document.querySelector('[data-bfn-ai-team-summary="' + team + '"]');
        if (el) el.textContent = t('modifiers.n_of_n_can_spawn', {n: activeCount, total: classes.length});
    });
}

function syncBFNPresetButtons() {
    const killedKits = [];
    document.querySelectorAll('.bfn-class-btn[data-section="kill"]').forEach(btn => {
        if (!btn.classList.contains('active')) killedKits.push(btn.getAttribute('data-kit'));
    });
    const killedSet = new Set(killedKits);
    let matchedPreset = killedKits.length === 0 ? '' : null;
    if (matchedPreset === null) {
        for (const [name, kits] of Object.entries(BFN_KILLSWITCH_PRESETS)) {
            if (kits.length === killedSet.size && kits.every(k => killedSet.has(k))) { matchedPreset = name; break; }
        }
    }
    document.querySelectorAll('.bfn-preset-btn').forEach(btn => {
        btn.classList.toggle('active', (btn.getAttribute('data-preset') || '') === (matchedPreset || ''));
    });
}

function toggleModifierBool(button) {
    const input = button.parentElement.querySelector('.mod-toggle-hidden, .inst-mod-toggle-hidden');
    if (!input) {
        return;
    }

    input.checked = !input.checked;
    button.classList.toggle('active', input.checked);
    button.setAttribute('aria-pressed', input.checked ? 'true' : 'false');
    updateModifierArgsPreview();
}

function updateModifierSlider(input) {
    const value = Number(input.value);
    let labels = [];
    try {
        labels = JSON.parse(decodeURIComponent(input.dataset.labels || '[]'));
    } catch (error) {
        labels = [];
    }

    const label = Array.isArray(labels) ? (labels[value] ?? String(value)) : String(value);
    const labelEl = input.parentElement.querySelector('[data-slider-label]');
    if (labelEl) {
        labelEl.textContent = label;
    }

    input.parentElement.querySelectorAll('[data-slider-step]').forEach(step => {
        step.classList.toggle('active', Number(step.getAttribute('data-slider-step')) === value);
    });
    updateModifierArgsPreview();
}

function updateBFNKillOutput() {
    const killed = [];
    document.querySelectorAll('.bfn-class-btn[data-section="kill"]').forEach(btn => {
        if (!btn.classList.contains('active')) killed.push('"' + btn.getAttribute('data-kit') + '"');
    });
    const el = document.getElementById('bfnKillOutput');
    if (el) el.textContent = killed.length ? '[' + killed.join(',') + ']' : 'None';
}

function updateBFNMasks() {
    let pm = 0, zm = 0;
    document.querySelectorAll('.bfn-class-btn[data-section="ai"][data-team="Plants"]').forEach(btn => {
        if (!btn.classList.contains('active')) pm += parseInt(btn.getAttribute('data-mask'));
    });
    document.querySelectorAll('.bfn-class-btn[data-section="ai"][data-team="Zombies"]').forEach(btn => {
        if (!btn.classList.contains('active')) zm += parseInt(btn.getAttribute('data-mask'));
    });
    const pe = document.getElementById('bfnPlantMask');
    const ze = document.getElementById('bfnZombieMask');
    if (pe) pe.textContent = pm;
    if (ze) ze.textContent = zm;
}

function getModifierArgs() {
    const parts = [];
    document.querySelectorAll('.mod-toggle').forEach(el => {
        const key = el.getAttribute('data-key');
        if (el.type === 'checkbox') {
            if (el.checked) parts.push('-' + key + ' true');
        } else if (el.type === 'range') {
            const defaultValue = el.getAttribute('data-default');
            if (defaultValue === null || el.value !== defaultValue) {
                parts.push('-' + key + ' ' + el.value);
            }
        } else if (el.tagName === 'SELECT') {
            if (el.value) parts.push('-' + key + ' ' + el.value);
        } else {
            if (el.value) {
                const val = el.value;
                parts.push('-' + key + ' ' + (val.includes(';') ? '"' + val + '"' : val));
            }
        }
    });
    const killOutput = document.getElementById('bfnKillOutput');
    if (killOutput && killOutput.textContent !== 'None' && killOutput.textContent !== '') {
        parts.push('-SyncedPVZSettings.CharacterKillSwitches ' + killOutput.textContent);
    }
    const pm = document.getElementById('bfnPlantMask');
    const zm = document.getElementById('bfnZombieMask');
    if (pm && parseInt(pm.textContent) > 0) parts.push('-SoloPlaySettings.DisabledPlantAIClassMask ' + pm.textContent);
    if (zm && parseInt(zm.textContent) > 0) parts.push('-SoloPlaySettings.DisabledZombieAIClassMask ' + zm.textContent);
    const game = getGame();
    const levelPicker = document.getElementById('levelPicker');
    if (game === 'GW2' && levelPicker) {
        const levelVal = levelPicker.value;
        if (levelVal === 'Level_FE_Hub' || levelVal === 'Level_Hub_TacoBandits') {
            const todVal = document.getElementById('todPicker').value;
            parts.push('-GameMode.ForceHUBTimeOfDay ' + todVal.toUpperCase());
        }
    }
    return parts.join(' ');
}

function updateModifierArgsPreview() {
    if (!manualMode) {
        const el = document.getElementById('serverArgsPreview');
        if (el) el.value = getModifierArgs();
    }
    updateChangedSettingsIndicator();
}

// instance modifier system (server controls / moderator)

function populateInstanceModifiers(containerId, game) {
    const container = document.getElementById(containerId);
    if (!container) return;
    const data = typeof GAME_DATA !== 'undefined' ? GAME_DATA[game] : null;
    if (!data || !data.modifierCategories) { container.innerHTML = ''; return; }

    container.innerHTML = '';
    data.modifierCategories.forEach(cat => {
        if (cat.special === 'gw2_costumes') {
            const catDiv = document.createElement('div');
            catDiv.className = 'mod-category';
            catDiv.innerHTML = buildInstanceGW2CostumeSection(containerId);
            container.appendChild(catDiv);
            initInstanceGW2Costumes(containerId);
            return;
        }
        if (cat.special === 'bfn_classes') {
            const catDiv = document.createElement('div');
            catDiv.className = 'mod-category';
            catDiv.innerHTML = buildInstanceBFNClassSection(containerId);
            container.appendChild(catDiv);
            return;
        }
        if (cat.special) return;
        const catDiv = document.createElement('div');
        catDiv.className = 'mod-category';
        let header = '<div class="mod-category-header"><div><strong>' + cat.name + '</strong><span class="mod-category-desc">' + (cat.desc || '') + '</span></div></div>';
        let modsHtml = '<div class="mod-category-body">';
        cat.mods.forEach(mod => {
            modsHtml += '<div class="mod-card mod-card-' + mod.type + (mod.type === 'bool' ? ' mod-card-bool' : '') + '">';
            modsHtml += '<div class="mod-card-info">';
            if (mod.type !== 'bool') {
                modsHtml += '<span class="mod-card-name">' + buildModifierTitle(mod.name, mod.desc) + '</span>';
            }
            modsHtml += '</div>';
            modsHtml += '<div class="mod-card-control' + (mod.type === 'bool' ? '' : ' mod-card-control-stretch') + '">';
            if (mod.type === 'bool') {
                modsHtml += '<input type="checkbox" class="inst-mod-toggle mod-toggle-hidden" data-key="' + mod.key + '" tabindex="-1" aria-hidden="true">';
                modsHtml += '<button type="button" class="mod-bool-btn" aria-pressed="false" onclick="toggleModifierBool(this)">' + buildModifierTitle(mod.name, mod.desc) + '</button>';
            } else if (mod.type === 'slider') {
                modsHtml += '<div class="slider-control">';
                modsHtml += '<div class="slider-value"><span>' + mod.name + '</span><strong class="slider-badge" data-slider-label>' + mod.labels[mod.defaultValue] + '</strong></div>';
                modsHtml += '<input type="range" class="modifier-slider inst-mod-toggle" data-key="' + mod.key + '" data-default="' + mod.defaultValue + '" data-labels="' + encodeURIComponent(JSON.stringify(mod.labels || [])) + '" min="' + mod.min + '" max="' + mod.max + '" step="' + mod.step + '" value="' + mod.defaultValue + '" oninput="updateModifierSlider(this)">';
                modsHtml += '<div class="slider-scale">' + mod.labels.map((label, index) => '<span data-slider-step="' + index + '">' + label + '</span>').join('') + '</div>';
                modsHtml += '</div>';
            } else if (mod.type === 'select') {
                const pickerId = 'instpk_' + containerId.replace(/[^a-z0-9]/gi, '_') + '_' + mod.key.replace(/[^a-z0-9]/gi, '_');
                const pickerType = mod.key.includes('AICharacterSet') ? 'aiset' : null;
                modsHtml += '<div class="smart-picker" data-picker-id="' + pickerId + '">';
                modsHtml += '<button type="button" class="smart-picker-trigger" id="' + pickerId + 'Trigger" onclick="togglePicker(\'' + pickerId + '\')">';
                modsHtml += '<span id="' + pickerId + 'TriggerText">' + escapeHtml(mod.options[0] ? mod.options[0].n : 'Select') + '</span>';
                modsHtml += '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="6 9 12 15 18 9"/></svg></button>';
                modsHtml += '<div class="smart-picker-panel" id="' + pickerId + 'Panel">';
                modsHtml += '<div class="smart-picker-options" id="' + pickerId + 'Options"></div></div></div>';
                modsHtml += '<select class="select-input ui-hidden inst-mod-toggle" data-key="' + mod.key + '" id="' + pickerId + '">';
                mod.options.forEach(o => { modsHtml += '<option value="' + o.v + '">' + o.n + '</option>'; });
                modsHtml += '</select>';
            } else if (mod.type === 'number') {
                modsHtml += '<input type="number" class="text-input text-input-sm inst-mod-toggle" data-key="' + mod.key + '" placeholder="' + (mod.placeholder||'default') + '">';
            }
            modsHtml += '</div></div>';
        });
        modsHtml += '</div>';
        catDiv.innerHTML = header + modsHtml;
        container.appendChild(catDiv);
        // render smart pickers for select-type mods
        catDiv.querySelectorAll('.smart-picker').forEach(sp => {
            const id = sp.getAttribute('data-picker-id');
            const type = id && id.includes('AICharacterSet') ? 'aiset' : null;
            if (id && !PICKER_REGISTRY[id]) registerPicker(id, { type: type, onChange: null });
            if (id) { renderPickerOptions(id); updatePickerTrigger(id); }
        });
    });
    container.querySelectorAll('.modifier-slider').forEach(updateModifierSlider);
}

function getInstanceModifierCommands(containerId) {
    const container = document.getElementById(containerId);
    if (!container) return [];
    const cmds = [];
    container.querySelectorAll('.inst-mod-toggle').forEach(el => {
        const key = el.getAttribute('data-key');
        if (el.type === 'checkbox') {
            cmds.push(key + (el.checked ? ' true' : ' false'));
        } else if (el.type === 'range') {
            const defaultValue = el.getAttribute('data-default');
            if (defaultValue === null || el.value !== defaultValue) {
                cmds.push(key + ' ' + el.value);
            }
        } else if (el.tagName === 'SELECT') {
            if (el.value) cmds.push(key + ' ' + el.value);
        } else {
            if (el.value) cmds.push(key + ' ' + el.value);
        }
    });

    // gw2 costume restrictions
    container.querySelectorAll('.inst-gw2-costume-input').forEach(el => {
        var val = el.value.trim();
        if (val) cmds.push('GameMode.AvailableCostumes "' + val + '"');
    });

    // bfn kill switches
    container.querySelectorAll('.inst-bfn-kill-output').forEach(el => {
        var val = el.textContent.trim();
        if (val && val !== 'None') cmds.push('SyncedPVZSettings.CharacterKillSwitches ' + val);
    });
    container.querySelectorAll('.inst-bfn-plant-mask').forEach(el => {
        var val = el.textContent.trim();
        if (val && val !== '0') cmds.push('SoloPlaySettings.DisabledPlantAIClassMask ' + val);
    });
    container.querySelectorAll('.inst-bfn-zombie-mask').forEach(el => {
        var val = el.textContent.trim();
        if (val && val !== '0') cmds.push('SoloPlaySettings.DisabledZombieAIClassMask ' + val);
    });

    return cmds;
}

// instance-scoped gw2 costume section
function buildInstanceGW2CostumeSection(prefix) {
    var p = prefix + '_';
    var html = '<div class="mod-category-header"><div><strong>' + t('modifiers.char_restrictions_title') + '</strong><span class="mod-category-desc">' + t('modifiers.char_restrictions_desc') + '</span></div></div>';
    html += '<div class="mod-category-body">';
    html += '<div class="mod-card mod-card-section gw2-costume-shell">';
    html += '<div class="gw2-costume-toolbar">';
    html += '<button type="button" class="preset-btn gw2-preset-btn is-clear" data-preset="" onclick="instApplyGW2CostumePreset(\'' + p + '\', \'\')">'+t('modifiers.all_characters')+'</button>';
    html += '<button type="button" class="preset-btn gw2-quick-btn" onclick="instApplyGW2CostumeTeam(\'' + p + '\', \'Plants\')">'+t('modifiers.plants_only')+'</button>';
    html += '<button type="button" class="preset-btn gw2-quick-btn" onclick="instApplyGW2CostumeTeam(\'' + p + '\', \'Zombies\')">'+t('modifiers.zombies_only')+'</button>';
    html += '</div>';
    html += '<div class="mod-card-info"><span class="mod-card-name">' + t('modifiers.weekly_presets') + '</span></div>';
    html += '<div class="preset-grid gw2-preset-grid">';
    if (typeof GW2_COSTUME_PRESETS !== 'undefined') {
        Object.keys(GW2_COSTUME_PRESETS).forEach(function(name) {
            html += '<button type="button" class="preset-btn gw2-preset-btn" onclick="instApplyGW2CostumePreset(\'' + p + '\', ' + JSON.stringify(name).replace(/"/g, '&quot;') + ')">' + name + '</button>';
        });
    }
    html += '</div>';
    html += '<div class="gw2-costume-summary" id="' + p + 'gw2Summary">' + t('modifiers.all_chars_playable') + '</div>';
    html += '<div class="gw2-costume-team-grid">';
    if (typeof GW2_COSTUME_CLASSES !== 'undefined') {
        Object.entries(GW2_COSTUME_CLASSES).forEach(function([team, classes]) {
            html += '<section class="gw2-costume-team">';
            html += '<div class="gw2-costume-team-header"><div><strong>' + team + '</strong></div><button type="button" class="preset-btn" onclick="instApplyGW2CostumeTeam(\'' + p + '\', \'' + team + '\')">' + t('modifiers.only_team', {team: team}) + '</button></div>';
            html += '<div class="gw2-costume-class-grid">';
            classes.forEach(function(cls) {
                var classIds = cls.variants.map(function(v) { return v.id; }).join(';');
                var iconKey = (typeof GW2_CLASS_ICON !== 'undefined') ? (GW2_CLASS_ICON[cls.name] || null) : null;
                html += '<article class="gw2-costume-class-card" data-ids="' + classIds + '">';
                html += '<div class="gw2-costume-class-head"><div class="gw2-costume-class-head-left">';
                if (iconKey) html += charIconImg(iconKey, 'class-icon');
                html += '<span class="gw2-costume-class-name">' + cls.name + '</span></div>';
                html += '<span class="gw2-class-count">0/' + cls.variants.length + '</span></div>';
                html += '<div class="gw2-costume-variant-grid">';
                cls.variants.forEach(function(v) {
                    var varIcon = (typeof GW2_VARIANT_ICON !== 'undefined') ? (GW2_VARIANT_ICON[v.id] || null) : null;
                    html += '<button type="button" class="gw2-costume-btn" data-id="' + v.id + '" data-prefix="' + p + '" onclick="instToggleGW2Costume(this)">';
                    if (varIcon) html += charIconImg(varIcon);
                    html += '<span>' + v.name + '</span></button>';
                });
                html += '</div></article>';
            });
            html += '</div></section>';
        });
    }
    html += '</div>';
    html += '<input type="text" class="text-input text-input-sm inst-gw2-costume-input" id="' + p + 'gw2Ids" placeholder="' + t('modifiers.costume_placeholder') + '" oninput="instSyncGW2CostumeUi(\'' + p + '\')">';
    html += '</div></div>';
    return html;
}

function initInstanceGW2Costumes(prefix) {
    // start with all enabled (empty = all)
}

function instToggleGW2Costume(btn) {
    var prefix = btn.getAttribute('data-prefix');
    btn.classList.toggle('active');
    instCollectGW2Costumes(prefix);
}

function instCollectGW2Costumes(prefix) {
    var container = document.getElementById(prefix + 'gw2Ids');
    if (!container) return;
    var parent = container.closest('.gw2-costume-shell');
    if (!parent) return;
    var ids = [];
    parent.querySelectorAll('.gw2-costume-btn.active').forEach(function(btn) {
        ids.push(btn.getAttribute('data-id'));
    });
    var allBtns = parent.querySelectorAll('.gw2-costume-btn');
    if (ids.length === allBtns.length) {
        container.value = '';
    } else {
        container.value = ids.join(';');
    }
    var summary = document.getElementById(prefix + 'gw2Summary');
    if (summary) {
        summary.textContent = ids.length === allBtns.length ? t('modifiers.all_chars_playable') : t('modifiers.n_of_n_enabled', {n: ids.length, total: allBtns.length});
    }
    // update class counts
    parent.querySelectorAll('.gw2-costume-class-card').forEach(function(card) {
        var total = card.querySelectorAll('.gw2-costume-btn').length;
        var active = card.querySelectorAll('.gw2-costume-btn.active').length;
        var countEl = card.querySelector('.gw2-class-count');
        if (countEl) countEl.textContent = active + '/' + total;
    });
}

function instSyncGW2CostumeUi(prefix) {
    var input = document.getElementById(prefix + 'gw2Ids');
    if (!input) return;
    var parent = input.closest('.gw2-costume-shell');
    if (!parent) return;
    var raw = input.value.trim();
    var allowed = raw ? raw.split(';').map(function(s) { return s.trim(); }) : [];
    parent.querySelectorAll('.gw2-costume-btn').forEach(function(btn) {
        btn.classList.toggle('active', allowed.length === 0 || allowed.indexOf(btn.getAttribute('data-id')) !== -1);
    });
    instCollectGW2Costumes(prefix);
}

function instApplyGW2CostumePreset(prefix, name) {
    var input = document.getElementById(prefix + 'gw2Ids');
    if (!input) return;
    if (!name || typeof GW2_COSTUME_PRESETS === 'undefined' || !GW2_COSTUME_PRESETS[name]) {
        input.value = '';
    } else {
        input.value = GW2_COSTUME_PRESETS[name].join(';');
    }
    instSyncGW2CostumeUi(prefix);
}

function instApplyGW2CostumeTeam(prefix, team) {
    var input = document.getElementById(prefix + 'gw2Ids');
    if (!input) return;
    if (typeof GW2_COSTUME_CLASSES === 'undefined') return;
    var ids = [];
    Object.entries(GW2_COSTUME_CLASSES).forEach(function([t, classes]) {
        if (t === team) {
            classes.forEach(function(cls) {
                cls.variants.forEach(function(v) { ids.push(String(v.id)); });
            });
        }
    });
    input.value = ids.join(';');
    instSyncGW2CostumeUi(prefix);
}

// instance-scoped bfn class section
function buildInstanceBFNClassSection(prefix) {
    var p = prefix + '_';
    var html = '<div class="mod-category-header"><div><strong>' + t('modifiers.char_restrictions_title') + '</strong><span class="mod-category-desc">' + t('modifiers.restrict_classes_desc') + '</span></div></div>';
    html += '<div class="mod-category-body">';
    html += '<div class="mod-card mod-card-section bfn-class-shell">';
    html += '<div class="mod-card-info"><span class="mod-card-name">' + t('modifiers.class_kill_switches_title') + '</span><span class="mod-card-desc">' + t('modifiers.class_kill_switches_desc') + '</span></div>';
    html += '<div class="bfn-class-toolbar">';
    html += '<button type="button" class="preset-btn bfn-preset-btn is-clear" data-preset="" onclick="instApplyBFNKillPreset(\'' + p + '\', \'\')">'+t('modifiers.all_playable')+'</button>';
    html += '</div>';
    if (typeof BFN_KILLSWITCH_PRESETS !== 'undefined') {
        html += '<div class="mod-card-info"><span class="mod-card-name">' + t('modifiers.weekly_presets') + '</span></div>';
        html += '<div class="preset-grid bfn-preset-grid">';
        Object.keys(BFN_KILLSWITCH_PRESETS).forEach(function(name) {
            html += '<button type="button" class="preset-btn bfn-preset-btn" data-preset="' + name + '" onclick="instApplyBFNKillPreset(\'' + p + '\', ' + JSON.stringify(name).replace(/"/g, '&quot;') + ')">' + name + '</button>';
        });
        html += '</div>';
    }
    html += '<div class="bfn-class-summary" id="' + p + 'bfnKillSummary">' + t('modifiers.all_classes_playable') + '</div>';
    html += '<div class="bfn-class-team-grid">';
    if (typeof BFN_CLASSES !== 'undefined') {
        Object.entries(BFN_CLASSES).forEach(function([team, classes]) {
            html += '<section class="bfn-class-team">';
            html += '<div class="bfn-class-team-header"><div><strong>' + team + '</strong></div></div>';
            html += '<div class="bfn-class-btn-grid">';
            classes.forEach(function(c) {
                var iconKey = (typeof BFN_CLASS_ICON !== 'undefined') ? (BFN_CLASS_ICON[c.name] || null) : null;
                html += '<button type="button" class="bfn-class-btn active" data-kit="' + c.kit + '" data-section="kill" data-prefix="' + p + '" onclick="instToggleBFNClass(this)">';
                if (iconKey) html += charIconImg(iconKey);
                html += '<span>' + c.name + '</span></button>';
            });
            html += '</div></section>';
        });
    }
    html += '</div>';
    html += '<div class="bfn-class-divider"></div>';
    html += '<div class="mod-card-info"><span class="mod-card-name">' + t('modifiers.ai_spawn_restrictions_title') + '</span><span class="mod-card-desc">' + t('modifiers.ai_spawn_restrictions_desc') + '</span></div>';
    html += '<div class="bfn-class-summary" id="' + p + 'bfnAiSummary">' + t('modifiers.all_ai_spawn') + '</div>';
    html += '<div class="bfn-class-team-grid">';
    if (typeof BFN_CLASSES !== 'undefined') {
        Object.entries(BFN_CLASSES).forEach(function([team, classes]) {
            html += '<section class="bfn-class-team">';
            html += '<div class="bfn-class-team-header"><div><strong>' + team + ' AI</strong></div></div>';
            html += '<div class="bfn-class-btn-grid">';
            classes.forEach(function(c) {
                var iconKey = (typeof BFN_CLASS_ICON !== 'undefined') ? (BFN_CLASS_ICON[c.name] || null) : null;
                html += '<button type="button" class="bfn-class-btn active" data-mask="' + c.mask + '" data-team="' + team + '" data-section="ai" data-prefix="' + p + '" onclick="instToggleBFNClass(this)">';
                if (iconKey) html += charIconImg(iconKey);
                html += '<span>' + c.name + '</span></button>';
            });
            html += '</div></section>';
        });
    }
    html += '</div>';
    html += '<code class="inst-bfn-kill-output" id="' + p + 'bfnKillOutput" style="display:none;">None</code>';
    html += '<code class="inst-bfn-plant-mask" id="' + p + 'bfnPlantMask" style="display:none;">0</code>';
    html += '<code class="inst-bfn-zombie-mask" id="' + p + 'bfnZombieMask" style="display:none;">0</code>';
    html += '</div></div>';
    return html;
}

function instToggleBFNClass(btn) {
    btn.classList.toggle('active');
    var prefix = btn.getAttribute('data-prefix');
    var section = btn.getAttribute('data-section');
    if (section === 'kill') {
        instUpdateBFNKillOutput(prefix);
    } else {
        instUpdateBFNMasks(prefix);
    }
}

function instUpdateBFNKillOutput(prefix) {
    var shell = document.getElementById(prefix + 'bfnKillOutput');
    if (!shell) return;
    var parent = shell.closest('.bfn-class-shell');
    var killed = [];
    parent.querySelectorAll('.bfn-class-btn[data-section="kill"]:not(.active)').forEach(function(btn) {
        killed.push('"' + btn.getAttribute('data-kit') + '"');
    });
    shell.textContent = killed.length ? '[' + killed.join(',') + ']' : 'None';
    var summary = document.getElementById(prefix + 'bfnKillSummary');
    if (summary) {
        var all = parent.querySelectorAll('.bfn-class-btn[data-section="kill"]').length;
        var active = parent.querySelectorAll('.bfn-class-btn[data-section="kill"].active').length;
        summary.textContent = active === all ? t('modifiers.all_classes_playable') : t('modifiers.n_of_n_classes_playable', {n: active, total: all});
    }
}

function instUpdateBFNMasks(prefix) {
    var plantMaskEl = document.getElementById(prefix + 'bfnPlantMask');
    var zombieMaskEl = document.getElementById(prefix + 'bfnZombieMask');
    if (!plantMaskEl || !zombieMaskEl) return;
    var parent = plantMaskEl.closest('.bfn-class-shell');
    var plantMask = 0, zombieMask = 0;
    parent.querySelectorAll('.bfn-class-btn[data-section="ai"]:not(.active)').forEach(function(btn) {
        var mask = parseInt(btn.getAttribute('data-mask')) || 0;
        var team = btn.getAttribute('data-team');
        if (team === 'Plants') plantMask |= mask;
        else zombieMask |= mask;
    });
    plantMaskEl.textContent = plantMask;
    zombieMaskEl.textContent = zombieMask;
    var summary = document.getElementById(prefix + 'bfnAiSummary');
    if (summary) {
        var all = parent.querySelectorAll('.bfn-class-btn[data-section="ai"]').length;
        var active = parent.querySelectorAll('.bfn-class-btn[data-section="ai"].active').length;
        summary.textContent = active === all ? t('modifiers.all_ai_spawn') : t('modifiers.n_of_n_ai_enabled', {n: active, total: all});
    }
}

function instApplyBFNKillPreset(prefix, name) {
    var shell = document.getElementById(prefix + 'bfnKillOutput');
    if (!shell) return;
    var parent = shell.closest('.bfn-class-shell');
    var preset = (typeof BFN_KILLSWITCH_PRESETS !== 'undefined' && name) ? (BFN_KILLSWITCH_PRESETS[name] || []) : [];
    parent.querySelectorAll('.bfn-class-btn[data-section="kill"]').forEach(function(btn) {
        btn.classList.toggle('active', !preset.includes(btn.getAttribute('data-kit')));
    });
    instUpdateBFNKillOutput(prefix);
}
