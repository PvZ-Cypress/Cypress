# Cypress CFB27 Private Server (Windows)

This package runs the CFB27 Dynasty and Blaze services on a dedicated Windows
host. The game does not run on this machine. Players connect through a private
VPN such as Tailscale or ZeroTier.

## Requirements

- Windows 10/11 or Windows Server 2019+ x64.
- A private-VPN IPv4 address assigned to the host.
- One offline CFB27 `DYNASTY*` save whose first eight bytes are `FBCHUNKS`.
- PowerShell 5.1 or newer. No SDK, Go, Node.js, or game installation is needed.

## First-time setup

Extract the ZIP to a permanent writable directory. From an elevated PowerShell
window, run:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\Setup-Server.ps1 -VpnBindAddress "100.64.0.5" -VpnRemoteAddress "100.64.0.0/10" -DynastySeed "D:\CFB27\DYNASTY-SEED" -Profile "MyLeague" -InstallFirewallRule
```

Use the host's actual VPN address. For ZeroTier, replace the default remote
range with that network's assigned subnet. Omitting `-InstallFirewallRule`
leaves Windows Firewall unchanged.

## Operations

```powershell
.\Start-Server.ps1
.\Test-Server.ps1 -RequireRunning
.\Stop-Server.ps1
```

Give players the VPN address and Blaze port printed by setup. Do not expose the
Blaze port directly to the public internet.

Back up the complete `data` directory while the server is stopped. Upgrades
must preserve `data` and `config/server.json`. Logs are written beneath `runs`.

If startup fails, inspect the newest `runs/<timestamp>/*.stderr.log` files.
Confirm that the VPN address is assigned to this host and that ports 27910,
27920, and 27921 are unused.
