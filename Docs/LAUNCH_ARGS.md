# Launch args

Here you can find launch arguments with their examples/defaults.

### Universal
```
-name Player
-dataPath ModData/Default
-Client.ServerIp 127.0.0.1:25200
-listen 0.0.0.0:25200
-Server.ServerPassword 123456
-Window.Fullscreen true
-GameTime.MaxSimFps 60
```

### GW1
```
-level _pvz/Levels/Coastal/Level_COOP_Coastal/Level_COOP_Coastal
-Game.DefaultLayerInclusion GameMode=Coop0
-Network.ServerPort 25200
-PerfOverlay.DrawFps false
-PVZServer.InActivityTimeOut 180
-SyncedBFSettings.AllUnlocksUnlocked true
```

### GW2
```
-level Levels/Level_FE_Hub/Level_FE_Hub
-Game.DefaultLayerInclusion GameMode=FreeRoam
-Network.ServerPort 25200
-PerfOverlay.DrawFps false
-PVZServer.InActivityTimeOut 180
-GameMode.SkipIntroHubNIS true
-Online.OnlineGameInteractionMasterKillSwitch true
-Render.FovMultiplier 1.428571428571429
```

### BFN
```
-GameSettings.InitialDSubLevel Levels/DSub_SocialSpace/DSub_SocialSpace
-GameSettings.StartPoint StartPoint_SocialSpace
-GameSettings.DefaultLayerInclusion GameMode=Mode_SocialSpace;HostedMode=PeerHosted
-NetObjectSystem.MaxServerConnectionCount 4
```

## Environment variables

`CYPRESS_MASTER_URL` Sets the master server to use (e.g. `https://api-cypress.v0e.dev`)

`CYPRESS_REQUIRE_IDENTITY` toggles whether the server will require a EA Authentication.

`CYPRESS_IDENTITY_JWT` unique identifier generated from your EA Account. Required to join servers with EA Auth on.

`CYPRESS_AUTH_TICKET` authenticates a client with relays. 

`CYPRESS_CLIENT_PORT`

`CYPRESS_SERVER_PORT`

`CYPRESS_HIDE_LAUNCHER_UI` Hides errors message windows.

`CYPRESS_ALLOW_GLOBAL_MODS` Allows moderators of the master server to monitor your server.

`CYPRESS_PROXY_KEY`

`CYPRESS_PROXY_REGISTER`

`CYPRESS_PROXY_ADDRESS`

`CYPRESS_PROXY_ACK`

`CYPRESS_BRIDGE_PORT`

### Log level

You can switch between log levels by settings `CYPRESS_LOG_LEVEL` environment variable.

```
Trace
Debug
Info
Warning
Error
```
