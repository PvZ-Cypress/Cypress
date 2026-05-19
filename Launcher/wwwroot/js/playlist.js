function plSetTod(selectId, value, btn) {
    const sel = document.getElementById(selectId);
    if (sel) sel.value = value;
    if (btn) {
        btn.closest('.segmented-control').querySelectorAll('.segmented-btn').forEach(b => {
            b.classList.toggle('active', b.getAttribute('data-value') === value);
        });
        btn.blur();
    }
}

function addPlaylistEntry() {
    const game = getGame();
    const data = GAME_DATA[game];
    const idx = plEntryCount++;
    const div = document.createElement('div');
    div.className = 'pl-entry';
    div.id = 'plEntry' + idx;

    const levelId = 'plLevel' + idx;
    const modeId = 'plMode' + idx;
    const todId = 'plTod' + idx;
    const modContainerId = 'plMods' + idx;

    // level options, use bare l.id so picker bg lookup works
    let levelOpts = '';
    const cats = {};
    data.levels.forEach(l => { if (!cats[l.cat]) cats[l.cat] = []; cats[l.cat].push(l); });
    for (const cat in cats) {
        levelOpts += '<optgroup label="' + cat + '">';
        cats[cat].forEach(l => {
            let optVal = (game === 'GW1' && l.variant !== undefined) ? l.id + '#' + l.variant : l.id;
            levelOpts += '<option value="' + optVal + '">' + l.name + '</option>';
        });
        levelOpts += '</optgroup>';
    }
    let modeOpts = '';
    const modeCats = {};
    data.modes.forEach(m => { if (!modeCats[m.cat]) modeCats[m.cat] = []; modeCats[m.cat].push(m); });
    for (const cat in modeCats) {
        modeOpts += '<optgroup label="' + cat + '">';
        modeCats[cat].forEach(m => { modeOpts += '<option value="' + m.id + '">' + m.name + '</option>'; });
        modeOpts += '</optgroup>';
    }

    let spSection = '';
    if (game === 'BFN') {
        let spOpts = data.startPoints.map(s => '<option value="' + s.id + '">' + s.name + '</option>').join('');
        spSection = '<div class="pl-field"><label class="field-label">' + t('mapmode.start_point_label') + '</label><select class="select-input select-input-sm" data-pl="startpoint">' + spOpts + '</select></div>';
    }

    const chevron = '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="6 9 12 15 18 9"/></svg>';

    div.innerHTML = '<div class="pl-entry-header"><span class="pl-entry-num">#' + (idx + 1) + '</span><button class="btn btn-sm btn-danger" onclick="removePlaylistEntry(' + idx + ')">' + t('playlist.entry_remove') + '</button></div>'
        + '<div class="pl-entry-fields">'
        + '<div class="pl-field"><label class="field-label">' + t('mapmode.level_label') + '</label>'
        + '<div class="smart-picker" data-picker-id="' + levelId + '">'
        + '<button type="button" class="smart-picker-trigger" id="' + levelId + 'Trigger" onclick="togglePicker(\'' + levelId + '\')">'
        + '<span id="' + levelId + 'TriggerText">' + t('mapmode.select_level') + '</span>' + chevron + '</button>'
        + '<div class="smart-picker-panel" id="' + levelId + 'Panel">'
        + '<input type="text" id="' + levelId + 'Search" class="text-input picker-search" placeholder="' + t('mapmode.filter_levels') + '" oninput="filterPickerOptions(\'' + levelId + '\')">'
        + '<div class="smart-picker-options" id="' + levelId + 'Options"></div></div></div>'
        + '<select id="' + levelId + '" class="select-input ui-hidden" data-pl="level">' + levelOpts + '</select></div>'
        + '<div class="pl-field"><label class="field-label">' + t('mapmode.mode_label') + '</label>'
        + '<div class="smart-picker" data-picker-id="' + modeId + '">'
        + '<button type="button" class="smart-picker-trigger" id="' + modeId + 'Trigger" onclick="togglePicker(\'' + modeId + '\')">'
        + '<span id="' + modeId + 'TriggerText">' + t('mapmode.select_mode') + '</span>' + chevron + '</button>'
        + '<div class="smart-picker-panel" id="' + modeId + 'Panel">'
        + '<input type="text" id="' + modeId + 'Search" class="text-input picker-search" placeholder="' + t('mapmode.filter_modes') + '" oninput="filterPickerOptions(\'' + modeId + '\')">'
        + '<div class="smart-picker-options" id="' + modeId + 'Options"></div></div></div>'
        + '<select id="' + modeId + '" class="select-input ui-hidden" data-pl="mode">' + modeOpts + '</select></div>'
        + spSection
        + '<div class="pl-field"><label class="field-label">' + t('playlist.entry_tod_label') + '</label>'
        + '<div class="segmented-control" data-select-target="' + todId + '">'
        + '<button type="button" class="segmented-btn active" data-value="" onclick="plSetTod(\'' + todId + '\',\'\',this)">' + t('playlist.entry_tod_default') + '</button>'
        + '<button type="button" class="segmented-btn" data-value="Day" onclick="plSetTod(\'' + todId + '\',\'Day\',this)" title="Day"><svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="5"/><line x1="12" y1="1" x2="12" y2="3"/><line x1="12" y1="21" x2="12" y2="23"/><line x1="4.22" y1="4.22" x2="5.64" y2="5.64"/><line x1="18.36" y1="18.36" x2="19.78" y2="19.78"/><line x1="1" y1="12" x2="3" y2="12"/><line x1="21" y1="12" x2="23" y2="12"/><line x1="4.22" y1="19.78" x2="5.64" y2="18.36"/><line x1="18.36" y1="5.64" x2="19.78" y2="4.22"/></svg></button>'
        + '<button type="button" class="segmented-btn" data-value="Night" onclick="plSetTod(\'' + todId + '\',\'Night\',this)" title="Night"><svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/></svg></button>'
        + '</div>'
        + '<select id="' + todId + '" class="select-input ui-hidden" data-pl="tod"><option value="">Default</option><option value="Day">Day</option><option value="Night">Night</option></select></div>'
        + '<div class="pl-field full-width pl-mods-toggle"><button class="btn btn-sm btn-secondary" onclick="this.nextElementSibling.style.display=this.nextElementSibling.style.display===\'none\'?\'\':\'none\'">' + t('playlist.entry_game_modifiers_btn') + '</button>'
        + '<div class="pl-mods-container" id="' + modContainerId + '" style="display:none;"></div></div>'
        + '<div class="pl-field full-width pl-loadscreen-toggle"><button class="btn btn-sm btn-secondary" onclick="this.nextElementSibling.style.display=this.nextElementSibling.style.display===\'none\'?\'\':\'none\'">' + t('playlist.entry_loadscreen_overrides_btn') + '</button>'
        + '<div class="pl-loadscreen-fields" style="display:none;">'
        + '<div class="pl-field"><label class="field-label">' + t('playlist.mode_name_override') + '</label><input type="text" class="text-input text-input-sm" data-pl="ls_mode" placeholder="' + t('playlist.entry_mode_name_placeholder') + '"></div>'
        + '<div class="pl-field"><label class="field-label">' + t('playlist.level_name_override') + '</label><input type="text" class="text-input text-input-sm" data-pl="ls_level" placeholder="' + t('playlist.entry_level_name_placeholder') + '"></div>'
        + '<div class="pl-field"><label class="field-label">' + t('playlist.desc_override') + '</label><input type="text" class="text-input text-input-sm" data-pl="ls_desc" placeholder="' + t('playlist.entry_desc_placeholder') + '"></div>'
        + '<div class="pl-field"><label class="field-label">' + t('playlist.asset_override') + '</label><input type="text" class="text-input text-input-sm" data-pl="ls_asset" placeholder="' + t('playlist.entry_asset_placeholder') + '"></div>'
        + '</div></div>'
        + '</div>';

    document.getElementById('plEntries').appendChild(div);

    // register and render smart pickers
    registerPicker(levelId, { type: 'level' });
    registerPicker(modeId, { type: 'mode' });
    renderPickerOptions(levelId);
    renderPickerOptions(modeId);
    updatePickerTrigger(levelId);
    updatePickerTrigger(modeId);

    // populate modifier toggles
    if (typeof populateInstanceModifiers === 'function') {
        populateInstanceModifiers(modContainerId, game);
    }
}

// extract settings string from a playlist entry's modifier container
function getPlaylistEntrySettings(entry) {
    const modContainer = entry.querySelector('.pl-mods-container');
    if (!modContainer) return '';
    const parts = [];
    modContainer.querySelectorAll('.inst-mod-toggle').forEach(el => {
        const key = el.getAttribute('data-key');
        if (el.type === 'checkbox') {
            if (el.checked) parts.push(key + ' true');
        } else if (el.type === 'range') {
            const def = el.getAttribute('data-default');
            if (def === null || el.value !== def) parts.push(key + ' ' + el.value);
        } else if (el.tagName === 'SELECT') {
            if (el.value) parts.push(key + ' ' + el.value);
        } else {
            if (el.value) parts.push(key + ' ' + el.value);
        }
    });
    return parts.join('|');
}

// convert bare level id to full frostbite path for playlist json
function levelIdToPlaylistPath(levelId, game) {
    if (game === 'GW1') return levelId;
    return 'Levels/' + levelId + '/' + levelId;
}

function removePlaylistEntry(idx) {
    const el = document.getElementById('plEntry' + idx);
    if (el) el.remove();
}

function onPlModeChanged() {
    const mode = document.getElementById('plMode').value;
    document.getElementById('plOrderedSection').style.display = mode === 'ordered' ? '' : 'none';
    document.getElementById('plMixedSection').style.display = mode === 'mixed' ? '' : 'none';
}

function generatePlaylistJSON() {
    const mode = document.getElementById('plMode').value;
    const rounds = parseInt(document.getElementById('plRoundsPerSetup').value) || 1;
    const obj = { RoundsPerSetup: rounds, IsMixed: mode === 'mixed' };

    const gmo = document.getElementById('plGlobalModeName')?.value;
    const glo = document.getElementById('plGlobalLevelName')?.value;
    const gdo = document.getElementById('plGlobalDesc')?.value;
    const gao = document.getElementById('plGlobalAsset')?.value;
    if (gmo) obj.Loadscreen_GamemodeNameOverride = gmo;
    if (glo) obj.Loadscreen_LevelNameOverride = glo;
    if (gdo) obj.Loadscreen_LevelDescriptionOverride = gdo;
    if (gao) obj.Loadscreen_UIAssetPathOverride = gao;

    if (mode === 'ordered') {
        obj.PlaylistRotation = [];
        var currentGame = getGame();
        document.querySelectorAll('.pl-entry').forEach(entry => {
            const e = {};
            const get = s => entry.querySelector('[data-pl="' + s + '"]');
            if (get('level')) {
                let levelVal = get('level').value;
                let variant = '0';
                if (currentGame === 'GW1' && levelVal.includes('#')) {
                    const idx = levelVal.lastIndexOf('#');
                    variant = levelVal.substring(idx + 1);
                    levelVal = levelVal.substring(0, idx);
                }
                e.LevelName = levelIdToPlaylistPath(levelVal, currentGame);
                if (get('mode')) {
                    let modeVal = get('mode').value;
                    if (currentGame === 'GW1') {
                        modeVal = modeVal + variant;
                    }
                    e.GameMode = modeVal;
                }
            } else if (get('mode')) {
                e.GameMode = get('mode').value;
            }
            if (get('startpoint')?.value) e.StartPoint = get('startpoint').value;
            if (get('tod')?.value) e.TOD = get('tod').value;
            // build settings from modifier toggles
            var settingsStr = getPlaylistEntrySettings(entry);
            if (settingsStr) e.SettingsToApply = settingsStr;
            if (get('ls_mode')?.value) e.Loadscreen_GamemodeName = get('ls_mode').value;
            if (get('ls_level')?.value) e.Loadscreen_LevelName = get('ls_level').value;
            if (get('ls_desc')?.value) e.Loadscreen_LevelDescription = get('ls_desc').value;
            if (get('ls_asset')?.value) e.Loadscreen_UIAssetPath = get('ls_asset').value;
            obj.PlaylistRotation.push(e);
        });
    } else {
        const modes = document.getElementById('plMixedModes').value.split(',').map(s => s.trim()).filter(Boolean);
        const levels = document.getElementById('plMixedLevels').value.split(',').map(s => s.trim()).filter(Boolean);
        obj.AvailableModes = modes;
        obj.AvailableLevelsForModes = {};
        modes.forEach(m => { obj.AvailableLevelsForModes[m] = levels; });
        const todText = document.getElementById('plMixedTOD')?.value?.trim();
        if (todText) {
            try { obj.AvailableTODForLevels = JSON.parse(todText); } catch(e) {}
        }
    }

    document.getElementById('plOutput').value = JSON.stringify(obj, null, 2);
}

function copyPlaylistJSON() {
    const text = document.getElementById('plOutput').value;
    if (text) { navigator.clipboard.writeText(text); showStatus(t('playlist.copied'), 'success'); }
}

// save/load/delete playlist files

function refreshPlaylistFileList() {
    send('getPlaylists', {});
}

function populatePlaylistFileSelect(files) {
    var sel = document.getElementById('plFileSelect');
    if (!sel) return;
    var prev = sel.value;
    sel.innerHTML = '<option value="">' + t('playlist.new_playlist') + '</option>';
    (files || []).forEach(function(f) {
        var opt = document.createElement('option');
        opt.value = f;
        opt.textContent = f;
        sel.appendChild(opt);
    });
    if (prev) sel.value = prev;
}

async function promptSavePlaylist() {
    generatePlaylistJSON();
    var content = document.getElementById('plOutput').value;
    if (!content || !content.trim()) {
        showStatus(t('playlist.generate_first'), 'warning');
        return;
    }
    var sel = document.getElementById('plFileSelect');
    var existing = sel ? sel.value : '';
    var name = await cypressPrompt(t('playlist.save_prompt_title'), t('playlist.save_filename_label'), existing || 'MyPlaylist.json');
    if (!name) return;
    send('savePlaylist', { name: name, content: content });
}

function loadSelectedPlaylist() {
    var sel = document.getElementById('plFileSelect');
    var name = sel ? sel.value : '';
    if (!name) {
        clearPlaylistBuilder();
        return;
    }
    send('loadPlaylist', { name: name });
}

async function deleteSelectedPlaylist() {
    var sel = document.getElementById('plFileSelect');
    var name = sel ? sel.value : '';
    if (!name) return;
    if (!await cypressConfirm(t('playlist.delete_confirm_title'), t('playlist.delete_confirm_body', {name: name}))) return;
    send('deletePlaylist', { name: name });
}

function onPlaylistSaved(data) {
    if (data.ok) {
        showStatus(t('playlist.saved', {name: data.name || ''}), 'success');
        var sel = document.getElementById('plFileSelect');
        if (sel && data.name) sel.value = data.name;
    } else {
        showStatus(t('playlist.save_failed', {error: data.error || ''}), 'error');
    }
}

function onPlaylistLoaded(data) {
    if (!data.ok) {
        showStatus(t('playlist.load_failed', {error: data.error || ''}), 'error');
        return;
    }
    try {
        var obj = JSON.parse(data.content);
        importPlaylistJSON(obj);
        document.getElementById('plOutput').value = JSON.stringify(obj, null, 2);
        showStatus(t('playlist.loaded', {name: data.name || ''}), 'success');
    } catch (e) {
        showStatus(t('playlist.invalid_json'), 'error');
    }
}

function onPlaylistDeleted(data) {
    if (data.ok) {
        showStatus(t('playlist.deleted', {name: data.name || ''}), 'success');
        var sel = document.getElementById('plFileSelect');
        if (sel) sel.value = '';
    } else {
        showStatus(t('playlist.delete_failed', {error: data.error || ''}), 'error');
    }
}

function clearPlaylistBuilder() {
    document.getElementById('plEntries').innerHTML = '';
    plEntryCount = 0;
    document.getElementById('plRoundsPerSetup').value = '1';
    document.getElementById('plMode').value = 'ordered';
    onPlModeChanged();
    document.getElementById('plMixedModes').value = '';
    document.getElementById('plMixedLevels').value = '';
    var todEl = document.getElementById('plMixedTOD');
    if (todEl) todEl.value = '';
    ['plGlobalModeName', 'plGlobalLevelName', 'plGlobalDesc', 'plGlobalAsset'].forEach(function(id) {
        var el = document.getElementById(id);
        if (el) el.value = '';
    });
    document.getElementById('plOutput').value = '';
}

// reverse of generatePlaylistJSON: populate builder from a playlist object
function importPlaylistJSON(obj) {
    clearPlaylistBuilder();
    if (obj.RoundsPerSetup) document.getElementById('plRoundsPerSetup').value = obj.RoundsPerSetup;
    if (obj.Loadscreen_GamemodeNameOverride) document.getElementById('plGlobalModeName').value = obj.Loadscreen_GamemodeNameOverride;
    if (obj.Loadscreen_LevelNameOverride) document.getElementById('plGlobalLevelName').value = obj.Loadscreen_LevelNameOverride;
    if (obj.Loadscreen_LevelDescriptionOverride) document.getElementById('plGlobalDesc').value = obj.Loadscreen_LevelDescriptionOverride;
    if (obj.Loadscreen_UIAssetPathOverride) document.getElementById('plGlobalAsset').value = obj.Loadscreen_UIAssetPathOverride;

    if (obj.IsMixed) {
        document.getElementById('plMode').value = 'mixed';
        onPlModeChanged();
        if (obj.AvailableModes) document.getElementById('plMixedModes').value = obj.AvailableModes.join(', ');
        // flatten all levels from the map
        if (obj.AvailableLevelsForModes) {
            var allLevels = [];
            Object.values(obj.AvailableLevelsForModes).forEach(function(arr) {
                arr.forEach(function(l) { if (allLevels.indexOf(l) === -1) allLevels.push(l); });
            });
            document.getElementById('plMixedLevels').value = allLevels.join(', ');
        }
        if (obj.AvailableTODForLevels) {
            var todEl = document.getElementById('plMixedTOD');
            if (todEl) todEl.value = JSON.stringify(obj.AvailableTODForLevels, null, 2);
        }
    } else {
        document.getElementById('plMode').value = 'ordered';
        onPlModeChanged();
        if (obj.PlaylistRotation && obj.PlaylistRotation.length) {
            obj.PlaylistRotation.forEach(function(entry) {
                addPlaylistEntry();
                var entryEl = document.getElementById('plEntry' + (plEntryCount - 1));
                if (!entryEl) return;
                var get = function(s) { return entryEl.querySelector('[data-pl="' + s + '"]'); };

                // resolve level id from frostbite path
                if (entry.LevelName) {
                    var parts = entry.LevelName.split('/');
                    var levelId = parts[parts.length - 1] || parts[parts.length - 2] || entry.LevelName;
                    var levelSel = get('level');
                    if (levelSel) {
                        // try exact match, otherwise try finding by id
                        if (levelSel.querySelector('option[value="' + levelId + '"]')) {
                            levelSel.value = levelId;
                        }
                        var pickerId = levelSel.id;
                        if (pickerId && typeof updatePickerTrigger === 'function') updatePickerTrigger(pickerId);
                    }
                }
                if (entry.GameMode) {
                    var modeSel = get('mode');
                    if (modeSel) {
                        modeSel.value = entry.GameMode;
                        var pickerId = modeSel.id;
                        if (pickerId && typeof updatePickerTrigger === 'function') updatePickerTrigger(pickerId);
                    }
                }
                if (entry.StartPoint && get('startpoint')) get('startpoint').value = entry.StartPoint;
                if (entry.TOD) {
                    var todSel = get('tod');
                    if (todSel) {
                        todSel.value = entry.TOD;
                        var ctrl = entryEl.querySelector('.segmented-control');
                        if (ctrl) {
                            ctrl.querySelectorAll('.segmented-btn').forEach(function(b) {
                                b.classList.toggle('active', b.getAttribute('data-value') === entry.TOD);
                            });
                        }
                    }
                }
                // loadscreen overrides
                if (entry.Loadscreen_GamemodeName && get('ls_mode')) get('ls_mode').value = entry.Loadscreen_GamemodeName;
                if (entry.Loadscreen_LevelName && get('ls_level')) get('ls_level').value = entry.Loadscreen_LevelName;
                if (entry.Loadscreen_LevelDescription && get('ls_desc')) get('ls_desc').value = entry.Loadscreen_LevelDescription;
                if (entry.Loadscreen_UIAssetPath && get('ls_asset')) get('ls_asset').value = entry.Loadscreen_UIAssetPath;
            });
        }
    }
}
