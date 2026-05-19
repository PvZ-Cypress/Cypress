    renderPickerOptions('modePicker');
    renderPickerOptions('browserFilterGame');
    updatePickerTrigger('browserFilterGame');
    renderPickerOptions('browserSort');
    updatePickerTrigger('browserSort');
    renderPickerOptions('browserFilterModded');
    updatePickerTrigger('browserFilterModded');
    syncPickerCompatibility();
    populateModifierToggles();
    onInclusionChanged();
    syncSegmentedGroup('todPicker');
    syncSegmentedGroup('hostedModePicker');
    syncRelayUi('join');
    syncRelayUi('host');
    syncFloatingFooter('join');
    addPlaylistEntry();
    initMotdEditor();
    buildMotdColorPresets();
    send('getTranslations', { lang: localStorage.getItem('cypress_lang') || 'en_us' });
    send('getTranslationsList', {});
    send('checkTos', {});
    send('checkUpdates', {});
});

document.addEventListener('click', function (event) {
    const anchor = event.target.closest('a.external-link, a[href^="http://"], a[href^="https://"]');
    if (anchor) {
        event.preventDefault();
        send('openExternal', { url: anchor.href });
        return;
    }

    if (!event.target.closest('.smart-picker')) {
        document.querySelectorAll('.smart-picker-panel.open').forEach(panel => panel.classList.remove('open'));
    }
});

document.addEventListener('keydown', function (event) {
    const key = event.key.toLowerCase();
    if ((event.ctrlKey && ['u', 'r'].includes(key)) ||
        event.key === 'F5' ||
        event.key === 'F12' ||
        (event.ctrlKey && event.shiftKey && ['q', 'j', 'c'].includes(key))) {
        event.preventDefault();
        event.stopPropagation();
    }
});
