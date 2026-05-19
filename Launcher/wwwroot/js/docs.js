function switchDoc(doc) {
    document.querySelectorAll('.docs-nav-btn').forEach(el => el.classList.remove('active'));
    const btn = document.querySelector('[data-doc="' + doc + '"]');
    if (btn) btn.classList.add('active');
    document.getElementById('docsContent').innerHTML = getDocContent(doc);
}

function getDocContent(doc) {
    switch (doc) {
        case 'quickstart': return docQuickStart();
        case 'joining': return docJoining();
        case 'hosting': return docHosting();
        case 'server-commands': return docServerCommands();
        case 'levels': return docLevels();
        case 'modifiers': return docModifiers();
        case 'playlists': return docPlaylists();
        default: return '';
    }
}

function docQuickStart() {
    return '<div class="docs-section active"><h3>' + t('docs.quickstart_title') + '</h3><div class="steps">'
        + step(1, t('docs.qs_step1_title'), t('docs.qs_step1_text'))
        + step(2, t('docs.qs_step2_title'), t('docs.qs_step2_text'))
        + step(3, t('docs.qs_step3_title'), t('docs.qs_step3_text'))
        + step(4, t('docs.qs_step4_title'), t('docs.qs_step4_text'))
        + step(5, t('docs.qs_step5_title'), t('docs.qs_step5_text'))
        + step(6, t('docs.qs_step6_title'), t('docs.qs_step6_text'))
        + '</div></div>';
}

function docJoining() {
    return '<div class="docs-section active"><h3>' + t('docs.joining_title') + '</h3>'
        + '<div class="info-card"><h5>' + t('docs.join_browser_title') + '</h5><div class="steps compact">'
        + step(1, '', t('docs.join_browser_step1'))
        + step(2, '', t('docs.join_browser_step2'))
        + step(3, '', t('docs.join_browser_step3'))
        + '</div></div>'
        + '<div class="info-card" style="margin-top:12px;"><h5>' + t('docs.join_direct_title') + '</h5><div class="steps compact">'
        + step(1, '', t('docs.join_direct_step1'))
        + step(2, '', t('docs.join_direct_step2'))
        + step(3, '', t('docs.join_direct_step3'))
        + step(4, '', t('docs.join_direct_step4'))
        + '</div></div>'
        + '<div class="info-card" style="margin-top:12px;"><h5>' + t('docs.join_relay_title') + '</h5><p style="margin-bottom:8px;font-size:12px;color:var(--text-muted);">' + t('docs.join_relay_intro') + '</p><div class="steps compact">'
        + step(1, '', t('docs.join_relay_step1'))
        + step(2, '', t('docs.join_relay_step2'))
        + step(3, '', t('docs.join_relay_step3'))
        + '</div></div>'
        + '<div class="info-card" style="margin-top:12px;"><h5>' + t('docs.join_vpn_title') + '</h5><p style="margin-bottom:8px;font-size:12px;color:var(--text-muted);">' + t('docs.join_vpn_intro') + '</p><div class="steps compact">'
        + step(1, '', t('docs.join_vpn_step1'))
        + step(2, '', t('docs.join_vpn_step2'))
        + '</div></div></div>';
}

function docHosting() {
    return '<div class="docs-section active"><h3>' + t('docs.hosting_title') + '</h3>'
        + '<div class="info-card"><h5>' + t('docs.host_relay_title') + '</h5><p style="margin-bottom:8px;font-size:12px;color:var(--text-muted);">' + t('docs.host_relay_intro') + '</p><div class="steps compact">'
        + step(1, '', t('docs.host_relay_step1'))
        + step(2, '', t('docs.host_relay_step2'))
        + step(3, '', t('docs.host_relay_step3'))
        + step(4, '', t('docs.host_relay_step4'))
        + '</div></div>'
        + '<div class="info-card" style="margin-top:12px;"><h5>' + t('docs.host_portfwd_title') + '</h5><div class="steps compact">'
        + step(1, '', t('docs.host_portfwd_step1'))
        + step(2, '', t('docs.host_portfwd_step2'))
        + step(3, '', t('docs.host_portfwd_step3'))
        + step(4, '', t('docs.host_portfwd_step4'))
        + step(5, '', t('docs.host_portfwd_step5'))
        + '</div><p style="margin-top:8px;font-size:12px;color:var(--text-muted);">' + t('docs.host_portfwd_note') + '</p></div>'
        + '<div class="info-card" style="margin-top:12px;"><h5>' + t('docs.host_radmin_title') + '</h5><p style="margin-bottom:8px;font-size:12px;color:var(--text-muted);">' + t('docs.host_radmin_intro') + '</p><div class="steps compact">'
        + step(1, '', t('docs.host_radmin_step1'))
        + step(2, '', t('docs.host_radmin_step2'))
        + step(3, '', t('docs.host_radmin_step3'))
        + step(4, '', t('docs.host_radmin_step4'))
        + '</div></div>'
        + '<div class="info-card" style="margin-top:12px;"><h5>' + t('docs.host_settings_title') + '</h5>'
        + '<table class="ref-table"><tr><th>' + t('docs.table_setting') + '</th><th>' + t('docs.table_description') + '</th></tr>'
        + '<tr><td><strong>Listed in Browser</strong></td><td>' + t('docs.host_setting_listed') + '</td></tr>'
        + '<tr><td><strong>GCBDB</strong></td><td>' + t('docs.host_setting_gcbdb') + '</td></tr>'
        + '<tr><td><strong>Block ID_ Usernames</strong></td><td>' + t('docs.host_setting_blockid') + '</td></tr>'
        + '<tr><td><strong>Max Players</strong></td><td>' + t('docs.host_setting_maxplayers') + '</td></tr>'
        + '<tr><td><strong>Server Password</strong></td><td>' + t('docs.host_setting_password') + '</td></tr>'
        + '<tr><td><strong>MOTD</strong></td><td>' + t('docs.host_setting_motd') + '</td></tr>'
        + '<tr><td><strong>Server Icon</strong></td><td>' + t('docs.host_setting_icon') + '</td></tr>'
        + '<tr><td><strong>Anticheat</strong></td><td>' + t('docs.host_setting_anticheat') + '</td></tr>'
        + '</table></div></div>';
}

function docServerCommands() {
    return '<div class="docs-section active"><h3>' + t('docs.commands_title') + '</h3>'
        + '<p>' + t('docs.commands_intro') + '</p>'
        + '<h4>' + t('docs.commands_all_games') + '</h4>'
        + '<table class="ref-table"><tr><th>' + t('docs.table_command') + '</th><th>' + t('docs.table_description') + '</th><th>' + t('docs.table_example') + '</th></tr>'
        + cmdRow('Server.RestartLevel', t('docs.cmd_restartlevel'), 'Server.RestartLevel')
        + cmdRow('Server.LoadLevel', t('docs.cmd_loadlevel'), 'Server.LoadLevel Level_FE_Hub GameMode=FreeRoam;TOD=Day;HostedMode=ServerHosted')
        + cmdRow('Server.KickPlayer', t('docs.cmd_kickplayer'), 'Server.KickPlayer Jim')
        + cmdRow('Server.KickPlayerById', t('docs.cmd_kickplayerbyid'), 'Server.KickPlayerById 4')
        + cmdRow('Server.BanPlayer', t('docs.cmd_banplayer'), 'Server.BanPlayer Jim')
        + cmdRow('Server.BanPlayerById', t('docs.cmd_banplayerbyid'), 'Server.BanPlayerById 4')
        + cmdRow('Server.UnbanPlayer', t('docs.cmd_unbanplayer'), 'Server.UnbanPlayer Jim')
        + cmdRow('Server.LoadNextPlaylistSetup', t('docs.cmd_loadnextplaylist'), 'Server.LoadNextPlaylistSetup')
        + '</table>'
        + '<h4>' + t('docs.commands_gw2_only') + '</h4>'
        + '<table class="ref-table"><tr><th>' + t('docs.table_command') + '</th><th>' + t('docs.table_description') + '</th><th>' + t('docs.table_example') + '</th></tr>'
        + cmdRow('Server.Say', t('docs.cmd_say'), "Server.Say 'Hello World!' 5")
        + cmdRow('Server.SayToPlayer', t('docs.cmd_saytoplayer'), "Server.SayToPlayer 'Jim' 'Hello!' 5")
        + '</table>'
        + '<div class="info-card" style="margin-top:12px;"><h5>' + t('docs.commands_runtime_title') + '</h5><p>' + t('docs.commands_runtime_text') + '</p></div>'
        + '</div>';
}

function cmdRow(cmd, desc, ex) {
    return '<tr><td><code>' + cmd + '</code></td><td>' + desc + '</td><td><code>' + ex + '</code></td></tr>';
}

function step(n, title, text) {
    return '<div class="step"><span class="step-num">' + n + '</span><div class="step-content">' + (title ? '<h4>' + title + '</h4>' : '') + '<p>' + text + '</p></div></div>';
}

function docLevels() {
    let html = '<div class="docs-section active"><h3>' + t('docs.levels_title') + '</h3>'
        + '<p>' + t('docs.levels_intro') + '</p>'
        + '<div class="game-filter">'
        + '<button class="game-filter-btn active" onclick="filterDoc(\'lvl\',\'gw1\',this)">GW1</button>'
        + '<button class="game-filter-btn" onclick="filterDoc(\'lvl\',\'gw2\',this)">GW2</button>'
        + '<button class="game-filter-btn" onclick="filterDoc(\'lvl\',\'bfn\',this)">BFN</button>'
        + '</div>';

    html += '<div id="lvl-gw1" class="doc-filter-section">'
        + '<div class="info-card"><h5>' + t('docs.levels_gw1_format_title') + '</h5><p>' + t('docs.levels_gw1_format_text') + '</p><pre>Level = _pvz/Levels/Mainstreet/Level_COOP_Mainstreet/Level_COOP_Mainstreet\nInclusion = GameMode=Coop0;TOD=Day</pre></div>'
        + '<h4>' + t('docs.table_levels') + '</h4><table class="ref-table"><tr><th>' + t('docs.table_level_path') + '</th><th>' + t('docs.table_name') + '</th><th>' + t('docs.table_category') + '</th><th>' + t('docs.table_variants') + '</th><th>' + t('docs.table_night') + '</th></tr>';
    GAME_DATA.GW1.levels.forEach(l => {
        html += '<tr><td><code>' + l.id + '</code></td><td>' + l.name + '</td><td>' + l.cat + '</td><td>' + (l.variant||'0') + '</td><td>' + (l.night ? t('common.yes') : t('common.no')) + '</td></tr>';
    });
    html += '</table><h4>' + t('docs.table_modes') + '</h4><table class="ref-table"><tr><th>' + t('docs.table_mode_id') + '</th><th>' + t('docs.table_name') + '</th><th>' + t('docs.table_notes') + '</th></tr>';
    GAME_DATA.GW1.modes.forEach(m => {
        html += '<tr><td><code>' + m.id + '</code></td><td>' + m.name + '</td><td>' + (m.note || t('docs.levels_gw1_mode_note_default')) + '</td></tr>';
    });
    html += '</table></div>';

    html += '<div id="lvl-gw2" class="doc-filter-section" style="display:none;">'
        + '<div class="info-card"><h5>' + t('docs.levels_gw2_format_title') + '</h5><p>' + t('docs.levels_gw2_format_text') + '</p><pre>Level = Level_FE_Hub\nInclusion = GameMode=FreeRoam;TOD=Day;HostedMode=ServerHosted</pre>'
        + '<p style="margin-top:6px;">' + t('docs.levels_gw2_hosted_desc') + '</p></div>'
        + '<h4>' + t('docs.table_levels') + '</h4><table class="ref-table"><tr><th>' + t('docs.table_level_id') + '</th><th>' + t('docs.table_name') + '</th><th>' + t('docs.table_category') + '</th><th>' + t('docs.table_supported_modes') + '</th></tr>';
    GAME_DATA.GW2.levels.forEach(l => {
        const modeNames = (l.modes||[]).map(mid => { const m = GAME_DATA.GW2.modes.find(x => x.id === mid); return m ? m.name : mid; }).join(', ');
        html += '<tr><td><code>' + l.id + '</code></td><td>' + l.name + '</td><td>' + l.cat + '</td><td style="font-size:11px;">' + modeNames + '</td></tr>';
    });
    html += '</table><h4>' + t('docs.table_modes') + '</h4><table class="ref-table"><tr><th>' + t('docs.table_mode_id') + '</th><th>' + t('docs.table_name') + '</th><th>' + t('docs.table_category') + '</th></tr>';
    GAME_DATA.GW2.modes.forEach(m => {
        html += '<tr><td><code>' + m.id + '</code></td><td>' + m.name + '</td><td>' + m.cat + '</td></tr>';
    });
    html += '</table></div>';

    html += '<div id="lvl-bfn" class="doc-filter-section" style="display:none;">'
        + '<div class="info-card"><h5>' + t('docs.levels_bfn_format_title') + '</h5><p>' + t('docs.levels_bfn_format_text') + '</p><pre>DSub = DSub_SocialSpace\nInclusion = GameMode=Mode_SocialSpace\nStartPoint = StartPoint_SocialSpace</pre></div>'
        + '<h4>' + t('docs.table_dsubs') + '</h4><table class="ref-table"><tr><th>' + t('docs.table_dsub_id') + '</th><th>' + t('docs.table_name') + '</th><th>' + t('docs.table_category') + '</th></tr>';
    GAME_DATA.BFN.levels.forEach(l => {
        html += '<tr><td><code>' + l.id + '</code></td><td>' + l.name + '</td><td>' + l.cat + '</td></tr>';
    });
    html += '</table><h4>' + t('docs.table_modes') + '</h4><table class="ref-table"><tr><th>' + t('docs.table_mode_id') + '</th><th>' + t('docs.table_name') + '</th><th>' + t('docs.table_category') + '</th></tr>';
    GAME_DATA.BFN.modes.forEach(m => {
        html += '<tr><td><code>' + m.id + '</code></td><td>' + m.name + '</td><td>' + m.cat + '</td></tr>';
    });
    html += '</table><h4>' + t('docs.table_start_points') + '</h4><table class="ref-table"><tr><th>' + t('docs.table_startpoint_id') + '</th><th>' + t('docs.table_mode_name') + '</th></tr>';
    GAME_DATA.BFN.startPoints.forEach(s => {
        html += '<tr><td><code>' + s.id + '</code></td><td>' + s.name + '</td></tr>';
    });
    html += '</table></div>';

    html += '</div>';
    return html;
}

function docModifiers() {
    let html = '<div class="docs-section active"><h3>' + t('docs.modifiers_title') + '</h3>'
        + '<p>' + t('docs.modifiers_intro') + '</p>'
        + '<pre>-GameMode.CrazyOption2 true -GameMode.CrazyOption5 true -GameMode.StoredDifficultyIndex 4</pre>'
        + '<div class="game-filter">'
        + '<button class="game-filter-btn active" onclick="filterDoc(\'mod\',\'gw1\',this)">GW1</button>'
        + '<button class="game-filter-btn" onclick="filterDoc(\'mod\',\'gw2\',this)">GW2</button>'
        + '<button class="game-filter-btn" onclick="filterDoc(\'mod\',\'bfn\',this)">BFN</button>'
        + '</div>';

    html += '<div id="mod-gw1" class="doc-filter-section"><table class="ref-table"><tr><th>' + t('docs.table_setting') + '</th><th>' + t('docs.table_name') + '</th><th>' + t('docs.table_description') + '</th><th>' + t('docs.table_type') + '</th></tr>';
    GAME_DATA.GW1.modifierCategories.forEach(cat => {
        if (cat.mods) cat.mods.forEach(m => {
            html += '<tr><td><code>' + m.key + '</code></td><td>' + m.name + '</td><td>' + m.desc + '</td><td>' + m.type + '</td></tr>';
        });
    });
    html += '</table></div>';

    html += '<div id="mod-gw2" class="doc-filter-section" style="display:none;">';
    GAME_DATA.GW2.modifierCategories.forEach(cat => {
        if (cat.special === 'gw2_costumes') {
            html += '<h4>' + t('docs.mod_gw2_costumes_title') + '</h4>'
                + '<p>' + t('docs.mod_gw2_costumes_text') + '</p>'
                + '<table class="ref-table"><tr><th>' + t('docs.table_preset_name') + '</th><th>' + t('docs.table_description') + '</th></tr>';
            Object.keys(GW2_COSTUME_PRESETS).forEach(name => {
                html += '<tr><td><strong>' + name + '</strong></td><td style="font-size:11px;word-break:break-all;"><code>' + GW2_COSTUME_PRESETS[name].substring(0,60) + '...</code></td></tr>';
            });
            html += '</table>';
            html += '<h4>' + t('docs.mod_gw2_ai_sets_title') + '</h4><table class="ref-table"><tr><th>' + t('docs.table_set_name') + '</th><th>' + t('docs.table_plant_id') + '</th><th>' + t('docs.table_zombie_id') + '</th></tr>';
            Object.keys(GW2_AI_SETS.Plants).forEach(name => {
                html += '<tr><td>' + name + '</td><td><code>' + GW2_AI_SETS.Plants[name] + '</code></td><td><code>' + (GW2_AI_SETS.Zombies[name]||'None') + '</code></td></tr>';
            });
            html += '</table>';
        } else if (cat.mods) {
            html += '<h4>' + cat.name + '</h4><table class="ref-table"><tr><th>' + t('docs.table_setting') + '</th><th>' + t('docs.table_name') + '</th><th>' + t('docs.table_description') + '</th><th>' + t('docs.table_type') + '</th></tr>';
            cat.mods.forEach(m => {
                html += '<tr><td><code>' + m.key + '</code></td><td>' + m.name + '</td><td>' + m.desc + '</td><td>' + m.type + '</td></tr>';
            });
            html += '</table>';
        }
    });
    html += '</div>';

    html += '<div id="mod-bfn" class="doc-filter-section" style="display:none;">';
    GAME_DATA.BFN.modifierCategories.forEach(cat => {
        if (cat.special === 'bfn_classes') {
            html += '<h4>' + t('docs.mod_bfn_kills_title') + '</h4>'
                + '<p>' + t('docs.mod_bfn_kills_text') + '</p>'
                + '<table class="ref-table"><tr><th>' + t('docs.table_preset') + '</th><th>' + t('docs.table_disabled_classes') + '</th></tr>';
            Object.entries(BFN_KILLSWITCH_PRESETS).forEach(([name, classes]) => {
                const enabled = [...BFN_CLASSES.Plants, ...BFN_CLASSES.Zombies].filter(c => !classes.includes(c.kit)).map(c => c.name);
                html += '<tr><td><strong>' + name + '</strong></td><td style="font-size:11px;">' + t('docs.mod_allows', { classes: enabled.join(', ') }) + '</td></tr>';
            });
            html += '</table>';
            html += '<h4>' + t('docs.mod_bfn_ai_mask_title') + '</h4>'
                + '<p>' + t('docs.mod_bfn_ai_mask_text') + '</p>'
                + '<table class="ref-table"><tr><th>' + t('docs.table_plant_class') + '</th><th>' + t('docs.table_value') + '</th><th>' + t('docs.table_zombie_class') + '</th><th>' + t('docs.table_value') + '</th></tr>';
            const maxLen = Math.max(BFN_CLASSES.Plants.length, BFN_CLASSES.Zombies.length);
            for (let i = 0; i < maxLen; i++) {
                const p = BFN_CLASSES.Plants[i];
                const z = BFN_CLASSES.Zombies[i];
                html += '<tr><td>' + (p?p.name:'') + '</td><td>' + (p?p.mask:'') + '</td><td>' + (z?z.name:'') + '</td><td>' + (z?z.mask:'') + '</td></tr>';
            }
            html += '</table>';
            html += '<h4>' + t('docs.mod_bfn_ai_sets_title') + '</h4><table class="ref-table"><tr><th>' + t('docs.table_set_name') + '</th><th>' + t('docs.table_plant_id') + '</th><th>' + t('docs.table_zombie_id') + '</th></tr>';
            Object.keys(BFN_AI_SETS.Plants).forEach(name => {
                html += '<tr><td>' + name + '</td><td><code>' + BFN_AI_SETS.Plants[name] + '</code></td><td><code>' + (BFN_AI_SETS.Zombies[name]||'None') + '</code></td></tr>';
            });
            html += '</table>';
        } else if (cat.mods) {
            html += '<h4>' + cat.name + '</h4><table class="ref-table"><tr><th>' + t('docs.table_setting') + '</th><th>' + t('docs.table_name') + '</th><th>' + t('docs.table_description') + '</th><th>' + t('docs.table_type') + '</th></tr>';
            cat.mods.forEach(m => {
                html += '<tr><td><code>' + m.key + '</code></td><td>' + m.name + '</td><td>' + m.desc + '</td><td>' + m.type + '</td></tr>';
            });
            html += '</table>';
        }
    });
    html += '</div></div>';
    return html;
}

function docPlaylists() {
    return '<div class="docs-section active"><h3>' + t('docs.playlists_title') + '</h3>'
        + '<div class="info-card"><h5>Setup</h5><p>' + t('docs.playlists_setup_text') + '</p></div>'
        + '<h4>' + t('docs.playlists_global_settings') + '</h4>'
        + '<table class="ref-table"><tr><th>' + t('docs.table_key') + '</th><th>' + t('docs.table_type') + '</th><th>' + t('docs.table_description') + '</th></tr>'
        + '<tr><td><code>RoundsPerSetup</code></td><td>Integer</td><td>' + t('docs.pl_rounds_desc') + '</td></tr>'
        + '<tr><td><code>IsMixed</code></td><td>Boolean</td><td>' + t('docs.pl_ismixed_desc') + '</td></tr>'
        + '<tr><td><code>Loadscreen_GamemodeNameOverride</code></td><td>String</td><td>' + t('docs.pl_ls_gamemode_desc') + '</td></tr>'
        + '<tr><td><code>Loadscreen_LevelNameOverride</code></td><td>String</td><td>' + t('docs.pl_ls_level_desc') + '</td></tr>'
        + '<tr><td><code>Loadscreen_LevelDescriptionOverride</code></td><td>String</td><td>' + t('docs.pl_ls_desc_desc') + '</td></tr>'
        + '<tr><td><code>Loadscreen_UIAssetPathOverride</code></td><td>String</td><td>' + t('docs.pl_ls_asset_desc') + '</td></tr>'
        + '</table>'
        + '<h4>' + t('docs.playlists_rotation_title') + '</h4>'
        + '<p>' + t('docs.playlists_rotation_intro') + '</p>'
        + '<table class="ref-table"><tr><th>' + t('docs.table_key') + '</th><th>' + t('docs.table_required') + '</th><th>' + t('docs.table_description') + '</th></tr>'
        + '<tr><td><code>LevelName</code></td><td>' + t('docs.pl_levelname_req') + '</td><td>' + t('docs.pl_levelname_desc') + '</td></tr>'
        + '<tr><td><code>GameMode</code></td><td>' + t('docs.pl_gamemode_req') + '</td><td>' + t('docs.pl_gamemode_desc') + '</td></tr>'
        + '<tr><td><code>StartPoint</code></td><td>' + t('docs.pl_startpoint_req') + '</td><td>' + t('docs.pl_startpoint_desc') + '</td></tr>'
        + '<tr><td><code>TOD</code></td><td>' + t('docs.pl_tod_req') + '</td><td>' + t('docs.pl_tod_desc') + '</td></tr>'
        + '<tr><td><code>SettingsToApply</code></td><td>' + t('docs.pl_settings_req') + '</td><td>' + t('docs.pl_settings_desc') + '</td></tr>'
        + '<tr><td><code>Loadscreen_*</code></td><td>' + t('docs.pl_loadscreen_req') + '</td><td>' + t('docs.pl_loadscreen_desc') + '</td></tr>'
        + '</table>'
        + '<div class="info-card" style="margin-top:8px;"><h5>' + t('docs.playlists_tip_title') + '</h5><p>' + t('docs.playlists_tip_text') + '</p></div>'
        + '<h4>' + t('docs.playlists_mixed_title') + '</h4>'
        + '<table class="ref-table"><tr><th>' + t('docs.table_key') + '</th><th>' + t('docs.table_description') + '</th></tr>'
        + '<tr><td><code>AvailableModes</code></td><td>' + t('docs.pl_avail_modes_desc') + '</td></tr>'
        + '<tr><td><code>AvailableLevelsForModes</code></td><td>' + t('docs.pl_avail_levels_desc') + '</td></tr>'
        + '<tr><td><code>AvailableTODForLevels</code></td><td>' + t('docs.pl_avail_tod_desc') + '</td></tr>'
        + '</table>'
        + '<h4>' + t('docs.playlists_example_title') + '</h4>'
        + '<pre>{\n  "RoundsPerSetup": 2,\n  "IsMixed": false,\n  "PlaylistRotation": [\n    {\n      "LevelName": "Levels/Level_Rush_Snow/Level_Rush_Snow",\n      "GameMode": "GnGLarge0",\n      "TOD": "Night"\n    },\n    {\n      "LevelName": "Levels/Level_Coop_Egypt/Level_Coop_Egypt",\n      "GameMode": "TeamVanquishLarge0",\n      "SettingsToApply": "GameMode.CrazyOption2 true"\n    }\n  ]\n}</pre>'
        + '</div>';
}

function filterDoc(prefix, key, btn) {
    btn.parentElement.querySelectorAll('.game-filter-btn').forEach(b => b.classList.remove('active'));
    btn.classList.add('active');
    document.querySelectorAll('.doc-filter-section').forEach(s => { if (s.id.startsWith(prefix + '-')) s.style.display = 'none'; });
    document.getElementById(prefix + '-' + key).style.display = '';
}

window.addEventListener('DOMContentLoaded', () => {
    populateLevelPicker(true);
    populateModePicker();
    renderPickerOptions('levelPicker');
