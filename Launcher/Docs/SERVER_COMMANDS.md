Around the bottom of the server window, right above all the information such as FPS, Level, and so on is text box you can type in. There are certain commands you can input!
Here are a list of special server commands available for Garden Warfare 1, 2, and BFN!

Garden Warfare & BFN:
- Server.RestartLevel
- Server.LoadLevel
- Server.KickPlayer
- Server.KickPlayerById
- Server.BanPlayer
- Server.BanPlayerById
- Server.UnbanPlayer
- Server.LoadNextPlaylistSetup

Garden Warfare 2:
- Server.RestartLevel
- Server.LoadLevel
- Server.KickPlayer
- Server.KickPlayerById
- Server.BanPlayer
- Server.BanPlayerById
- Server.UnbanPlayer
- Server.Say
- Server.SayToPlayer
- Server.LoadNextPlaylistSetup

Here's a cheat sheet dictionary of what all of these commands do, and their needed indexes.

- **Server.RestartLevel** - Restarts the current level the server is hosting
- **Server.LoadLevel** - Loads the level of a server, requires the level and inclusion - EXAMPLE: `Server.LoadLevel Level_FE_Hub GameMode=FreeRoam;TOD=Day;HostedMode=ServerHosted` - More info can be found within the Level Info readmes.
- **Server.KickPlayer** - Kicks a player out of the server based on name - EXAMPLE: `Server.KickPlayer Jim`
- **Server.KickPlayerById** - Kicks a player out of the server based on the Player ID the server assigned them - EXAMPLE: `Server.KickPlayerById 4`
- **Server.BanPlayer** - Bans a player out of the server based on name - EXAMPLE: `Server.BanPlayer Jim`
- **Server.BanPlayerById** - Bans a player out of the server based on the Player ID the server assigned them - EXAMPLE: `Server.BanPlayerById 4`
- **Server.UnbanPlayer** - Unbans a player based off name used before Ban. EXAMPLE: `Server.UnbanPlayer 'Jimmy'`
- **Server.Say** - Displays a message to everyone's screen for a certain amount of time. - EXAMPLE: `Server.Say 'Hello World!' 5`
- **Server.SayToPlayer** - Displays a message to a specified user's screen for a certain amount of time. EXAMPLE: `Server.SayToPlayer 'Jim' 'Hello World!' 5`
- **Server.LoadNextPlaylistSetup** - Loads the next setup in a Playlist.

You can also initiate settings found within settings_Win32! - EXAMPLE: `GameMode.ModeTeamId 2`
