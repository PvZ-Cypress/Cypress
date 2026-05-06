If you want to add these as launch arguments in the Additional Server Launch Arguments box, ensure they have a dash, -, at the start. For example: `-GameMode.CrazyOption2 true`

These are separated by spaces. Example: `-GameMode.CrazyOption2 true -GameMode.CrazyOption5 true`

- **GameMode.CrazyOption1** - Doesn't exist
- **GameMode.CrazyOption2** - Low Gravity
- **GameMode.CrazyOption3** - Half Health
- **GameMode.CrazyOption4** - Fast Reload
- **GameMode.CrazyOption5** - Speed Boost Craziness
- **GameMode.CrazyOption6** - Simply Explode!
- **GameMode.CrazyOption7** - Time Warp (Slowmo on Inflicting Damage)
- **GameMode.CrazyOption9** - Time Bomb Madness - Semi works, Time doesn't reset on kill
- **GameMode.CrazyOption10** - Death Bomb (Zombies Only)
- **GameMode.CrazyOption11** - Death Bomb (Plants Only)
- **GameMode.CrazyOption12** - Fast Reload (Zombies Only)
- **GameMode.CrazyOption13** - Fast Reload (Plants Only)
- **GameMode.CrazyOption14** - Half Health (Zombies Only)
- **GameMode.CrazyOption15** - Half Health (Plants Only)
- **GameMode.CrazyOption16** - Speed Boost Craziness (Zombies Only)
- **GameMode.CrazyOption17** - Speed Boost Craziness (Plants Only)
- **GameMode.SkipSpawnRandomCostumes** - Character Jumble
- **GameMode.IsCoolLasers** - Satellite Takeover (Turf) - Enables lasers that knock you back when capturing point
- **GameMode.IsSuburbinationKotH** - Suburbination Isolation - Only enables one active point at a time

- **GameMode.BackfillMpWithAI** - Determines if AI are able to backfill in lobbies
- **GameMode.LocalPlayPlantBotsPerTeam** - Determines the maximum of backfill AI Plants
- **GameMode.LocalPlayZombieBotsPerTeam** - Determines the maximum of backfill AI Zombies

**GameMode.WeeklyEventIndex** - Enables events in certain Game Modes
- 6 - Boss Team Vanquish - Team Vanquish
- 9 - Boss Turf Takeover - Turf Takeover
- 10 - Power Team Vanquish - Team Vanquish
- 21 - Double Trouble - Double Damage in Vanquish Confirmed

- **GameMode.ShouldPreventBotSpawnsInSocialHUB** - Determines if you do not want AI in your Giddy Park session
- **GameMode.ForceHUBSeason** - Forces Giddy Park to load under different seasons
  - 0 - Default
  - 1 - Default (Night)
  - 2 - Lawn of Doom (October - Halloween)
  - 3 - Food Fight (November - Thanksgiving)
  - 4 - Feastivus (December - Christmas)
  - 5 - Snow Day / Beastiez (January - Winter)
  - 6 - Valenbrainz (February - Valentines)
  - 7 - Luck of the Zombie (March - St. Patrick's)
  - 8 - Springening (April - Easter)
  - 9 - Birthdayz (May - Anniversary)
  - 10 - Summer Nights (June - Summer)
  - 11 - Summer Days / Summer Sportz - (July - Summer)
  - 12 - Summer Nights Closing
  - 13 - Season12Placeholder

- **GameMode.OverridePreRoundTime** - Overrides how long Preround is (in seconds) before the match begins
- **GameMode.OverrideRoundStartPlayerCount** - Overrides how many players need to be in a match, spawned in, before the game starts.

- **GameMode.LocalPlaySelectedAICharacterSetPlantsUnlockId** - Determines what Plants spawn based on the AICharacterSetAsset's Identifier
- **GameMode.LocalPlaySelectedAICharacterSetZombiesUnlockId** - Determines what Zombies spawn based on the AICharacterSetAsset's Identifier

If the integer has a negative beside, make sure to copy that as well.

Back-up in Arms:
- Plants - 2090792163
- Zombies - 1563382864

Collision Course:
- Plants - -1990775730
- Zombies - 1658927976

Cool Explosions Only:
- Plants - 418761909
- Zombies - -797648325

Garden Warfare:
- Plants - 1026679735
- Zombies - 1271312176

Harm, Help, Hinder:
- Plants - -1305270575
- Zombies - -1535154181

Lunchtime at the Lab:
- Plants - 490694228
- Zombies - -1627931030

Minion Madness:
- Plants - 1520392824
- Zombies - 1849893156

Movers and Shakers:
- Plants - 2004063672
- Zombies - 52123528

My Turf, My Rules:
- Plants - -2015125458
- Zombies - 448280939

The Reinforcements:
- Plants - -1541534021
- Zombies - -1419477310

Space Nuts:
- Plants - 1677687298
- Zombies - 603979365

Up-close & Impersonal:
- Plants - 1964445672
- Zombies - 1133254212

**SoloPlaySettings.DisabledPlantAIClassMask** - Determines what batch of Plant will spawn in based on a certain bitmask
**SoloPlaySettings.DisabledZombieAIClassMask** - Determines what batch of Zombies will spawn in based on a certain bitmask

Plants:
- Peashooter - 1
- Chomper - 2
- Kernel Corn - 4
- Night Cap - 8
- Snap Dragon - 16
- Cactus - 32
- Citron - 64
- Acorn - 128
- Sunflower - 256
- Rose - 512
- Wildflower - 1024

Zombies:
- Foot Soldier - 1
- Imp - 2
- Super Brainz - 4
- 80's Action Hero - 8
- Electric Slide - 16
- Captain Deadbeard - 32
- All-Star - 64
- Space Cadet - 128
- Scientist - 256
- Engineer - 512
- Wizard - 1024
- TV Head - 2048


Any character(s) you want to hide, you add up based on the mask value, the numbers on the right. So, in this instance, if you want to hide Scientist and All-Star, so no AI of them spawn, you would add 256 and 64, and 320 will be the value used for SoloPlaySettings.DisabledZombieAIClassMask

**SyncedPVZSettings.CharacterKillSwitches** - Prevents you from selecting specified classes. Based on name of the Class' Kit file.

- Peashooter - PeashooterClass
- Chomper - ChomperClass
- Kernel Corn - CornClass
- Night Cap - MushroomClass
- Snapdragon - SnapDragonClass
- Cactus - CactusClass
- Citron - CitronClass
- Acorn - AcornClass
- Sunflower - SunflowerClass
- Rose - RoseClass
- WildFlower - WildflowerClass

- Foot Soldier - SoldierClass
- Imp - ImpClass
- Super Brainz - SuperBrainzClass
- 80's Action Hero - ActionHeroClass
- Electric Slide - ElectricClass
- Captain Deadbeard - PirateClass
- All-Star - AllStarClass
- Space Cadet - AstronautClass
- Scientist - ScientistClass
- Engineer - EngineerClass
- Wizard - WizardClass
- TV Head - TVHeadClass

Formatted similar to a JSON entry. Encased with brackets, []. Each Class being encased in Quotations, before being separated by a comma, looking something like this.
`SyncedPVZSettings.CharacterKillSwitches ["RoseClass","ImpClass","WizardClass"]` <-- This command prevents Rose, Imp, and Wizard from being selectable.

Here are some settings to use if you want to emulate Weekly Modes!

Back-up in Arms:
`SyncedPVZSettings.CharacterKillSwitches ["PeashooterClass","ChomperClass","CornClass","MushroomClass","SnapDragonClass","CactusClass","CitronClass","AcornClass","WildflowerClass","SoldierClass","ImpClass","SuperBrainzClass","ActionHeroClass","ElectricClass","PirateClass","AllStarClass","AstronautClass","WizardClass","TVHeadClass"]`

Collision Course:
`SyncedPVZSettings.CharacterKillSwitches ["PeashooterClass","ChomperClass","CornClass","MushroomClass","SnapDragonClass","CactusClass","RoseClass","WildflowerClass","SoldierClass","ImpClass","SuperBrainzClass","ActionHeroClass","ElectricClass","PirateClass","EngineerClass","WizardClass","TVHeadClass"]`

Cool Explosions Only:
`SyncedPVZSettings.CharacterKillSwitches ["ChomperClass","MushroomClass","CactusClass","CitronClass","AcornClass","SunflowerClass","RoseClass","WildflowerClass","ImpClass","ElectricClass","PirateClass","AllStarClass","AstronautClass","ScientistClass","EngineerClass","WizardClass","TVHeadClass"]`

Garden Warfare:
`SyncedPVZSettings.CharacterKillSwitches ["CornClass","MushroomClass","SnapDragonClass","CitronClass","AcornClass","RoseClass","WildflowerClass","ImpClass","SuperBrainzClass","ActionHeroClass","ElectricClass","PirateClass","AstronautClass","WizardClass","TVHeadClass"]`

Harm, Help, Hinder:
`SyncedPVZSettings.CharacterKillSwitches ["PeashooterClass","ChomperClass","CornClass","SnapDragonClass","CactusClass","CitronClass","RoseClass","WildflowerClass","SoldierClass","SuperBrainzClass","ActionHeroClass","ElectricClass","AllStarClass","AstronautClass","EngineerClass","WizardClass","TVHeadClass"]`

Lunchtime at the Lab:
`SyncedPVZSettings.CharacterKillSwitches ["PeashooterClass","CornClass","MushroomClass","SnapDragonClass","CactusClass","CitronClass","AcornClass","SunflowerClass","RoseClass","WildflowerClass","SoldierClass","ImpClass","SuperBrainzClass","ActionHeroClass","ElectricClass","PirateClass","AllStarClass","AstronautClass","EngineerClass","WizardClass","TVHeadClass"]`

Minion Madness:
`SyncedPVZSettings.CharacterKillSwitches ["PeashooterClass","ChomperClass","CornClass","MushroomClass","SnapDragonClass","CactusClass","CitronClass","AcornClass","SunflowerClass","RoseClass","SoldierClass","ImpClass","SuperBrainzClass","ActionHeroClass","ElectricClass","PirateClass","AllStarClass","AstronautClass","ScientistClass","EngineerClass","WizardClass"]`

Movers and Shakers:
`SyncedPVZSettings.CharacterKillSwitches ["ChomperClass","CornClass","SnapDragonClass","CactusClass","CitronClass","AcornClass","SunflowerClass","WildflowerClass","SoldierClass","SuperBrainzClass","ActionHeroClass","PirateClass","AllStarClass","AstronautClass","ScientistClass","WizardClass","TVHeadClass"]`

My Turf, My Rules:
`SyncedPVZSettings.CharacterKillSwitches ["PeashooterClass","ChomperClass","CornClass","MushroomClass","CactusClass","CitronClass","AcornClass","SunflowerClass","WildflowerClass","SoldierClass","ImpClass","ActionHeroClass","PirateClass","AllStarClass","AstronautClass","ScientistClass","EngineerClass","WizardClass","TVHeadClass"]`

The Reinforcements:
`SyncedPVZSettings.CharacterKillSwitches ["PeashooterClass","ChomperClass","CornClass","CactusClass","CitronClass","SunflowerClass","RoseClass","WildflowerClass","SoldierClass","ImpClass","SuperBrainzClass","PirateClass","AllStarClass","ScientistClass","EngineerClass","WizardClass","TVHeadClass"]`

Space Nuts:
`SyncedPVZSettings.CharacterKillSwitches ["PeashooterClass","ChomperClass","CornClass","MushroomClass","SnapDragonClass","CactusClass","CitronClass","SunflowerClass","RoseClass","WildflowerClass","SoldierClass","ImpClass","SuperBrainzClass","ActionHeroClass","ElectricClass","PirateClass","AllStarClass","ScientistClass","EngineerClass","WizardClass","TVHeadClass"]`

Up-close & Impersonal
`SyncedPVZSettings.CharacterKillSwitches ["PeashooterClass","ChomperClass","CornClass","MushroomClass","CitronClass","AcornClass","SunflowerClass","RoseClass","WildflowerClass","SoldierClass","ImpClass","ActionHeroClass","ElectricClass","AllStarClass","AstronautClass","ScientistClass","EngineerClass","WizardClass","TVHeadClass"]`
