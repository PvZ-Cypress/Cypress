Here's an example for Garden Ops in Garden Center;

```
--LEVEL--
Level = _pvz/Levels/Coastal/Level_COOP_Coastal/Level_COOP_Coastal

--INCLUSION--
Inclusion = GameMode=Coop0;TOD=Day
```

For Cypress in GW1, you do need the full entire Level path, something similar to what's shown in Level above.
All parts of the inclusion are separated by a semicolon. How did we determine these values to load? Why is there a 0 after Coop? Let's check the cheat sheet below!

- **Level** = The Level you are going to load into.
- **GameMode** = The type of mode you're going to load into with the Level. The number at the end of the GameMode, 0 or 1, determine which variation of the Level is loaded. Make sure it is included. More on that below.
- **TOD** = The Time of Day you're going to load into with the Level.

Here are all known Level Paths, left side being their internal name and the right side displaying their in-game name.

COOP:
- `_pvz/Levels/Coastal/Level_COOP_Coastal/Level_COOP_Coastal` - Sharkbite Shores (0) or Port Scallywag (1)
- `_pvz/Levels/Mainstreet/Level_COOP_Mainstreet/Level_COOP_Mainstreet` - Garden Center (0) or Chomp Town (1)
- `_pvz/Levels/COOP_Suburbia/Level_COOP_Suburbia/Level_COOP_Suburbia` - Suburban Flats (0) or Zomboss Estate (1)
- `_pvz/Levels/Desert/Level_COOP_Desert/Level_COOP_Desert` - Jewl Junction (0) - Supports Night TOD
- `_pvz/Levels/Desert/Level_COOP_Palms/Level_COOP_Palms` - Crash Course (0) - Supports Night TOD

RUSH (Gardens and Graveyards):
- `_pvz/Levels/Coastal/Rush_Coastal_001/Rush_Coastal_001` - Driftwood Shores (0) - Supports Night TOD
- `_pvz/Levels/Desert/Rush_Desert_001/Rush_Desert_001` - Cactus Canyon (0) - Supports Night TOD
- `_pvz/Levels/Sandbox/Level_Suburbia2/Level_Suburbia2` - Main Street (0) - Supports Night TOD
- `_pvz/Levels/Sandbox/Level_SuburbiaC/Level_SuburbiaC` - Wall-nut Hills (0) - Supports Night TOD

Now you know all the levels, here are all the available Game Modes. DO KEEP IN MIND that you'll need either 0 or 1 after the GameMode you've chosen, shown above in all Levels above, to determine the Map that'll be loaded.

Ops:
- **Coop** - Supported by all Levels under the COOP tab (Garden Ops)

Turf Takeover:
- **RushLarge** - Supported by all levels under the RUSH tab (Gardens and Graveyards)

Team Modes:
- **TeamDeathMatch** - Supported by all Levels under the COOP tab (Team Vanquish)
- **SmallTDM** - Supported by all Levels under the COOP tab (Super Team Vanquish)
- **KillConfirmed** - Supported by all Levels under the COOP (Vanquish Confirmed)
- **Domination** - Supported by all Levels under the COOP tab (Suburbination)
- **GnomeBomb** - Supported by all Levels under the COOP tab (Gnome Bomb)
- **SingleFlagCTF** - Supported by all Levels under the COOP tab (Taco Bandits)
- **ChompVirus** - Supported by Garden Center (0) and Chomp Town (1)

Here are some Time of Day values!
- **Day** - Supported in all Levels!
- **Night** - Supported in levels above saying they have support.
