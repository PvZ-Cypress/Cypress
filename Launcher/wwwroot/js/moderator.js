// cypress global moderator panel

var modLoggedIn = false;
var modUsername = '';

function switchModPanel(panel) {
    var bansDiv = document.getElementById('modPanelBans');
    var serversDiv = document.getElementById('modPanelServers');
    var bansBtn = document.getElementById('modPanelBansBtn');
    var serversBtn = document.getElementById('modPanelServersBtn');

    if (panel === 'bans') {
        bansDiv.style.display = '';
        serversDiv.style.display = 'none';
        bansBtn.className = 'btn btn-sm btn-primary';
        serversBtn.className = 'btn btn-sm btn-secondary';
        send('modGetGlobalBans', {});
    } else {
        bansDiv.style.display = 'none';
        serversDiv.style.display = '';
        bansBtn.className = 'btn btn-sm btn-secondary';
        serversBtn.className = 'btn btn-sm btn-primary';
        send('modGetBannedServers', {});
    }
}

function modLogin() {
    send('modLogin', {});
}

function modLogout() {
    send('modLogout', {});
}

function onModLoginResult(data) {
    if (data.ok) {
        modLoggedIn = true;
        modUsername = data.username || '';
        document.getElementById('modAuthSection').style.display = 'none';
        document.getElementById('modPanel').style.display = '';
        document.getElementById('modUsernameDisplay').textContent = modUsername;
        document.getElementById('modAuthStatus').textContent = t('moderator.logged_in_as', { name: modUsername });
        document.getElementById('modLoginError').style.display = 'none';
        // refresh mod tabs on any selected client instance (global mod override)
        if (typeof refreshModTabVisibility === 'function') refreshModTabVisibility();
        // load bans
        send('modGetGlobalBans', {});
    } else {
        var err = document.getElementById('modLoginError');
        err.textContent = data.error || t('moderator.login_failed');
        err.style.display = '';
    }
}

function onModLogoutResult() {
    modLoggedIn = false;
    modUsername = '';
    document.getElementById('modAuthSection').style.display = '';
    document.getElementById('modPanel').style.display = 'none';
    document.getElementById('modAuthStatus').textContent = t('moderator.not_logged_in');
    // refresh mod tabs (might lose access if not local mod)
    if (typeof refreshModTabVisibility === 'function') refreshModTabVisibility();
}

// global bans
function modAddGlobalBan() {
    var hash = document.getElementById('modBanHwid').value.trim();
    var reason = document.getElementById('modBanReason').value.trim();
    if (!hash) return;
    // treat input as a component hash so viral matching applies
    send('modGlobalBan', { hwid: '', reason: reason, components: [hash] });
}

function modBanByPid() {
    var pid = document.getElementById('modBanPid').value.trim();
    var reason = document.getElementById('modBanPidReason').value.trim();
    if (!pid) return;
    send('modBanByPid', { pid: pid, reason: reason });
}

function onModBanByPidResult(data) {
    if (data.ok) {
        document.getElementById('modBanPid').value = '';
        document.getElementById('modBanPidReason').value = '';
        send('modGetGlobalBans', {});
    }
}

function onModGlobalBanResult(data) {
    if (data.ok) {
        document.getElementById('modBanHwid').value = '';
        document.getElementById('modBanReason').value = '';
        send('modGetGlobalBans', {});
    }
}

function onModGlobalBansList(data) {
    var container = document.getElementById('modGlobalBansList');
    var bans = data.bans || [];
    if (!bans.length) {
        container.innerHTML = '<p class="text-muted">' + t('moderator.no_bans') + '</p>';
        return;
    }
    var html = '<table class="mod-table"><thead><tr><th>' + t('moderator.table_ea_pid') + '</th><th>' + t('moderator.table_hwid') + '</th><th>' + t('moderator.table_reason') + '</th><th>' + t('moderator.table_banned_by') + '</th><th>' + t('moderator.table_date') + '</th><th></th></tr></thead><tbody>';
    for (var i = 0; i < bans.length; i++) {
        var b = bans[i];
        var date = new Date(b.created_at * 1000).toLocaleDateString();
        var shortHwid = b.hwid ? (b.hwid.length > 16 ? b.hwid.substring(0, 16) + '...' : b.hwid) : '-';
        var shortPid = b.ea_pid || '-';
        html += '<tr>';
        html += '<td><code>' + escapeHtml(shortPid) + '</code></td>';
        html += '<td title="' + escapeAttr(b.hwid || '') + '"><code>' + escapeHtml(shortHwid) + '</code></td>';
        html += '<td>' + escapeHtml(b.reason || '-') + '</td>';
        html += '<td>' + escapeHtml(b.banned_by) + '</td>';
        html += '<td>' + escapeHtml(date) + '</td>';
        html += '<td><button class="btn btn-sm btn-danger" onclick="modRemoveGlobalBan(' + b.id + ')">' + t('moderator.unban') + '</button></td>';
        html += '</tr>';
    }
    html += '</tbody></table>';
    container.innerHTML = html;
}

function modRemoveGlobalBan(id) {
    send('modGlobalUnban', { id: id });
}

function onModGlobalUnbanResult(data) {
    if (data.ok) send('modGetGlobalBans', {});
}

// server bans
function modBanServer() {
    var ip = document.getElementById('modBanServerIp').value.trim();
    var reason = document.getElementById('modBanServerReason').value.trim();
    if (!ip) return;
    send('modBanServer', { ip: ip, reason: reason });
}

function onModBanServerResult(data) {
    if (data.ok) {
        document.getElementById('modBanServerIp').value = '';
        document.getElementById('modBanServerReason').value = '';
        send('modGetBannedServers', {});
    }
}

function onModBannedServersList(data) {
    var container = document.getElementById('modBannedServersList');
    var servers = data.servers || [];
    if (!servers.length) {
        container.innerHTML = '<p class="text-muted">' + t('moderator.no_banned_servers') + '</p>';
        return;
    }
    var html = '<table class="mod-table"><thead><tr><th>' + t('moderator.table_ip') + '</th><th>' + t('moderator.table_reason') + '</th><th>' + t('moderator.table_banned_by') + '</th><th>' + t('moderator.table_date') + '</th><th></th></tr></thead><tbody>';
    for (var i = 0; i < servers.length; i++) {
        var s = servers[i];
        var date = new Date(s.created_at * 1000).toLocaleDateString();
        html += '<tr>';
        html += '<td><code>' + escapeHtml(s.ip) + '</code></td>';
        html += '<td>' + escapeHtml(s.reason || '-') + '</td>';
        html += '<td>' + escapeHtml(s.banned_by) + '</td>';
        html += '<td>' + escapeHtml(date) + '</td>';
        html += '<td><button class="btn btn-sm btn-danger" onclick="modUnbanServer(\'' + escapeAttr(s.ip) + '\')">' + t('moderator.unban') + '</button></td>';
        html += '</tr>';
    }
    html += '</tbody></table>';
    container.innerHTML = html;
}

function modUnbanServer(ip) {
    send('modUnbanServer', { ip: ip });
}

function onModUnbanServerResult(data) {
    if (data.ok) send('modGetBannedServers', {});
}
