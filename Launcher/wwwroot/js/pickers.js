// unified smart picker system
// each picker is registered with a type ('level', 'mode', 'game', or null)
// that determines background image handling automatically.
// callbacks fire when a value is selected via the picker ui.

const PICKER_REGISTRY = {};

function registerPicker(selectId, opts) {
    PICKER_REGISTRY[selectId] = {
        type: (opts && opts.type) || null,       // 'level' | 'mode' | 'game' | null
        onChange: (opts && opts.onChange) || null  // callback when value selected
    };
}

// register all pickers up front
registerPicker('levelPicker',    { type: 'level', onChange: () => { if (typeof onLevelPickerChanged === 'function') onLevelPickerChanged(); } });
registerPicker('modePicker',     { type: 'mode',  onChange: () => { if (typeof onModePickerChanged === 'function') onModePickerChanged(); } });
registerPicker('startPointPicker', { type: null,  onChange: () => { if (typeof onInclusionChanged === 'function') onInclusionChanged(); } });
registerPicker('srvLevel',       { type: 'level', onChange: () => { if (typeof onSrvLevelChanged === 'function') onSrvLevelChanged(); } });
registerPicker('srvMode',        { type: 'mode',  onChange: () => { if (typeof onSrvModeChanged === 'function') onSrvModeChanged(); } });
registerPicker('modLevel',       { type: 'level', onChange: () => { if (typeof onModLevelChanged === 'function') onModLevelChanged(); } });
registerPicker('modMode',        { type: 'mode',  onChange: () => { if (typeof onModModeChanged === 'function') onModModeChanged(); } });
registerPicker('browserFilterGame', { type: 'game', onChange: () => { if (typeof filterBrowserList === 'function') filterBrowserList(); } });
registerPicker('browserSort', { type: null, onChange: () => { if (typeof filterBrowserList === 'function') filterBrowserList(); } });
registerPicker('browserFilterModded', { type: null, onChange: () => { if (typeof filterBrowserList === 'function') filterBrowserList(); } });
registerPicker('srvStartPoint',  { type: null, onChange: null });
registerPicker('modStartPoint',  { type: null, onChange: null });
registerPicker('playlistSelect', { type: null, onChange: null });

// game bg cache, populated lazily from pre-loaded bgData* img elements
const GAME_BG_CACHE = {};

function initGameBgCache() {
    if (GAME_BG_CACHE._init) return;
    GAME_BG_CACHE._init = true;
    ['GW1', 'GW2', 'BFN'].forEach(g => {
        const el = document.getElementById('bgData' + g);
        if (!el) return;
        const src = el.getAttribute('src') || '';
        const b64 = src.indexOf(',') !== -1 ? src.split(',')[1] : null;
        if (b64) GAME_BG_CACHE[g] = typeof base64ToBlobUrl === 'function' ? base64ToBlobUrl(b64) : src;
    });
}

// background helpers

function getPickerBgKey(selectId, value) {
    const reg = PICKER_REGISTRY[selectId];
    if (!reg) return null;
    if (reg.type === 'level' && typeof LEVEL_MAP_BG !== 'undefined') {
        // try full value first (includes GW1 variant suffix), then stripped
        if (LEVEL_MAP_BG[value]) return LEVEL_MAP_BG[value];
        const levelId = value.includes('#') ? value.substring(0, value.lastIndexOf('#')) : value;
        return LEVEL_MAP_BG[levelId] || null;
    }
    if (reg.type === 'mode'  && typeof MODE_BG !== 'undefined')      return MODE_BG[value] || null;
    if (reg.type === 'game')  return value || null;
    if (reg.type === 'aiset' && typeof AI_SET_ID_TO_BG !== 'undefined') return AI_SET_ID_TO_BG[value] || null;
    return null;
}

function getPickerBgCache(selectId) {
    const reg = PICKER_REGISTRY[selectId];
    if (!reg) return null;
    if (reg.type === 'level' && typeof MAP_BG_CACHE !== 'undefined')     return MAP_BG_CACHE;
    if (reg.type === 'mode'  && typeof MODE_BG_CACHE !== 'undefined')    return MODE_BG_CACHE;
    if (reg.type === 'game')  { initGameBgCache(); return GAME_BG_CACHE; }
    if (reg.type === 'aiset' && typeof AI_SET_BG_CACHE !== 'undefined') return AI_SET_BG_CACHE;
    return null;
}

function requestPickerBg(selectId, bgKey) {
    const reg = PICKER_REGISTRY[selectId];
    if (!reg || !bgKey) return;
    if (reg.type === 'level') send('getMapBg', { key: bgKey });
    if (reg.type === 'mode')  send('getModeBg', { key: bgKey });
    if (reg.type === 'aiset') send('getAiSetBg', { key: bgKey });
    // game bgs are already loaded from dom, no request needed
}

function getPickerIdsOfType(type) {
    return Object.keys(PICKER_REGISTRY).filter(id => PICKER_REGISTRY[id].type === type);
}

// called when a map bg arrives from backend
function updatePickerOptionBgs(bgKey) {
    document.querySelectorAll('.picker-option[data-bg-key="' + bgKey + '"]').forEach(btn => {
        if (!btn.querySelector('.picker-option-bg') && MAP_BG_CACHE[bgKey]) {
            const img = document.createElement('img');
            img.className = 'picker-option-bg';
            img.src = MAP_BG_CACHE[bgKey];
            img.alt = '';
            img.draggable = false;
            btn.insertBefore(img, btn.firstChild);
        }
    });
    getPickerIdsOfType('level').forEach(id => updatePickerTrigger(id));
}

// called when a mode bg arrives from backend
function updateModePickerOptionBgs(bgKey) {
    document.querySelectorAll('.picker-option[data-bg-key="' + bgKey + '"]').forEach(btn => {
        if (!btn.querySelector('.picker-option-bg') && MODE_BG_CACHE[bgKey]) {
            const img = document.createElement('img');
            img.className = 'picker-option-bg picker-option-bg-mode';
            img.src = MODE_BG_CACHE[bgKey];
            img.alt = '';
            img.draggable = false;
            btn.insertBefore(img, btn.firstChild);
        }
    });
    getPickerIdsOfType('mode').forEach(id => updatePickerTrigger(id));
}

// called when an ai set bg arrives from backend
function updateAiSetPickerBgs(bgKey) {
    document.querySelectorAll('.picker-option[data-bg-key="' + bgKey + '"]').forEach(btn => {
        if (!btn.querySelector('.picker-option-bg') && AI_SET_BG_CACHE[bgKey]) {
            const img = document.createElement('img');
            img.className = 'picker-option-bg';
            img.src = AI_SET_BG_CACHE[bgKey];
            img.alt = '';
            img.draggable = false;
            btn.insertBefore(img, btn.firstChild);
        }
    });
    getPickerIdsOfType('aiset').forEach(id => updatePickerTrigger(id));
}

// display text

function getPickerDisplayText(selectId, option) {
    if (!option) return t('pickers.select_option');
    const reg = PICKER_REGISTRY[selectId];
    // mode pickers: strip the note suffix
    if (reg && reg.type === 'mode') return option.textContent.split(' - ')[0];
    return option.textContent;
}

// trigger (the visible button showing current selection)

function updatePickerTrigger(selectId) {
    const select = document.getElementById(selectId);
    const triggerText = document.getElementById(selectId + 'TriggerText');
    if (!select || !triggerText) return;

    const reg = PICKER_REGISTRY[selectId];
    const selectedOption = select.options[select.selectedIndex];
    triggerText.textContent = getPickerDisplayText(selectId, selectedOption);

    // game icon in trigger
    var existingIcon = triggerText.parentElement && triggerText.parentElement.querySelector('.picker-trigger-icon');
    if (reg && reg.type === 'game' && typeof getGameIconSrc === 'function') {
        var iconSrc = getGameIconSrc(selectedOption ? selectedOption.value : '');
        if (iconSrc) {
            if (!existingIcon) {
                existingIcon = document.createElement('img');
                existingIcon.className = 'picker-trigger-icon';
                existingIcon.alt = '';
                existingIcon.draggable = false;
                triggerText.parentElement.insertBefore(existingIcon, triggerText);
            }
            existingIcon.src = iconSrc;
        } else if (existingIcon) {
            existingIcon.remove();
        }
    } else if (existingIcon) {
        existingIcon.remove();
    }

    const trigger = document.getElementById(selectId + 'Trigger');
    if (!trigger || !selectedOption) return;

    const bgKey = getPickerBgKey(selectId, selectedOption.value);
    const cache = getPickerBgCache(selectId);
    const isMode = reg && reg.type === 'mode';
    let existingBg = trigger.querySelector('.picker-trigger-bg');

    if (bgKey && cache && cache[bgKey]) {
        if (!existingBg) {
            existingBg = document.createElement('img');
            existingBg.className = 'picker-trigger-bg' + (isMode ? ' picker-trigger-bg-mode' : '');
            existingBg.alt = '';
            existingBg.draggable = false;
            trigger.insertBefore(existingBg, trigger.firstChild);
        }
        existingBg.src = cache[bgKey];
        trigger.classList.add('picker-trigger-has-bg');
    } else {
        if (existingBg) existingBg.remove();
        trigger.classList.remove('picker-trigger-has-bg');
        if (bgKey && cache && cache[bgKey] === undefined) {
            cache[bgKey] = null;
            requestPickerBg(selectId, bgKey);
        }
    }
}

// option building

function buildPickerOption(selectId, option) {
    const button = document.createElement('button');
    button.type = 'button';
    button.className = 'picker-option';
    button.setAttribute('data-value', option.value);
    button.setAttribute('data-search-text', option.textContent.toLowerCase());

    const parts = option.textContent.split(' - ');
    let bgHtml = '';

    const bgKey = getPickerBgKey(selectId, option.value);
    const cache = getPickerBgCache(selectId);

    const reg = PICKER_REGISTRY[selectId];
    const isMode = reg && reg.type === 'mode';

    if (bgKey) {
        button.setAttribute('data-bg-key', bgKey);
        if (cache && cache[bgKey]) {
            bgHtml = '<img class="picker-option-bg' + (isMode ? ' picker-option-bg-mode' : '') + '" src="' + cache[bgKey] + '" alt="" draggable="false">';
        } else if (cache && cache[bgKey] === undefined) {
            cache[bgKey] = null;
            requestPickerBg(selectId, bgKey);
        }
        button.classList.add('picker-option-has-bg');
    }

    var iconHtml = '';
    if (reg && reg.type === 'game' && typeof getGameIconSrc === 'function') {
        var iconSrc = getGameIconSrc(option.value);
        if (iconSrc) iconHtml = '<img class="picker-option-icon" src="' + iconSrc + '" alt="" draggable="false">';
    }

    button.innerHTML = bgHtml
        + iconHtml
        + '<span class="picker-option-title">' + escapeHtml(parts[0]) + '</span>'
        + (parts.length > 1 ? '<span class="picker-option-meta">' + escapeHtml(parts.slice(1).join(' - ')) + '</span>' : '');
    button.onclick = function () { selectPickerValue(selectId, option.value); };
    return button;
}

// render / sync / select

function renderPickerOptions(selectId) {
    const select = document.getElementById(selectId);
    const container = document.getElementById(selectId + 'Options');
    if (!select || !container) return;

    container.innerHTML = '';
    Array.from(select.children).forEach(child => {
        if (child.tagName === 'OPTGROUP') {
            const section = document.createElement('div');
            section.className = 'picker-section';

            const label = document.createElement('div');
            label.className = 'picker-section-label';
            label.textContent = child.label;
            section.appendChild(label);

            Array.from(child.children).forEach(option => section.appendChild(buildPickerOption(selectId, option)));
            container.appendChild(section);
            return;
        }

        if (child.tagName === 'OPTION') {
            container.appendChild(buildPickerOption(selectId, child));
        }
    });

    updatePickerTrigger(selectId);
    syncPickerSelection(selectId);
}

function syncPickerSelection(selectId) {
    const select = document.getElementById(selectId);
    if (!select) return;

    document.querySelectorAll('#' + selectId + 'Options .picker-option').forEach(button => {
        button.classList.toggle('active', button.getAttribute('data-value') === select.value);
    });
}

function selectPickerValue(selectId, value) {
    const select = document.getElementById(selectId);
    if (!select) return;

    select.value = value;
    updatePickerTrigger(selectId);
    syncPickerSelection(selectId);
    closePicker(selectId);

    const reg = PICKER_REGISTRY[selectId];
    if (reg && reg.onChange) reg.onChange();
}

// toggle / close / filter

function togglePicker(selectId) {
    const panel = document.getElementById(selectId + 'Panel');
    if (!panel) return;

    const willOpen = !panel.classList.contains('open');
    document.querySelectorAll('.smart-picker-panel.open').forEach(openPanel => openPanel.classList.remove('open'));
    if (willOpen) {
        panel.classList.add('open');
        const search = document.getElementById(selectId + 'Search');
        if (search) {
            search.value = '';
            filterPickerOptions(selectId);
            search.focus();
        }
    }
}

function closePicker(selectId) {
    const panel = document.getElementById(selectId + 'Panel');
    if (panel) panel.classList.remove('open');
}

function filterPickerOptions(selectId) {
    const search = document.getElementById(selectId + 'Search');
    const query = (search?.value || '').trim().toLowerCase();

    document.querySelectorAll('#' + selectId + 'Options .picker-option').forEach(button => {
        const haystack = button.getAttribute('data-search-text') || '';
        button.style.display = !query || haystack.includes(query) ? '' : 'none';
    });

    document.querySelectorAll('#' + selectId + 'Options .picker-section').forEach(section => {
        const hasVisibleOptions = Array.from(section.querySelectorAll('.picker-option')).some(button => button.style.display !== 'none');
        section.style.display = hasVisibleOptions ? '' : 'none';
    });
}

// segmented controls (tod, HostedMode, etc.)

function syncSegmentedGroup(selectId) {
    const group = document.querySelector('.segmented-control[data-select-target="' + selectId + '"]');
    const select = document.getElementById(selectId);
    if (!group || !select) return;

    group.querySelectorAll('.segmented-btn').forEach(button => {
        const active = button.getAttribute('data-value') === select.value;
        button.classList.toggle('active', active);
        button.setAttribute('aria-pressed', active ? 'true' : 'false');
    });
}

function setSegmentedValue(selectId, value, button) {
    const select = document.getElementById(selectId);
    if (!select) return;

    select.value = value;
    syncSegmentedGroup(selectId);

    if (button) button.blur();

    onInclusionChanged();
}
