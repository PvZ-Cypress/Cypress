set INSTANCEDIRECTORY=./Instance/
set MAPNAME=Levels/Level_Coop_ZombossFactory/Level_Coop_ZombossFactory

set GAMEMODE=GameMode=TeamVanquishLarge0
set HOSTEDMODE=HostedMode=ServerHosted
set TimeOfDay=TOD=Day
::set LAYERINCLUSION=%GAMEMODE%;%TimeOfDay%
set LAYERINCLUSION=%GAMEMODE%;%HOSTEDMODE%;%TimeOfDay%

start GW2.Main_Win64_Retail.exe ^
#-Server.ServerPassword uncommentifyouwantapassword ^
-server ^
-enableServerLog ^
#-dataPath "ModData\Default" ^
#-usePlaylist ^
#-playlistFilename playtest_lov.json ^
-allowMultipleInstances ^
-serverInstancePath "./Instance/" ^
-enableServerLog ^
-level Level_Coop_ZombossFactory ^
-inclusion GameMode=GraveyardOps0;TOD=Day;HostedMode=ServerHosted ^
-GameMode.ModeTeamId 1 ^
#-GameMode.FarewellTaco true ^
-runMultipleInstances ^
-listen 0.0.0.0:25200 ^
-name "GW2 Debug Server" ^
-platform Win32 ^
-console ^
-Game.Platform GamePlatform_Win32 ^
#-Game.DifficultyIndex 12 ^
-Game.EnableServerLog true ^
-Network.ServerPort 25200 ^
-Network.ClientPort 25100 ^
-Network.ServerAddress 0.0.0.0 ^
-GameMode.SkipIntroHubNIS true ^
-Online.Backend Backend_Local ^
-Online.PeerBackend Backend_Local ^
-PVZServer.MapSequencerEnabled false ^
-Network.MaxClientCount 48
