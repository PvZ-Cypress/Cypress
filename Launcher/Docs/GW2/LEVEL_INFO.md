Here's an example for Backyard Battleground.

```
--LEVEL--
Level = Level_FE_Hub

--INCLUSION--
Inclusion = GameMode=FreeRoam;TOD=Day;HostedMode=ServerHosted
```

For Cypress in GW2, you do NOT need the full entire Level path, simply the Level ID, or, Level_FE_Hub, instead of Levels/Level_FE_Hub/Level_FE_Hub.
All parts of the inclusion are separated by a semicolon. How did we determine these values to load with Level_FE_Hub? Let's check the cheat sheet below!

- **Level** = The Level you are going to load into.
- **GameMode** = The type of mode you're going to load into with the Level.
- **TOD** = The Time of Day you're going to load into with the Level.
- **HostedMode** = Whether the Level + Mode loads under its Locally, Peer, Server hosted mode. More explanation at the bottom.

Here are all known Level Paths, left side being their internal name and the right side displaying their in-game name.

MISC:
- Level_FE_Splash - Splash Screen
- Level_FE_Hub - Backyard Battleground
- Level_Hub_TacoBandits - Backyard Battleground (Community Portal Multiplayer)
- Level_HUB_GnomeUnderworld - Trials of Gnomus
- Level_Endless_TimeVoid - The Gnomiverse (Infinity Time)
- Level_Coop_InfinityPlane - Gnomiverse XL (Cats vs. Dinos)

COOP:
- Level_Coop_Asia - Zen Peak
- Level_Coop_Brainstreet - Aqua Center
- Level_Coop_Dino - Boney Island
- Level_Coop_Egypt - Sandy Sands
- Level_Coop_Rome - Colizeum
- Level_Coop_Snow - Frosty Creek
- Level_Coop_Space - Lunar Landing
- Level_Coop_TimePark - Time Park
- Level_Coop_ZombossFactory - Z-Tech Factory
- Level_Coop_Zomburbia - Frontline Flats

RUSH (Gardens and Graveyards):
- Level_Rush_Snow - Great White North
- Level_Rush_Suburbia - Wall-nut Hills
- Level_Rush_Themepark - Seeds of Time

HERB (Herbal Assault):
- Level_Herb_Brainstreet - Zombopolis
- Level_Herb_Space - Moon Base Z
- Level_Herb_Zomburbia - Zomburbia

Now you know all the levels, here are all the available Game Modes. DO KEEP IN MIND, these modes are supported by maps based on Vanilla GW2, or unmodded. Any attempts to load into Levels with unsupported GameModes will result in a blackscreen unless a mod is used which adds support for the GameMode in the Level.

MISC:
- **FreeRoam** - Supported by Backyard Battleground, Trials of Gnomus, and Colizeum
- **StreamingInstall** - Mystery of the Moon - Supported by Moon Base Z
- **SinglePlayerPlants0** - Tutorial - Supported by Zomburbia
- **Splash** - Splash Screen - Supported by Splash Screen

Ops:
- **GardenOps0** - Garden Ops - Supported by all Levels under the COOP tab
- **GraveyardOps0** - Graveyard Ops - Supported by all Levels under the COOP tab
- **Endless0** - Infinity Time - Supported by The Gnomiverse

Turf Takeover:
- **GnGLarge0** - Gardens and Graveyards - Supported by all levels under the RUSH tab
- **HerbalAssaultLarge0** - Herbal Assault - Supported by all levels under the HERB tab

Team Modes:
- **TeamVanquishLarge0** - Team Vanquish - Supported by all Levels under the COOP tab AND the Community Portal version of the Backyard
- **VanquishConfirmedLarge0** - Vanquish Confirmed - Supported by all Levels under the COOP tab AND the Community Portal version of the Backyard
- **TeamVanquishGoats0** - Team Vanquish Goats - Supported by all Levels under the COOP tab
- **SuburbinationLarge0** - Suburbination - Supported by all Levels under the COOP tab
- **GnomeBomb0** - Gnome Bomb - Supported by all Levels under the COOP tab
- **TeamElimination0** - Soil Survivors - Supported by all Levels under the COOP tab EXCEPT Time Park
- **TacoBandits0** - Capture The Taco - Supported by Zen Peak, Colizeum, Lunar Landing, Frontline Flats, and the Community Portal Backyard Battleground.
- **BossHunt0** - Storyline Boss Hunt - Supported by Time Park (Royal Hypno Flower) and Frosty Creek (Captain Smasher)
- **CommunityBossHunt0** - Community Boss Hunt - Supported by Aqua Center (Spooky Squash) Time Park (Brothers Gnomus) Sandy Sands (Sasquatch) Frosty Creek (Yeti King)
- **CatsVsDinos0** - Cats vs. Dinos - Supported by Gnomiverse XL

Here are some Time of Day values!
- **Day** - Supported just about anywhere.
- **Night** - Supported just about anywhere, but works fully on Seeds of Time and Great White North

Here are some Hosted Mode values!
- **LocalHosted** - Affects Ops by making the server load you into the solo variation, where you can summon AI, and affects MP lobbies by adding in the Solo/Private Play bots.
- **PeerHosted** - Affects MP lobbies by adding in the Solo/Private Play bots.
- **ServerHosted** - Typical normal MP lobbies like you'd find in the normal GW2 servers, no Solo/Private Play bots.
