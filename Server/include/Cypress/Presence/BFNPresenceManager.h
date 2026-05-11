#pragma once
#include <Cypress/Presence/PresenceManager.h>
#include <fb/Engine/MessageListener.h>
#include <EASTL/string.h>
#include <EASTL/map.h> 
#include <fb/Engine/Message.h>
#include <fb/Engine/TypeInfo.h>

namespace fb
{
	enum class PVZRecordInfoType
	{
		Bool = 0,
		String = 1,
		Integer = 2,
		Float = 3,
		Unknown = 5
	};

	struct PVZRecordInfo
	{
		eastl::string String;
		union
		{
			bool Bool;
			float Float;
			unsigned int Int;
		};
		PVZRecordInfoType Type;
		bool UnkBool;

		PVZRecordInfo() : Int(0), Type(PVZRecordInfoType::Unknown), UnkBool(false) {}
	};

	class PVZPresenceUpdateByteVaultRecordResultMessageBase : public fb::Message {
	public:
		void* unk1;
		eastl::string m_subCategorie;
		eastl::map<eastl::string, PVZRecordInfo&>* m_records;
	};

	class BlazeClientBytevaultManager {
	public:
		static BlazeClientBytevaultManager* instance() { return *reinterpret_cast<BlazeClientBytevaultManager**>(0x14464D210); }
	};

	struct ItemLoadout
	{
		uint32_t categoryId;
		uint32_t itemId;
	};

	struct OwnedItem
	{
		GUID unlockGuid;
		void* offer;
	};

	struct PlayerInventory
	{
		eastl::map<uint32_t, eastl::map<int, eastl::vector<ItemLoadout>>> customizationLoadouts;
		eastl::vector<GUID> socialBoardFavorites;
		GUID selectedTombstone;
		GUID selectedTombstoneLeftPuncher;
		GUID selectedTombstoneRightPuncher;
		eastl::vector<OwnedItem> ownedItems;
		int64_t unk1;
		bool unk2;
		bool loadoutsLoaded;
	};

	class PVZBlazeClientInventoryManager {
	public:
		void** vftptr;
		char pad[0x18];
		eastl::vector<PlayerInventory> m_inventoryPerPlayer; //2 by default

		static PVZBlazeClientInventoryManager* instance() { return *reinterpret_cast<PVZBlazeClientInventoryManager**>(0x144658DB0); }
	};

}

namespace Cypress
{
	class BFNPresenceManager : public PresenceManager, public fb::MessageListener {
	public:
		BFNPresenceManager();
		void Initialize() override;

	protected:
		void InitializePresenceState() {};

	private:
		void LoadInventoryData();
		void SaveInventory();

		void onMessage(fb::Message& inMessage) override;
		uint16_t ordering() const override	{ return 100; }
	};
}
