BFN introduces to StartPoints. From what can be examined, StartPoints are pretty much closely similar to that of GameModes.

To make the game level system more optimized, BFN uses DSubs, which are Sub Levels of the main level, Level_Picnic_Root. DSubs in this case, work the exact same as Levels in GW1 and GW2.

Here is an example of a level setup.

```
DSub = DSub_SocialSpace
Inclusion = GameMode=Mode_SocialSpace
StartPoint = StartPoint_SocialSpace
```

Here are all the DSubs

MISC:
- DSub_SocialSpace - Giddy Park
- DSub_4v4_SuperArena - Funderdome
- DSub_4v4_BattleArena_V2 - Funderdome V2

REGIONS:
- DSub_Region_CheeseMines - Mount Steep (Region - Plants) - Rocky Flats (MP)
- DSub_Region_Dreadwood - Weirding Woods (Region - Zombies) - Z-Tech Factory (MP)
- DSub_Region_TownCenter - Town Center (Region - Plants & Zombies) - Pressure Pier (MP)

RUSH:
- DSub_Rush_CheeseMines - Goopy Gully
- DSub_Rush_Dreadwood - Loggy Acres
- DSub_Rush_Farm - Preserve Pastures
- DSub_Rush_Resort - Tourist Trap Island
- DSub_Rush_Shoreline - Turning Point
- DSub_Rush_TownCenter - Peachy District

COOP:
- DSub_TVM_CheeseMines - Colossal Fossil
- DSub_TVM_Dreadwood - Ruiny Ruins
- DSub_TVM_Suburbs - Daisy Drive
- DSub_TVM_ZRefinery - ZTech Oozevoir

Here's a list of all the GameModes
- **FreeRoam** - Freely Roam around the Map - Works on ALL maps that isn't Giddy Park
- **Mode_Competitive_12** - Intended for 6v6 modes.
- **Mode_Competitive_16** - Intended for 8v8 modes.
- **Mode_Competitive_24** - Intended for 12v12 modes.
- **Mode_Competitive_8** - Intended for 4v4 Modes.
- **Mode_Coop_Plant** - Intended for Garden Ops & Regions
- **Mode_Coop_SoloPlant** - Intended for Garden Ops (Good for Playlists as it contains EOR, End of Round)
- **Mode_Coop_SoloZombie** - Intended for Graveyard Ops (Good for Playlists as it contains EOR, End of Round)
- **Mode_Coop_Zombie** - Intended for Graveyard Ops & Regions
- **Mode_SocialSpace** - Intended for Giddy Park

Here's a list containing a vast majority of StartPoints
- **StartPoint_FreeRoam** - Works on ALL maps that isn't Giddy Park
- **StartPoint_SocialSpace** - Works on Giddy Park with Mode_SocialSpace GameMode
- **StartPoint_Rush** - Turf Takeover - Works on all maps under RUSH using any Mode_Competitive GameMode
- **StartPoint_TVM** - Team Vanquish - Works on all maps under COOP, REGION, and Funderdome using any Mode_Competitive GameMode
- **StartPoint_VanquishConfirmed** - Vanquish Confirmed - Works on all maps under COOP & REGION using any Mode_Competitive GameMode
- **StartPoint_Suburbination** - Suburbination - Works on all maps under COOP and REGION using any Mode_Competitive GameMode
- **StartPoint_GnomeBomb** - Gnome Bomb - Works on all maps under COOP and REGION using any Mode_Competitive GameMode
- **StartPoint_Coop_Ops_GardenA** - Garden Ops - Works on all maps under COOP and REGION using Mode_Coop_Plant and Mode_Coop_SoloPlant GameMode
- **StartPoint_Coop_Ops_GraveyardA** - Graveyard Ops - Works on all maps under COOP and REGION using Mode_Coop_Zombie and Mode_Coop_SoloZombie GameMode
- **StartPoint_CUOps_GraveOps** - Graveyard Ops (Phase 3) - Works on Ruiny Ruins with Mode_Coop_Zombie and Mode_Coop_SoloZombie GameMode
- **StartPoint_TeamElimination** - Battle Arena - Works on Funderdome and Funderdome v2 with Mode_Competitive_8 GameMode
- **StartPoint_Coop_Explore_Plants** - Region (Plants) - Works on Town Center and Mount Steep with Mode_Coop_Plant GameMode
- **StartPoint_Coop_Boss_Plants** - Region Boss Fight (Plants) - Works on Town Center (Olds Cool) and Mount Steep (Major Problem) with Mode_Coop_Plant GameMode
- **StartPoint_Coop_Explore_Zombies** - Region (Zombies) - Works on Town Center and Weirding Woods with Mode_Coop_Zombie GameMode
- **StartPoint_Coop_Boss_Zombies** - Region Boss Fight (Zombies) - Works on Town Center (Blight Cap) and Weirding Woods (Dreadwood) with Mode_Coop_Zombie GameMode

**StartPoint_Coop_GnomeShrine** - Region Gnome Boss Fight - Works on the following:
- Town Center (Sir Biff) - Plants (Mode_Coop_Plant) - Zombies (Mode_Coop_Zombie)
- Weirding Woods (Sir Baff) - Zombies (Mode_Coop_Zombie)
- Mount Steep (Sir Boff) - Plants (Mode_Coop_Plant)
