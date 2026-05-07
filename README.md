# Cypress

Dedicated servers for Plants vs. Zombies: Garden Warfare 1, Garden Warfare 2, and Battle for Neighborville.

Monorepo containing the desktop launcher, server DLL, and backend tools. Server DLL based on [KYBER](https://github.com/ArmchairDevelopers/Kyber) (Star Wars: Battlefront II private servers).

## Features

- **Join** servers by IP or through the server browser
- **Host** dedicated servers with up to **48+ players** (vs 24 stock)
- **Server Browser** with master server registration and heartbeats
- **Playlist Editor** for custom level rotations
- **Relay Support** for NAT traversal without port forwarding
- **Moderator Panel** for managing players, kicks, and bans
- **MOTD Editor** with rich text formatting and color gradients
- **Anticheat** server-side cheat detection modules
- **Multi-instance** management, run multiple servers/clients from one launcher
- **Side-channel** TCP protocol with HMAC challenge-response auth
- **EA Auth-based bans**
- **Headless thin-client** mode (server without rendering)

## Supported Games

| Game | Version | Server DLL |
|------|---------|-----------|
| Garden Warfare 1 | v1.0.3.0 | `cypress_GW1.dll` |
| Garden Warfare 2 | v1.0.12 (PreEAAC) | `cypress_GW2.dll` |
| Battle for Neighborville | 1.0.55.50001 (PreEAAC) | `cypress_BFN.dll` |

## Prerequisites

- Windows 10+ / Ubuntu 24.04+
- .NET 8.0 Runtime
- A legally owned copy of the game

## Building

**Requirements:** Visual Studio 2022+ with both the .NET desktop and C++ desktop development workloads.

```sh
# Server
cd Server
cmake --fresh -S . -B build -DCYPRESS_GW2=ON
cmake --build build --config Release
# Launcher
cd Launcher
dotnet publish CypressLauncher.csproj -c Release -f net8.0-windows -o build /p:LangVersion=latest
```

See [the workflows](./.github/workflows) for more details.

### Server Build Configurations

Each game has its own configuration via preprocessor defines (`CYPRESS_GW1`, `CYPRESS_GW2`, `CYPRESS_BFN`):

| Configuration | Output |
|--------------|--------|
| `Release - GW1 \| x64` | `cypress_GW1.dll` |
| `Release - GW2 \| x64` | `cypress_GW2.dll` |
| `Release - BFN \| x64` | `cypress_BFN.dll` |

## How It Works

1. Server DLL is placed in the game directory as `dinput8.dll` (DirectInput hijack)
2. Game loads the DLL, which installs [MinHook](https://github.com/tsudakageyu/minhook) hooks into Frostbite engine functions
3. Hooks reimplement dedicated server functionality (player connections, level loading, console commands)
4. A side-channel TCP protocol provides real-time events to the launcher

## Project Structure

```
Server/
  Source/
    Examples/               # launch script templates
    Core/
      Program.h/cpp         # entry point, DLL lifecycle, stdin command reader
      Server.h/cpp          # server management, player tracking, status UI
      Client.h/cpp          # client state, side-channel TCP, HWID generation
      Logging.h/cpp         # JSON + colored console logging
      Console/              # game-specific console commands
    GameHooks/
      fbMainHooks.*         # core engine hooks (init, main, console)
      fbServerHooks.*       # server lifecycle hooks (start, update, player join/leave)
      fbClientHooks.*       # client state hooks
      fbEnginePeerHooks.*   # Kyber socket manager integration
    GameModules/
      GW1Module.cpp         # GW1-specific patches and hooks
      GW2Module.cpp         # GW2-specific patches and hooks
      BFNModule.cpp         # BFN-specific patches (Lua console, thin-client UI)
    Anticheat/              # server-side cheat detection (GW2)
  include/
    SideChannel.h/cpp       # side-channel TCP server/client/tunnel
    ServerBanlist.h         # ban system with hardware fingerprinting
    ServerPlaylist.h        # playlist rotation logic
    Kyber/                  # socket management (from KYBER project)
    MinHook/                # runtime function hooking
    fb/                     # reverse-engineered Frostbite engine types
Launcher/
  Assets/                   # Assets (images, fonts) used in the launcher
  CypressLauncher/          # C# backend (Photino.NET, .NET 8)
  wwwroot/                  # Frontend (HTML/CSS/JS)
  Server/                   # C++ server DLL injected into the game
  tools/                    # rceedit
  Docs/                     # Hosting/joining/playlist guides
tools/
  cypress-servers/          # Master server, relay server (Go)
```

## Credits
<table>
<tr>
<td align="center" width="50%">
  <a href="https://github.com/ArmchairDevelopers/Kyber">
    <img src="https://github.com/ArmchairDevelopers.png" width="60"><br>
    <b>KYBER</b>
  </a><br>
  Frostbite socket manager reimplementation
</td>

<td align="center" width="50%">
  <a href="https://github.com/Andersson799">
    <img src="https://github.com/Andersson799.png" width="60"><br>
    <b>Andersson799</b>
  </a><br>
  Frostbite dedicated server reverse engineering
</td>
</tr>

<tr>
<td align="center" width="50%">
  <a href="https://github.com/breakfastbrainz2">
    <img src="https://github.com/breakfastbrainz2.png" width="60"><br>
    <b>BreakfastBrainz2</b>
  </a><br>
  GW1 & GW2 dedicated servers, original launcher
</td>

<td align="center" width="50%">
  <a href="https://github.com/ghdrago">
    <img src="https://github.com/ghdrago.png" width="60"><br>
    <b>Ghup</b>
  </a><br>
  BFN dedicated servers
</td>
</tr>

<tr>
<td align="center" width="50%">
  <a href="https://github.com/dylannws">
    <img src="https://github.com/dylannws.png" width="60"><br>
    <b>Dylan</b>
  </a><br>
  GW2 anticheat
</td>

<td align="center" width="50%">
  <a href="https://github.com/dotthefox">
    <img src="https://github.com/dotthefox.png" width="60"><br>
    <b>Gargos69Junior</b>
  </a><br>
  Continuation of the launcher
</td>
</tr>

<tr>
<td align="center" width="50%">
  <a href="https://github.com/v0ee">
    <img src="https://github.com/v0ee.png" width="60"><br>
    <b>v0e</b>
  </a><br>
  Launcher revamp, side-channel, relay tunnel
</td>

<td align="center" width="50%">
  <a href="https://www.youtube.com/@raymondthejester/">
    <img src="https://yt3.googleusercontent.com/cHv9bXD3143NfpmJV3KxNhXqymhxcrwtQxzu0d-dWloxXROc06Jp77qaa9wX6fm3AS_XWdjzVQ=s160-c-k-c0x00ffffff-no-rj" width="60"><br>
    <b>RaymondTheJester</b>
  </a><br>
  Logo
</td>
</tr>
</table>

## License

[GPL-3.0](LICENSE)

## Terms of Service
[Terms Of Service](TOS.md)
