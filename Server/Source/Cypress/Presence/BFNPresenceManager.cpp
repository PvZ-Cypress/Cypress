#include <Cypress/Presence/BFNPresenceManager.h>
#include <fb/Engine/ClientGameContext.h>

#include <Cypress/Core/Program.h>

namespace Cypress
{
	BFNPresenceManager::BFNPresenceManager()
	{
		fb::ClientGameContext* ctx = fb::ClientGameContext::GetInstance();

		//inventory
		ctx->m_messageManager.registerMessageListener(fnvHashConstexpr("PVZNetwork"), this);
		ctx->m_messageManager.registerMessageListener(fnvHashConstexpr("PVZUI"), this);
		ctx->m_messageManager.registerMessageListener(fnvHashConstexpr("PVZTelemetry"), this);

		//records
		ctx->m_messageManager.registerMessageListener(fnvHashConstexpr("RimeUISystem"), this);
		ctx->m_messageManager.registerMessageListener(fnvHashConstexpr("PVZPresence"), this);
	}

	void BFNPresenceManager::Initialize()
	{
		LoadSaveFile();
		LoadInventoryData();
	}

	void BFNPresenceManager::onMessage(fb::Message& message)
	{
		switch (message.m_type)
		{
			case fnvHashConstexpr("PVZNetworkSpawnSelectedCharacterResultMessage"):
			case fnvHashConstexpr("PVZUISocialBoardFavoritesUpdatedMessage"):
			case fnvHashConstexpr("PVZTelemetryTombstoneCustomizedMessage"):
				SaveInventory();
				break;
			case fnvHashConstexpr("RimeUISystemPostInitCompleteMessage"):
			{
#ifdef CYPRESS_BFN
				g_program->GetClient()->AddPrimaryUser();
#endif
				using tOnGetByteVaultRecord = void (*)(fb::BlazeClientBytevaultManager*, bool, const char*, int*, bool, unsigned int);
				auto OnGetBVR = reinterpret_cast<tOnGetByteVaultRecord>(0x1415AB060);

				std::string json = m_saveFile.dump();
				int charCount = json.size();
				OnGetBVR(fb::BlazeClientBytevaultManager::instance(), true, json.c_str(), &charCount, false, 0);

				break;
			}
			case fnvHashConstexpr("PVZPresenceUpdateByteVaultRecordResultMessageBase"):
			{
				auto& updateBv = static_cast<fb::PVZPresenceUpdateByteVaultRecordResultMessageBase&>(message);

				CYPRESS_DEBUG_LOGMESSAGE(LogLevel::Info, "Updating records for category: {}", updateBv.m_subCategorie.c_str());

				for (auto it = updateBv.m_records->begin(); it != updateBv.m_records->end(); it++)
				{
					std::string kName = std::format("{}.{}", updateBv.m_subCategorie.c_str(), it->first.c_str());

					switch (it->second.Type)
					{
						case fb::PVZRecordInfoType::Bool:
							m_saveFile[kName] = it->second.Bool;
							break;
						case fb::PVZRecordInfoType::String:
							m_saveFile[kName] = it->second.String.c_str();
							break;
						case fb::PVZRecordInfoType::Integer:
							m_saveFile[kName] = it->second.Int;
							break;
						case fb::PVZRecordInfoType::Float:
							m_saveFile[kName] = it->second.Float;
							break;
						default:
							break;
					}
				}

				std::ofstream saveTo("cypsave.json");
				saveTo << m_saveFile.dump(2);
				saveTo.close();
				break;
			}
			default: break;
		}
	}

	void BFNPresenceManager::SaveInventory()
	{
		fb::PlayerInventory& playerInventory = fb::PVZBlazeClientInventoryManager::instance()->m_inventoryPerPlayer[0];
		std::ofstream inventoryFile("cypinventorysave.bin", std::ios::binary);

		if (inventoryFile.is_open())
		{
			CYPRESS_DEBUG_LOGMESSAGE(LogLevel::Info, "Saving inventory to save data");

			int numClasses = playerInventory.customizationLoadouts.size();
			inventoryFile.write(reinterpret_cast<char*>(&numClasses), 4);
			int numFavSocialBoardItems = playerInventory.socialBoardFavorites.size();
			inventoryFile.write(reinterpret_cast<char*>(&numFavSocialBoardItems), 4);

			for (auto it = playerInventory.customizationLoadouts.begin(); it != playerInventory.customizationLoadouts.end(); it++)
			{
				int classId = it->first;
				inventoryFile.write(reinterpret_cast<char*>(&classId), 4);
				int numClassLoadouts = it->second.size();
				inventoryFile.write(reinterpret_cast<char*>(&numClassLoadouts), 4);

				for (auto loadoutIt = it->second.begin(); loadoutIt != it->second.end(); loadoutIt++)
				{
					int loadoutIndex = loadoutIt->first;
					inventoryFile.write(reinterpret_cast<char*>(&loadoutIndex), 4);

					auto& savedCategories = loadoutIt->second;
					int numOfCategories = savedCategories.size();
					inventoryFile.write(reinterpret_cast<char*>(&numOfCategories), 4);

					for (const auto& categorie : savedCategories)
					{
						inventoryFile.write(reinterpret_cast<const char*>(&categorie.categoryId), 4);
						inventoryFile.write(reinterpret_cast<const char*>(&categorie.itemId), 4);
					}
				}
			}

			for (const GUID& itemGuid : playerInventory.socialBoardFavorites)
				inventoryFile.write(reinterpret_cast<const char*>(&itemGuid), sizeof(GUID));

			inventoryFile.write(reinterpret_cast<const char*>(&playerInventory.selectedTombstone), sizeof(GUID));
			inventoryFile.write(reinterpret_cast<const char*>(&playerInventory.selectedTombstoneLeftPuncher), sizeof(GUID));
			inventoryFile.write(reinterpret_cast<const char*>(&playerInventory.selectedTombstoneRightPuncher), sizeof(GUID));
		}

		inventoryFile.close();
	}

	void BFNPresenceManager::LoadInventoryData()
	{
		fb::PlayerInventory& playerInventory = fb::PVZBlazeClientInventoryManager::instance()->m_inventoryPerPlayer[0];
		std::ifstream inventoryFile("cypinventorysave.bin", std::ios::binary);

		if (inventoryFile.is_open())
		{
			CYPRESS_DEBUG_LOGMESSAGE(LogLevel::Info, "Loading inventory from save data");

			int numClasses, numSocialBoardFavItems;
			inventoryFile.read(reinterpret_cast<char*>(&numClasses), 4);
			inventoryFile.read(reinterpret_cast<char*>(&numSocialBoardFavItems), 4);

			for (int i = 0; i < numClasses; i++)
			{
				int classId, loadoutCount;
				inventoryFile.read(reinterpret_cast<char*>(&classId), 4);
				inventoryFile.read(reinterpret_cast<char*>(&loadoutCount), 4);

				auto k = playerInventory.customizationLoadouts.insert(classId);

				for (int j = 0; j < loadoutCount; j++)
				{
					int loadoutIndex, numOfCategories;
					inventoryFile.read(reinterpret_cast<char*>(&loadoutIndex), 4);
					inventoryFile.read(reinterpret_cast<char*>(&numOfCategories), 4);

					auto lk = k.first->second.insert(loadoutIndex);
					for (int l = 0; l < numOfCategories; l++)
					{
						int categoryId, itemId;
						inventoryFile.read(reinterpret_cast<char*>(&categoryId), 4);
						inventoryFile.read(reinterpret_cast<char*>(&itemId), 4);

						fb::ItemLoadout iL{categoryId, itemId};
						lk.first->second.push_back(iL);
					}
				}
			}

			for (int i = 0; i < numSocialBoardFavItems; i++)
			{
				GUID itemGuid;
				inventoryFile.read(reinterpret_cast<char*>(&itemGuid), sizeof(GUID));
				playerInventory.socialBoardFavorites.push_back(itemGuid);
			}

			GUID selectedTombstone, selectedTombstoneLeftPunch, selectedTombstoneRightPunch;
			inventoryFile.read(reinterpret_cast<char*>(&selectedTombstone), sizeof(GUID));
			inventoryFile.read(reinterpret_cast<char*>(&selectedTombstoneLeftPunch), sizeof(GUID));
			inventoryFile.read(reinterpret_cast<char*>(&selectedTombstoneRightPunch), sizeof(GUID));

			memmove(&playerInventory.selectedTombstone, &selectedTombstone, sizeof(GUID));
			memmove(&playerInventory.selectedTombstoneLeftPuncher, &selectedTombstoneLeftPunch, sizeof(GUID));
			memmove(&playerInventory.selectedTombstoneRightPuncher, &selectedTombstoneRightPunch, sizeof(GUID));
		}

		playerInventory.loadoutsLoaded = true;
		inventoryFile.close();
	}

}
