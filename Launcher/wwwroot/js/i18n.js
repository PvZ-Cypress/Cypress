// i18n translation system
// usage: t('key') or t('key', {var: value}) for {var} interpolation
// html: add data-i18n="key" on elements whose textContent should be translated
//       add data-i18n-placeholder="key" for input placeholders
//       add data-i18n-title="key" for title attributes
//       add data-i18n-tip="key" for data-tip tooltip attributes

window._i18nStrings = {};

window.t = function(key, vars) {
    var str = window._i18nStrings[key];
    if (str === undefined) return key;
    if (vars) {
        str = str.replace(/\{(\w+)\}/g, function(_, k) {
            return vars[k] !== undefined ? String(vars[k]) : '{' + k + '}';
        });
    }
    return str;
};

function onTranslationsList(data) {
    var select = document.getElementById('languageSelect');
    if (!select || !data.langs) return;
    var current = localStorage.getItem('cypress_lang') || 'en_us';
    select.innerHTML = '';
    data.langs.forEach(function(lang) {
        var opt = document.createElement('option');
        opt.value = lang;
        opt.textContent = lang;
        if (lang === current) opt.selected = true;
        select.appendChild(opt);
    });
}

function onLanguageChanged(lang) {
    if (!lang) return;
    localStorage.setItem('cypress_lang', lang);
    send('getTranslations', { lang: lang });
}

function applyDomTranslations() {
    document.body.style.visibility = '';
    document.querySelectorAll('[data-i18n]').forEach(function(el) {
        var key = el.getAttribute('data-i18n');
        var val = window._i18nStrings[key];
        if (val !== undefined) {
            el.textContent = val;
            el.dir = 'auto';
        }
    });
    document.querySelectorAll('[data-i18n-placeholder]').forEach(function(el) {
        var key = el.getAttribute('data-i18n-placeholder');
        var val = window._i18nStrings[key];
        if (val !== undefined) el.placeholder = val;
    });
    document.querySelectorAll('[data-i18n-title]').forEach(function(el) {
        var key = el.getAttribute('data-i18n-title');
        var val = window._i18nStrings[key];
        if (val !== undefined) el.title = val;
    });
    document.querySelectorAll('[data-i18n-tip]').forEach(function(el) {
        var key = el.getAttribute('data-i18n-tip');
        var val = window._i18nStrings[key];
        if (val !== undefined) el.setAttribute('data-tip', val);
    });
}
