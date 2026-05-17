#include "pch.h"
#ifdef CYPRESS_GW2
#include "LoadoutValidator.h"
#include <unordered_map>
#include <StringUtil.h>
#include <EASTL/sort.h>

#include <fb/Engine/ServerPlayer.h>
#include <fb/TypeInfo/PVZCharacterCustomizationAsset.h>
#include <fb/Engine/ResourceManager.h>

#include "Cypress/Core/Program.h"

bool LoadoutValidator::isValidTeamForFaction(int teamId, bool isPlant, bool isZombie) {
    if (isPlant && teamId != 2) return false;
    if (isZombie && teamId != 1) return false;
    return true;
}

static std::string normalizeTeamName(int teamId)
{
    if (teamId == 2) return "plants";
    if (teamId == 1) return "zombies";
    return "unknown";
}

ValidationResult LoadoutValidator::validateWeapons(fb::PVZCharacterServerPlayerExtent* extent, const WeaponSet& allowedWeapons, const char* playerName)
{
    ValidationResult result;

    if (!extent)
    {
        result.addFlag(ValidationFlag::NullPointer);
        return result;
    }

    // primary weapon
    if (extent->m_primary) {
        result.invalidPrimary = extent->m_primary->Name;
        result.weaponName = cutPath(extent->m_primary->Name);

        const uint32_t hash = fnvHash(cutPath(extent->m_primary->Name));
        if (!allowedWeapons.primary.count(hash)) {
            bool prefixAllowed = false;
            if (extent->m_primary->Name) {
                std::string fullName = extent->m_primary->Name;
                for (const auto& prefix : allowedWeapons.allowedPrimaryPrefixes) {
                    if (fullName.size() >= prefix.size() && fullName.compare(0, prefix.size(), prefix) == 0) {
                        prefixAllowed = true;
                        break;
                    }
                }
            }
            if (!prefixAllowed) {
                result.addFlag(ValidationFlag::InvalidPrimary);
            }
        }
    }

    if (!allowedWeapons.skipAbilityValidation)
    {
        // ability 1
        if (extent->m_ability1) {
            result.invalidAbility1 = extent->m_ability1->Name;

            const uint32_t hash = fnvHash(cutPath(extent->m_ability1->Name));
            if (!allowedWeapons.ability1.count(hash)) {
                result.addFlag(ValidationFlag::InvalidAbility1);
            }
        }

        // ability 2
        if (extent->m_ability2) {
            result.invalidAbility2 = extent->m_ability2->Name;

            const uint32_t hash = fnvHash(cutPath(extent->m_ability2->Name));
            if (!allowedWeapons.ability2.count(hash)) {
                result.addFlag(ValidationFlag::InvalidAbility2);
            }
        }

        // ability 3
        if (extent->m_ability3) {
            result.invalidAbility3 = extent->m_ability3->Name;

            const uint32_t hash = fnvHash(cutPath(extent->m_ability3->Name));
            if (!allowedWeapons.ability3.count(hash)) {
                result.addFlag(ValidationFlag::InvalidAbility3);
            }
        }

        // alternate weapon
        if (extent->m_alternate) {
            result.invalidAlternate = extent->m_alternate->Name;

            if (!allowedWeapons.allowAlternate) {
                result.addFlag(ValidationFlag::InvalidAlternate);
            }
            else {
                const uint32_t hash = fnvHash(cutPath(extent->m_alternate->Name));
                if (!allowedWeapons.alternate.count(hash)) {
                    result.addFlag(ValidationFlag::InvalidAlternate);
                }
            }
        }
    }

    if (!extent->m_primary)
        return result;

    if (allowedWeapons.skipUpgradeValidation)
        return result;

    const uint32_t primaryHash = fnvHash(cutPath(extent->m_primary->Name));

    auto it = upgradeSets.find(primaryHash);

    UpgradeSet allowedUpgrades;
    if (it == upgradeSets.end()) {
        result.addFlag(ValidationFlag::InvalidSoldier);
    }
    else {
        allowedUpgrades = it->second;
    }

    validateUpgrades(extent, result, allowedUpgrades, playerName);

    return result;
}

void LoadoutValidator::validateUpgrades(fb::PVZCharacterServerPlayerExtent* extent, ValidationResult& result, const UpgradeSet& allowedUpgrades, const char* playerName)
{
    if (extent->m_upgrade[0])
    {
        for (int i = 0; i < extent->m_numUpgrades; i++)
        {
            fb::Asset* upgrade = extent->m_upgrade[i];
            if (upgrade)
            {
                result.invalidUpgrades.push_back(upgrade->Name);

                const uint32_t hash = fnvHash(cutPath(upgrade->Name));

                if (!allowedUpgrades.selectable.count(hash) && (!allowedUpgrades.hasNonSelectable || !allowedUpgrades.nonSelectable.count(hash))) {
                    result.addFlag(ValidationFlag::InvalidUpgrade);
                }
            }
        }
    }
}

void LoadoutValidator::init()
{
    weaponSets.clear();
    upgradeSets.clear();
    upgradableWeaponIds.clear();

    const std::set<std::string>& effectiveBlacklist = kitBlacklist;

    std::vector<fb::PVZCharacterCustomizationAsset*> foundKits;

    fb::ResourceManager* resourceManager = fb::ResourceManager::getInstance();
    if (resourceManager == nullptr)
    {
        g_program->GetServer()->GetAnticheat()->AC_LogMessage(LogLevel::Error, "Failed to get resource manager");
        return;
    }

    fb::ResourceManager::Compartment* compartment = resourceManager->m_compartments[3];
    if (compartment == nullptr)
    {
        g_program->GetServer()->GetAnticheat()->AC_LogMessage(LogLevel::Error, "Failed to get GameStatic compartment");
        return;
    }

    for (fb::Asset* container : compartment->m_dataContainers)
    {
        if (container == nullptr)
            continue;

        if (container->getType() != fb::PVZCharacterCustomizationAsset::c_TypeInfo)
            continue;

        if (effectiveBlacklist.count(container->Name))
            continue;


        //g_program->GetServer()->GetAnticheat()->AC_LogMessage(LogLevel::Debug, "Found kit {} at {}", container->Name, reinterpret_cast<void*>(container));
        foundKits.push_back(reinterpret_cast<fb::PVZCharacterCustomizationAsset*>(container));
    }

    g_program->GetServer()->GetAnticheat()->AC_LogMessage(LogLevel::Info, "Found {} kits", foundKits.size());

    for (fb::PVZCharacterCustomizationAsset* kit : foundKits)
    {
        if (kit == nullptr)
            continue;

        uint32_t kitHash = fnvHash(cutPath(kit->Name));

        g_program->GetServer()->GetAnticheat()->AC_LogMessage(LogLevel::Debug, "=== {} ===", kit->Name);

        for (int i = 0; i < fb::WeaponSlot::WeaponSlot_NumSlots; i++)
        {
            fb::WeaponSlot slot = static_cast<fb::WeaponSlot>(i);

            std::set<fb::PVZCharacterWeaponUnlockAsset*> weapons = kit->getWeaponsInSlot(slot);

            if (weapons.size() == 0)
                continue;

            g_program->GetServer()->GetAnticheat()->AC_LogMessage(LogLevel::Debug, "{}:", fb::toString(slot));

            for (fb::PVZCharacterWeaponUnlockAsset* weapon : weapons)
            {
                g_program->GetServer()->GetAnticheat()->AC_LogMessage(LogLevel::Debug, "- {} ({})", weapon->Name, reinterpret_cast<void*>(weapon));

                uint32_t weaponHash = fnvHash(cutPath(weapon->Name));

                auto& set = weaponSets.try_emplace(kitHash).first->second;

                switch (slot)
                {
                case fb::WeaponSlot::WeaponSlot_0:
                    set.primary.insert(weaponHash);
                    set.primaryNames.push_back(cutPath(weapon->Name));
                    break;
                case fb::WeaponSlot::WeaponSlot_1: set.ability1.insert(weaponHash); break;
                case fb::WeaponSlot::WeaponSlot_2: set.ability2.insert(weaponHash); break;
                case fb::WeaponSlot::WeaponSlot_3: set.ability3.insert(weaponHash); break;
                case fb::WeaponSlot::WeaponSlot_4:
                    {
                        upgradableWeaponIds.push_back(weapon->getIdentifier());
                        set.alternate.insert(weaponHash);
                        set.allowAlternate = true;
                        break;
                    }
                case fb::WeaponSlot::WeaponSlot_5: set.weaponSlot5.insert(weaponHash); break;
                case fb::WeaponSlot::WeaponSlot_6: set.weaponSlot6.insert(weaponHash); break;
                case fb::WeaponSlot::WeaponSlot_7: set.weaponSlot7.insert(weaponHash); break;
                case fb::WeaponSlot::WeaponSlot_8: set.weaponSlot8.insert(weaponHash); break;
                case fb::WeaponSlot::WeaponSlot_9: set.weaponSlot9.insert(weaponHash); break;
                default: break;
                }

                auto& upgradeSet = upgradeSets.try_emplace(weaponHash).first->second;

                fb::Array<fb::Asset*> selectableUpgrades = weapon->getSelectableWeaponUpgrades();
                for (fb::Asset* upgrade : selectableUpgrades)
                {
                    if (upgrade == nullptr)
                        continue;

                    uint32_t upgradeHash = fnvHash(cutPath(upgrade->Name));
                    upgradeSet.selectable.insert(upgradeHash);
                }

                fb::Array<fb::Asset*> nonSelectableUpgrades = weapon->getNonSelectableWeaponUpgrades();

                upgradeSet.hasNonSelectable = (nonSelectableUpgrades.size() > 0);

                for (fb::Asset* upgrade : nonSelectableUpgrades)
                {
                    if (upgrade == nullptr)
                        continue;

                    uint32_t upgradeHash = fnvHash(cutPath(upgrade->Name));
                    upgradeSet.nonSelectable.insert(upgradeHash);
                }
            }
        }
    }

    struct SpecialKit {
        const char* kitPath;
        const char* weaponPrefix;
        const char* requiredMode;
        const char* allowedSoldier;
    };

    static const SpecialKit specialKitDefs[] = {
        {"Gameplay/Kits/Plant_Junkasaurus",     "Gameplay/Weapons/Junkasaurus/", "Endless0",     "MpPlant_Junkasaurus"},
        {"Gameplay/Kits/Zombie_ZombossCat",     "Gameplay/Weapons/ZombossCat/",  "Endless0",     "MpZombie_ZombossCat"},
        {"Gameplay/Kits/Plant_Junkasaurus_CvD", "Gameplay/Weapons/Junkasaurus/", "CatsVsDinos0", "MpPlant_Junkasaurus_CvD"},
        {"Gameplay/Kits/Zombie_ZombossCat_CvD", "Gameplay/Weapons/ZombossCat/",  "CatsVsDinos0", "MpZombie_ZombossCat_CvD"},
        {"Gameplay/Kits/Plant_LaserGoat_Green", "Gameplay/Weapons/LaserGoat/",  "TeamVanquishGoats0", "MpPlant_LaserGoat_Green"},
        {"Gameplay/Kits/Zombie_LaserGoat_Purple", "Gameplay/Weapons/LaserGoat/",  "TeamVanquishGoats0", "MpZombie_LaserGoat_Purple"},
    };

    for (const auto& sk : specialKitDefs)
    {
        const uint32_t kitHash = fnvHash(cutPath(sk.kitPath));
        auto& set = weaponSets[kitHash];
        set.allowedPrimaryPrefixes.push_back(sk.weaponPrefix);
        set.allowedSoldierSubstrings.push_back(sk.allowedSoldier);
        set.requiredMode = sk.requiredMode;
        set.skipAbilityValidation = true;
        set.skipUpgradeValidation = true;
    }
    g_program->GetServer()->GetAnticheat()->AC_LogMessage(LogLevel::Info, "Registered {} special kits", std::size(specialKitDefs));

    m_initialized = true;
    g_program->GetServer()->GetAnticheat()->AC_LogMessage(LogLevel::Info, "Weapon and Upgrade sets built successfully");
}

ValidationResult LoadoutValidator::validatePlayer(fb::ServerPlayer* player) {
    ValidationResult result;

    if (!player) {
        result.addFlag(ValidationFlag::NullPointer);
        return result;
    }

    const char* playerName = player->m_name ? player->m_name : "<unknown>";
    result.playerName = playerName;
    result.personaId = player->getPlayerId();

    fb::Asset* customizationAsset = player->getPVZCharacterCustomizationAsset();
    if (!customizationAsset || !customizationAsset->Name) {
        g_program->GetServer()->GetAnticheat()->AC_LogMessage(LogLevel::Error, "Player {} has no customization asset", playerName);
        result.addFlag(ValidationFlag::NullPointer);
        return result;
    }
    const uint32_t kitHash = fnvHash(cutPath(customizationAsset->Name));

    g_program->GetServer()->GetAnticheat()->AC_LogMessage(LogLevel::Info, "Player {} kit: {} (hash: {}, weaponSets size: {})", playerName, customizationAsset->Name, kitHash, weaponSets.size());

    fb::PVZCharacterServerPlayerExtent* extent = player->getPVZCharacterServerPlayerExtent();

    fb::Asset* characterBlueprint = player->getPVZCharacterBlueprint();
    if (!characterBlueprint)
    {
        g_program->GetServer()->GetAnticheat()->AC_LogMessage(LogLevel::Error, "Player {} has no character blueprint, what?", playerName);
    }

    result.teamId = player->getTeamId();
    result.teamName = normalizeTeamName(result.teamId);
    const uint32_t soldierHash = fnvHash(cutPath(characterBlueprint->Name));
    result.characterName = cutPath(characterBlueprint->Name);

    const bool isPlant = std::strstr(characterBlueprint->Name, "MpPlant_") != nullptr;
    const bool isZombie = std::strstr(characterBlueprint->Name, "MpZombie_") != nullptr;

    if (!isValidTeamForFaction(result.teamId, isPlant, isZombie)) {
        result.addFlag(ValidationFlag::InvalidTeam);
    }

    // find allowed weapon set for this character
    auto it = weaponSets.find(kitHash);

    WeaponSet allowedWeapons;
    result.invalidSoldier = characterBlueprint->Name;
    if (it == weaponSets.end()) {
        g_program->GetServer()->GetAnticheat()->AC_LogMessage(LogLevel::Error, "Kit hash {} not found in weaponSets ({} entries)", kitHash, weaponSets.size());
        result.addFlag(ValidationFlag::InvalidSoldier);
    }
    else {
        allowedWeapons = it->second;

        // enforce mode restriction for special kits
        if (!allowedWeapons.requiredMode.empty())
        {
            std::string currentMode = g_program->GetServer()->GetSideChannel()->GetServerInfo().mode;
            if (currentMode != allowedWeapons.requiredMode)
            {
                g_program->GetServer()->GetAnticheat()->AC_LogMessage(LogLevel::Error,
                    "Player {} kit {} requires mode {} but server is running {}", playerName, customizationAsset->Name, allowedWeapons.requiredMode, currentMode);
                result.addFlag(ValidationFlag::InvalidSoldier);
            }
        }

        // if the kit specifies allowed soldier blueprints, enforce them
        if (!allowedWeapons.allowedSoldierSubstrings.empty() && characterBlueprint && characterBlueprint->Name)
        {
            bool soldierAllowed = false;
            for (const auto& sub : allowedWeapons.allowedSoldierSubstrings)
            {
                if (std::strstr(characterBlueprint->Name, sub.c_str()) != nullptr)
                {
                    soldierAllowed = true;
                    break;
                }
            }
            if (!soldierAllowed)
            {
                g_program->GetServer()->GetAnticheat()->AC_LogMessage(LogLevel::Error,
                    "Player {} soldier {} not allowed for kit {}", playerName, characterBlueprint->Name, customizationAsset->Name);
                result.addFlag(ValidationFlag::InvalidSoldier);
            }
        }
    }

    // validate all weapons and upgrades
    ValidationResult weaponValidation = validateWeapons(extent, allowedWeapons, playerName);

    if (weaponValidation.weaponName.empty() && allowedWeapons.primaryNames.size() == 1) {
        weaponValidation.weaponName = allowedWeapons.primaryNames.front();
    }

    // merge weapon validation results to later return it
    result.flags |= weaponValidation.flags;
    if (!weaponValidation.isValid) {
        result.isValid = false;
    }

    if (!weaponValidation.invalidPrimary.empty()) {
        result.invalidPrimary = weaponValidation.invalidPrimary;
    }
    if (!weaponValidation.invalidAbility1.empty()) {
        result.invalidAbility1 = weaponValidation.invalidAbility1;
    }
    if (!weaponValidation.invalidAbility2.empty()) {
        result.invalidAbility2 = weaponValidation.invalidAbility2;
    }
    if (!weaponValidation.invalidAbility3.empty()) {
        result.invalidAbility3 = weaponValidation.invalidAbility3;
    }
    if (!weaponValidation.invalidAlternate.empty()) {
        result.invalidAlternate = weaponValidation.invalidAlternate;
    }
    if (!weaponValidation.weaponName.empty()) {
        result.weaponName = weaponValidation.weaponName;
    }
    if (!weaponValidation.invalidUpgrades.empty()) {
        result.invalidUpgrades = weaponValidation.invalidUpgrades;
    }

    return result;
}
#endif
