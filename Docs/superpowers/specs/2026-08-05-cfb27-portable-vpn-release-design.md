# CFB27 Portable VPN Release Design

## Goal

Produce a versioned Windows release that lets a dedicated host run the current
CFB27 private-server backend while players run legally owned copies of CFB27 on
their own PCs and connect through a private VPN such as Tailscale or ZeroTier.

## Release topology

The release contains two ZIP archives.

`Cypress-CFB27-Server` runs on the dedicated Windows host. It contains the
Dynasty and Blaze executables, a portable Node.js runtime, production-only npm
dependencies, the franchise and asset tools, the required extracted Dynasty
schema/assets, configuration, lifecycle scripts, health checks, and operating
instructions. The host does not need CFB27, Go, Node.js, npm, Visual Studio, or
the .NET SDK installed.

`Cypress-CFB27-Client` runs on each player's Windows PC. It contains the
self-contained Cypress launcher, the CFB27 bridge DLL, endpoint data, client
configuration and install/uninstall helpers, compatibility metadata, and player
instructions. The player supplies a legitimate CFB27 installation.

The Blaze listener binds to a configured private-VPN address. Dynasty and
diagnostics listeners remain on loopback. Windows Firewall setup limits the
Blaze port to the VPN subnet or an administrator-provided remote-address list.

## Data ownership

The server first uses title data supplied by a compatible game extraction when
such an extractor becomes available. The current repository cannot extract the
required FTC/FTX files from CFB27's packed archives, so this release includes the
known-good 46 MB `Dynasty_Assets` set as the necessary fallback.

The release does not contain a user's personal Dynasty save. On first setup the
host administrator supplies an `FBCHUNKS` `DYNASTY*` save as the seed. Runtime
databases, generated coach catalogs, mutable Dynasty files, and logs live under
the server's `data` directory and are not overwritten by upgrades.

## Game update compatibility

Client setup and launch calculate SHA-256 for `CollegeFB27.exe` or
`CollegeFB27_Trial.exe` and compare it with a release compatibility manifest.
Known builds may install and launch. An unknown build is rejected before the DLL
is installed or the game starts and produces a small diagnostic report with the
file name, size, version, and SHA-256.

Compatibility data is kept outside the binaries so a later release can add a
profile without changing the server package. Unknown future executable layouts
cannot be supported automatically or promised in advance; they require a
validated compatibility profile and potentially a rebuilt bridge DLL. This
design survives updates safely and makes compatibility servicing incremental.

## Components

The server package contains:

- `server/bin/dynasty.exe` and `server/bin/cfb27blaze.exe`.
- `server/runtime/node.exe` and its required runtime files.
- `server/tools` with the franchise and catalog exporter modules.
- `server/node_modules` with production dependencies and their license data.
- `server/assets/Dynasty_Assets` with numbered asset slots.
- `server/config/server.example.json` and a generated local `server.json`.
- `Setup-Server.ps1`, `Start-Server.ps1`, `Stop-Server.ps1`, and
  `Test-Server.ps1`.

The client package contains:

- A self-contained `CypressLauncher.exe` publication and its application files.
- `cypress_CFB27.dll`, endpoint metadata, and build compatibility metadata.
- `Setup-Client.ps1`, `Start-Client.ps1`, and `Uninstall-Client.ps1`.

Both archives contain `VERSION.txt`, `manifest.json`, `SHA256SUMS.txt`, license
notices, and a focused README.

## Configuration and lifecycle

Server setup asks for or accepts parameters for the VPN bind address, Blaze
port, profile name, Dynasty seed, asset slot, and allowed VPN subnet. It validates
all required files, copies the seed into mutable server data, exports the coach
catalog, writes local configuration, and installs a narrowly scoped firewall
rule when run elevated.

Server start creates a timestamped run directory, starts Dynasty, waits for its
HTTP health endpoint, starts Blaze, waits for diagnostics health, writes a PID
file, and reports the address players should use. Stop reads only that PID file
and stops those exact processes. Test performs static validation and live health
checks without launching the game.

Client setup locates or accepts the game directory, validates the executable
against the compatibility manifest, writes the VPN endpoint configuration, and
backs up an existing `dinput8.dll` before installing Cypress's bridge. Uninstall
restores that backup when it belongs to the same setup operation.

## Failure handling

Scripts use terminating PowerShell errors and emit actionable messages. They do
not silently download dependencies. Setup stops on an unknown game build,
invalid Dynasty seed, missing asset slot, occupied port, inaccessible VPN bind
address, or failed health check. Runtime logs avoid secrets and remain beneath
the package's mutable data directory.

## Build and verification

The release build runs Go and Node tests, CFB27 bridge tests, and launcher tests;
builds fresh release binaries; publishes the launcher self-contained for
`win-x64`; assembles both packages from an explicit allow-list; and writes
manifests, checksums, and ZIP archives.

Verification extracts both ZIPs into fresh temporary directories. Server scripts
are syntax-checked and exercised with temporary data and loopback ports through
health startup/shutdown. Client scripts are syntax-checked and their compatibility
and non-destructive install behavior are tested against fixture game directories.
No verification step launches CFB27.
