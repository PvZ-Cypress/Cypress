To get Cypress Launcher to detect playlists, create a Playlists folder in your game's directory. Any playlist JSON that is put in the folder will be found by the launcher.

There are a few settings that come before two different methods of rotation. Those include:
- **RoundsPerSetup** - An integer that determines how many rounds of the same Level and GameMode play before the playlist rotates.
- **IsMixed** - A boolean that determines if Levels and GameModes will be randomized instead of following an order. More info below
- **LoadScreen_GamemodeNameOverride** - Overrides the GameMode's name. Will affect every level in rotation of the playlist.
- **LoadScreen_LevelNameOverride** - Overrides level's name. Will affect every level in rotation of the playlist.
- **LoadScreen_LevelDescriptionOverride** - Overrides the Level's Description. Will affect every level in rotation of the playlist.
- **LoadScreen_UIAssetPathOverride** - Overrides the Level's Load Screen. Will affect every level in rotation of the playlist.

Here are two different methods of Rotations. PlaylistRotation, and if IsMixed is set to true. Let's go over PlaylistRotation.

## PLAYLIST ROTATION
PlaylistRotation is what's used if you want your playlist to run in a specified order. This list goes from top to bottom. There are several options determined:
- **LevelName** (Required)
- **GameMode** (Required)
- **StartPoint** (Required - ONLY FOR BFN)
- **TOD** (Optional)
- **SettingsToApply** (Optional)

LevelName is the Level you want to load into. HOWEVER, unlike the Sever.LoadLevel command, where you can have just a Level name like Level_Rush_Snow, you'll need the entire level's path, which is equivalent to Levels/Level_Rush_Snow/Level_Rush_Snow. The format is basically Levels/'LevelName'/'LevelName'. For GW1, levels are in completely different paths instead of being organized to one folder. You can find their complete paths in its own LevelInfo Readme.

GameMode is the Game Mode the level loads in to.
StartPoint is equivalent to that of Sub-GameModes. More Info in the BFNLevelInfo ReadMe.
TOD is the Time of Day, whether it's Day or Night
SettingsToApply are game changing settings/modifiers that'll be applied to the server.

Remember, If you want settings, like let's say for this example, crazy settings! You will put certain settings that'll be applied to SettingsToApply. Each entry in SettingsToApply is separated by a Vertical Bar, or |

Example:
The first entry of the playlist enables
`GameMode.CrazyOption8 true|GameMode.CrazyOption3 true`

If you want these turned off on the next round, then in the next entry, you'll add it to the SettingsToApply in there, and make it
`GameMode.CrazyOption8 false|GameMode.CrazyOption3 false`

More info on Game Modifiers can be found in the corresponding game modifiers file.

- **LoadScreen_GamemodeName** (Optional) - Overrides the GameMode's name.
- **LoadScreen_LevelName** (Optional) - Overrides level's name.
- **LoadScreen_LevelDescription** (Optional) - Overrides the Level's Description.
- **LoadScreen_UIAssetPath** (Optional) - Overrides the Level's Load Screen. - Use example, wanting to replace the Sandy Sands loading screen with the one from Sasquatch, you set `"LoadScreen_UIAssetPath"` to `"_pvz/UI/Assets/LoadScreen_CP_Sasquatch"`

## ISMIXED
If IsMixed is set to true, Cypress will expect two settings, an additional, optional setting. Those being:
- **AvailableModes** - A list of GameModes that will be randomized in the playlist
- **AvailableLevelsForModes** - A list determining what GameMode will be exclusive to what Levels in the randomization
- **AvailableTODForLevels** - Optional - A list determining what Levels will have Time of Day randomization

Examples of the structurization can be found inside the PlaylistExamples folder.

Bit of a heads up, the structure on playlists using IsMixed for BFN is a tad different. If you wish to make randomized Playlists for BFN, it's recommended to check the example provided and check up its LevelInfo ReadMe.
