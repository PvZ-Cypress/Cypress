# Cypress CFB27 Private Client (Windows)

This package connects a legally installed copy of CFB27 to a Cypress host over
Tailscale, ZeroTier, or another trusted private VPN.

## Setup

Join the same VPN as the server and confirm that you can reach its VPN address.
Extract this ZIP, close CFB27 and the EA launcher, then run:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\Setup-Client.ps1 -GameDirectory "C:\Program Files\EA Games\EA SPORTS College Football 27" -ServerAddress "100.64.0.5" -BlazePort 27920 -Profile "PlayerName"
```

Setup hashes the game executable before changing the game directory. If the
build is unknown, it stops without installing the bridge and writes a report to
`%APPDATA%\Cypress\CFB27\Remote\compatibility-reports`. Send that report to the
server maintainer. Game updates can require a new compatibility profile or DLL;
do not bypass this check.

## Play and uninstall

Launch the private configuration with:

```powershell
.\Start-Client.ps1
```

Keep the extracted client package because its scripts and compatibility
manifest are part of the installation. To remove Cypress and restore a prior
`dinput8.dll` safely:

```powershell
.\Uninstall-Client.ps1
```

Uninstall refuses to delete a DLL that changed after Cypress installed it.
