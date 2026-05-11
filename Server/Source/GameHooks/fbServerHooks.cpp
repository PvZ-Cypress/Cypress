#include "pch.h"
#include "fbServerHooks.h"

#include <string>
#include <thread>
#include <intrin.h>
#include <Cypress/Core/Program.h>
#include <Cypress/Core/Logging.h>
#include <Cypress/Core/Console/ConsoleFunctions.h>
#include <fb/Engine/Server.h>
#include <fb/Engine/ServerGameContext.h>

#ifdef CYPRESS_GW2
#include "Anticheat/LoadoutValidator.h"
#endif

#ifdef CYPRESS_BFN
#include <fb/Engine/ScriptContext.h>
#include <fb/SecureReason.h>
#endif

#ifdef CYPRESS_BFN
static constexpr int MAX_NAME_LENGTH = 32;
#else
static constexpr int MAX_NAME_LENGTH = 16;
#endif

// raw byte check before we touch the name as a std::string
static bool IsNameSafeRaw(const char* s)
{
	if (!s)
		return false;
	int len = 0;
	for (const char* p = s; *p; ++p)
	{
		unsigned char c = (unsigned char)*p;
		if (c < 0x20 || c >= 0x7F)
			return false;
		if (++len > MAX_NAME_LENGTH)
			return false;
	}
	return len > 0;
}

// strip non-ascii so fmt doesnt throw on garbage names
static std::string SanitizeForLog(const char* s)
{
	std::string out;
	for (const char* p = s; *p && (p - s) < 64; ++p)
	{
		unsigned char c = (unsigned char)*p;
		if (c >= 0x20 && c < 0x7F)
			out += (char)c;
		else
			out += '?';
	}
	return out;
}

static const std::vector<std::string>& GetSlurList()
{
	static const std::vector<std::string> slurs = {
		"nigger", "nigga", "faggot", "fag", "dyke", "kike", "tranny", "troon"
	};
	return slurs;
}

static std::string ToLowerStr(const std::string& s)
{
	std::string out = s;
	for (auto& c : out) c = (char)tolower((unsigned char)c);
	return out;
}

#ifdef CYPRESS_BFN
#endif

static bool IsNameValid(const std::string& name, std::string& reason)
{
	if (name.empty())
	{
		reason = "name is empty";
		return false;
	}
	if (name.length() > MAX_NAME_LENGTH)
	{
		reason = "name too long (max 16)";
		return false;
	}

	for (char c : name)
	{
		if (!((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') ||
			c == ' ' || c == '-' || c == '_' || c == '!' || c == '?'))
		{
			reason = "invalid character in name";
			return false;
		}
	}

	// block ID_ because of idiots using long strings to cover the screen 
	char blockIdBuf[8] = {};
	if (GetEnvironmentVariableA("CYPRESS_BLOCK_ID_NAMES", blockIdBuf, sizeof(blockIdBuf)) > 0
		&& strcmp(blockIdBuf, "1") == 0)
	{
		if (name.length() >= 3 && name[0] == 'I' && name[1] == 'D' && name[2] == '_')
		{
			reason = "names starting with ID_ are blocked";
			return false;
		}
	}

	std::string lower = ToLowerStr(name);
	for (const auto& slur : GetSlurList())
	{
		if (lower.find(slur) != std::string::npos)
		{
			reason = "inappropriate name";
			return false;
		}
	}

	return true;
}

#if(HAS_DEDICATED_SERVER)
#ifndef CYPRESS_BFN
DEFINE_HOOK(
	fb_Server_start,
	__fastcall,
	__int64,

	void* thisPtr,
	fb::ServerSpawnInfo* info,
	Kyber::ServerSpawnOverrides* spawnOverrides
)
{
#if(HAS_DEDICATED_SERVER)
	if (!g_program->GetInitialized())
	{
		bool wsaInit = g_program->InitWSA();
		CYPRESS_ASSERT(wsaInit, "WSA failed to initialize!");
		g_program->SetInitialized(true);

		// Start side-channel TCP listener for server
		if (g_program->IsServer())
			g_program->GetServer()->StartSideChannel();
	}

#endif
	return Orig_fb_Server_start(thisPtr, info, spawnOverrides);
}

#else

DEFINE_HOOK(
	fb_Server_start,
	__fastcall,
	__int64,

	void* thisPtr,
	fb::ServerSpawnInfo* info,
	Kyber::ServerSpawnOverrides* spawnOverrides,
	Kyber::SocketManager* socketManager
)
{
#if(HAS_DEDICATED_SERVER)
	if (!g_program->GetInitialized())
	{
		bool wsaInit = g_program->InitWSA();
		CYPRESS_ASSERT(wsaInit, "WSA failed to initialize!");
		g_program->SetInitialized(true);

		// Start side-channel TCP listener for server
		if (g_program->IsServer())
			g_program->GetServer()->StartSideChannel();
	}

	static bool startedBfnMetadataThread = false;
	if (g_program->IsServer() && !startedBfnMetadataThread)
	{
		startedBfnMetadataThread = true;
		std::thread([]()
		{
			while (g_program && g_program->IsServer())
			{
				Cypress::Server* server = g_program->GetServer();
				if (server)
					server->TickPlayerMetadata();
				Sleep(1000);
			}
		}).detach();
	}

#endif
	return Orig_fb_Server_start(thisPtr, info, spawnOverrides, socketManager);
}
#endif

#ifdef CYPRESS_BFN
DEFINE_HOOK(
	fb_Server_update,
	__fastcall,
	bool,

	void* thisPtr,
	void* params
)
{
#if(HAS_DEDICATED_SERVER)
	bool updated = Orig_fb_Server_update(thisPtr, params);
	if (g_program->IsServer())
	{
		Cypress::Server* server = g_program->GetServer();
		server->UpdateStatus(thisPtr, (float)ptrread<int>(params, 0x18) * 0.000000001);
		server->TickPlayerMetadata();
		
		bool statusUpdated = !server->GetStatusUpdated();
		server->SetStatusUpdated(true);
		if (statusUpdated)
		{
			if (g_program->IsEmbedded())
			{
				Cypress_EmitJsonStatus("", "", server->GetStatusColumn1().c_str(), server->GetStatusColumn2().c_str());
			}
			else
			{
				PostMessageA(*server->GetMainWindow(), WM_APP_UPDATESTATUS, 0, 0);
			}
		}
	}
	return updated;
#else
	Orig_fb_Server_update(thisPtr, params);
#endif
}
#else

DEFINE_HOOK(
	fb_Server_update,
	__fastcall,
	bool,

	void* thisPtr,
	void* params
)
{
#if(HAS_DEDICATED_SERVER)
	bool updated = Orig_fb_Server_update(thisPtr, params);
	if (g_program->IsServer())
	{
		Cypress::Server* server = g_program->GetServer();
		server->UpdateStatus(thisPtr, ptrread<float>(params, CYPRESS_GW_SELECT(0x18, 0x28, 0)));
		server->TickPlayerMetadata();

		bool statusUpdated = !server->GetStatusUpdated();
		server->SetStatusUpdated(true);
		if (statusUpdated)
		{
			if (g_program->IsEmbedded())
			{
				Cypress_EmitJsonStatus("", "", server->GetStatusColumn1().c_str(), server->GetStatusColumn2().c_str());
			}
			else
			{
				PostMessageA(*server->GetMainWindow(), WM_APP_UPDATESTATUS, 0, 0);
			}
		}
	}
	return updated;
#else
	Orig_fb_Server_update(thisPtr, params);
#endif
}

std::string g_commandBoxCommand;

DEFINE_HOOK(
	fb_editBoxWndProcProxy,
	__cdecl,
	LRESULT,

	HWND hWnd,
	UINT msg,
	WPARAM wParam,
	LPARAM lParam
)
{
	switch (msg)
	{
	case WM_KEYDOWN:
		switch (wParam)
		{
		case VK_RETURN:
		case VK_TAB:
			char buf[1024];
			int cmdLen = GetWindowTextA(hWnd, buf, sizeof(buf));
			g_commandBoxCommand = buf;
			break;
		}
	}

	LRESULT ret = Orig_fb_editBoxWndProcProxy(hWnd, msg, wParam, lParam);

	switch (msg)
	{
	case WM_KEYDOWN:
		switch (wParam)
		{
		case VK_RETURN:
			CYPRESS_LOGTOSERVER(LogLevel::Info, "{}", g_commandBoxCommand.c_str());
			break;
		}
	}

	return ret;
}
#endif

#ifdef CYPRESS_BFN
DEFINE_HOOK(
	fb_windowProcedure,
	__stdcall,
	__int64,

	HWND hWnd,
	UINT msg,
	WPARAM wParam,
	LPARAM lParam
)
{
	if (g_program->IsServer())
	{
		Cypress::Server* pServer = g_program->GetServer();
		HWND commandBox = pServer->GetCommandBox();

		switch (msg)
		{
		case WM_SIZE:
		{
			if (commandBox != NULL)
			{
				RECT rect;
				GetClientRect(*pServer->GetMainWindow(), &rect);

				MoveWindow(pServer->GetListBox(), 0, 0, rect.right, rect.bottom - 88, 1);
				MoveWindow(commandBox, 0, rect.bottom - 88, rect.right, 17, 1);
				MoveWindow(pServer->GetToggleLogButtonBox(), rect.right - 80, rect.bottom - 16, 80, 16, 1);

				int index = 0, width = 0;
				HWND* currentBox = pServer->GetStatusBox();

				do
				{
					if (index == 4)
						width = rect.right - 800;
					else
						width = 200;
					MoveWindow(*currentBox++, 200 * index++, rect.bottom - 71, width, 71, 1);

				} while (index < 5);
			}
			return DefWindowProcW(hWnd, msg, wParam, lParam);
		}
		default: break;
		}

		if (msg == WM_APP_UPDATESTATUS && commandBox)
		{
			HWND* g_statusBox = pServer->GetStatusBox();
			SetWindowTextA(*g_statusBox, pServer->GetStatusColumn1().c_str());
			SetWindowTextA(*++g_statusBox, pServer->GetStatusColumn2().c_str());
			return DefWindowProcA(hWnd, msg, wParam, lParam);
		}
		else if (msg == WM_COMMAND)
		{
			if (HIWORD(wParam) == BN_CLICKED && (HWND)lParam == pServer->GetToggleLogButtonBox())
			{
				if (!pServer->GetServerLogEnabled())
				{
					pServer->SetServerLogEnabled(true);
					CYPRESS_LOGTOSERVER(LogLevel::Info, "Log window enabled.");
					SetWindowTextA(pServer->GetToggleLogButtonBox(), "Disable Logs");
					PostMessageA(*pServer->GetMainWindow(), WM_APP_UPDATESTRINGLIST, 0, 0);
				}
				else
				{
					CYPRESS_LOGTOSERVER(LogLevel::Info, "Log window disabled.");
					pServer->SetServerLogEnabled(false);
					SetWindowTextA(pServer->GetToggleLogButtonBox(), "Enable Logs");
				}
				return DefWindowProcA(hWnd, msg, wParam, lParam);
			}
		}
	}
	return Orig_fb_windowProcedure(hWnd, msg, wParam, lParam);
}

DEFINE_HOOK(
	fb_ServerPVZRoundControl_event,
	__fastcall,
	void,

	void* thisPtr,
	fb::EntityEvent* event
)
{
	Orig_fb_ServerPVZRoundControl_event(thisPtr, event);

	if (g_program->IsServer())
	{
		Cypress::Server* server = g_program->GetServer();
	}

	if (event->eventId == 0xDB4B330) //Reset
	{
		Cypress::Server* pServer = g_program->GetServer();

		if (pServer->IsUsingPlaylist())
		{
			fb::LevelSetup nextLevelSetup;

			const auto nextSetup = pServer->GetServerPlaylist()->GetNextSetup();
			pServer->LevelSetupFromPlaylistSetup(&nextLevelSetup, nextSetup);
			pServer->ApplySettingsFromPlaylistSetup(nextSetup);

			fb::PostServerLoadLevelMessage(&nextLevelSetup, true, false);
		}
	}
}
#else
DEFINE_HOOK(
	fb_windowProcedure,
	__stdcall,
	__int64,

	HWND hWnd,
	UINT msg,
	WPARAM wParam,
	LPARAM lParam
)
{
	if (g_program->IsServer() && !g_program->IsHeadless())
	{
		HWND commandBox = *g_program->GetServer()->GetCommandBox();
		if (msg == WM_APP_UPDATESTATUS && commandBox)
		{
			HWND* g_statusBox = (HWND*)OFFSET_G_STATUSBOX;
			SetWindowTextA(*g_statusBox, g_program->GetServer()->GetStatusColumn1().c_str());
			SetWindowTextA(*++g_statusBox, g_program->GetServer()->GetStatusColumn2().c_str());
			return DefWindowProcA(hWnd, msg, wParam, lParam);
		}
		else if (msg == WM_COMMAND)
		{
			if (HIWORD(wParam) == BN_CLICKED && (HWND)lParam == *g_program->GetServer()->GetToggleLogButtonBox())
			{
				if (!g_program->GetServer()->GetServerLogEnabled())
				{
					g_program->GetServer()->SetServerLogEnabled(true);
					CYPRESS_LOGTOSERVER(LogLevel::Info, "Log window enabled.");
					SetWindowTextA(*g_program->GetServer()->GetToggleLogButtonBox(), "Disable Logs");
					PostMessageA(*g_program->GetServer()->GetMainWindow(), WM_APP_UPDATESTRINGLIST, 0, 0);
				}
				else
				{
					CYPRESS_LOGTOSERVER(LogLevel::Info, "Log window disabled.");
					g_program->GetServer()->SetServerLogEnabled(false);
					SetWindowTextA(*g_program->GetServer()->GetToggleLogButtonBox(), "Enable Logs");
				}
				return DefWindowProcA(hWnd, msg, wParam, lParam);
			}
		}
	}

	return Orig_fb_windowProcedure(hWnd, msg, wParam, lParam);
}

DEFINE_HOOK(
	fb_ServerPVZLevelControlEntity_loadLevel,
	__fastcall,
	void,

	void* thisPtr,
	const char* level,
	const char* inclusion
)
{
	g_program->GetServer()->SetLoadRequestFromLevelControl(true);
	Orig_fb_ServerPVZLevelControlEntity_loadLevel(thisPtr, level, inclusion);
	g_program->GetServer()->SetLoadRequestFromLevelControl(false);
}

DEFINE_HOOK(
	fb_ServerLevelControlEntity_loadLevel,
	__fastcall,
	void,

	void* thisPtr,
	bool notifyLevelComplete
)
{
	g_program->GetServer()->SetLoadRequestFromLevelControl(true);
	Orig_fb_ServerLevelControlEntity_loadLevel(thisPtr, notifyLevelComplete);
	g_program->GetServer()->SetLoadRequestFromLevelControl(false);
}

DEFINE_HOOK(
	fb_ServerLoadLevelMessage_post,
	__cdecl,
	void,

	fb::LevelSetup* levelSetup,
	bool fadeOut,
	bool forceReloadResources
)
{
	Cypress::Server* server = g_program->GetServer();
	if (server->GetIsLoadRequestFromLevelControl() && server->IsUsingPlaylist())
	{
		const auto nextSetup = server->GetServerPlaylist()->GetNextSetup();
		server->LevelSetupFromPlaylistSetup(levelSetup, nextSetup);
		server->ApplySettingsFromPlaylistSetup(nextSetup);
	}

	// update browser level/mode
	{
		auto* sc = server->GetSideChannel();
		auto info = sc->GetServerInfo();
#ifdef CYPRESS_BFN
		info.level = levelSetup->m_levelManagerInitialLevel.c_str();
#else
		info.level = levelSetup->m_name.c_str();
#endif
		const char* gm = levelSetup->getInclusionOption("GameMode");
		info.mode = gm ? gm : "";
		sc->SetServerInfo(info);
	}

	Orig_fb_ServerLoadLevelMessage_post(levelSetup, fadeOut, forceReloadResources);

#ifdef CYPRESS_GW2
	LoadoutValidator::getInstance().invalidate();
#endif
}
#endif

#ifdef CYPRESS_BFN
DEFINE_HOOK(
	fb_ServerConnection_onCreatePlayerMsg,
	__fastcall,
	void*,

	fb::ServerConnection* thisPtr,
	void* msg
)
{
	const char* playerName = reinterpret_cast<const char*>(reinterpret_cast<uintptr_t>(msg) + 0x6C);

		try
	{
		// reject garbage bytes before constructing any strings
		if (!IsNameSafeRaw(playerName))
		{
			thisPtr->disconnect(fb::SecureReason_KickedOut, "Invalid name");
			return nullptr;
		}

		// validate name first before touching anything else
		std::string nameRejectReason;
		if (!IsNameValid(playerName, nameRejectReason))
		{
			CYPRESS_LOGTOSERVER(LogLevel::Warning, "Kicking {} - {}", playerName, nameRejectReason);
			thisPtr->disconnect(fb::SecureReason_KickedOut, nameRejectReason.c_str());
			return nullptr;
		}

	// grab fingerprint from side channel if they connected, or from hw cache if they connected before
	const Cypress::HardwareFingerprint* fp = nullptr;
	std::string hwid;
	std::string accountId;
	auto* sideChannel = g_program->GetServer()->GetSideChannel();
	auto peer = sideChannel->FindPeerByName(playerName);
	if (peer)
	{
		fp = &peer->fingerprint;
		hwid = peer->hwid;
		accountId = peer->accountId;
	}
	else
	{
		auto& cache = g_program->GetServer()->GetPlayerHwCache();
		auto it = cache.find(playerName);
		if (it != cache.end())
		{
			hwid = it->second.first;
			fp = &it->second.second;
		}
	}

	auto* banlist = g_program->GetServer()->GetServerBanlist();
	if (banlist->IsBanned(playerName, hwid.empty() ? thisPtr->m_machineId.c_str() : hwid.c_str(), fp, accountId.empty() ? nullptr : accountId.c_str()))
	{
		if (fp) banlist->SpreadComponents(*fp, playerName);
		thisPtr->disconnect(fb::SecureReason_Banned, "Banned from server");
		return Orig_fb_ServerConnection_onCreatePlayerMsg(thisPtr, msg);
	}

	// spread components even if not banned (absorbs new hw into existing bans)
	if (fp) banlist->SpreadComponents(*fp, playerName);

	int nameLen = (int)strlen(playerName);
	if (nameLen < 3 || nameLen > 32)
	{
		thisPtr->disconnect(fb::SecureReason_KickedOut, "Invalid Username Length");
	}

	for (const char* p = playerName; *p != '\0'; ++p)
	{
		if (iscntrl(static_cast<unsigned char>(*p)))
		{
			thisPtr->disconnect(fb::SecureReason_KickedOut, "Invalid Characters in Username");
			break;
		}
	}

	static const char* bannedChars = "\t\n!\"#$%&'()*+,./:;<=>?@[\\]^`{|}~";
	if (strpbrk(playerName, bannedChars) != nullptr)
	{
		thisPtr->disconnect(fb::SecureReason_KickedOut, "Invalid Characters in Username");
	}


	CYPRESS_LOGTOSERVER(LogLevel::Info, "{} is trying to join from machine {}", playerName, thisPtr->m_machineId.c_str());
	return Orig_fb_ServerConnection_onCreatePlayerMsg(thisPtr, msg);

	}
	catch (...)
	{
		thisPtr->disconnect(fb::SecureReason_KickedOut, "Internal error");
		return nullptr;
	}
}
#else
DEFINE_HOOK(
	fb_ServerConnection_onCreatePlayerMsg,
	__fastcall,
	void*,

	fb::ServerConnection* thisPtr,
	void* msg
)
{
	const char* playerName = ptrread<const char*>(msg, 0x48);

		try
	{
		// reject garbage bytes before constructing any strings
		if (!IsNameSafeRaw(playerName))
		{
			thisPtr->m_shouldDisconnect = true;
			thisPtr->m_disconnectReason = 0x4;
			thisPtr->m_reasonText = "Invalid name";
			return nullptr;
		}

		// validate name first before touching anything else
		std::string nameRejectReason;
		if (!IsNameValid(playerName, nameRejectReason))
		{
			CYPRESS_LOGTOSERVER(LogLevel::Warning, "Kicking {} - {}", playerName, nameRejectReason);
			thisPtr->m_shouldDisconnect = true;
			thisPtr->m_disconnectReason = 0x4;
			thisPtr->m_reasonText = nameRejectReason.c_str();
			return nullptr;
		}

	// grab fingerprint from side channel if they connected, or from hw cache if they connected before
	const Cypress::HardwareFingerprint* fp = nullptr;
	std::string hwid;
	std::string accountId;
	auto* sideChannel = g_program->GetServer()->GetSideChannel();
	auto peer = sideChannel->FindPeerByName(playerName);
	if (peer)
	{
		fp = &peer->fingerprint;
		hwid = peer->hwid;
		accountId = peer->accountId;
	}
	else
	{
		auto& cache = g_program->GetServer()->GetPlayerHwCache();
		auto it = cache.find(playerName);
		if (it != cache.end())
		{
			hwid = it->second.first;
			fp = &it->second.second;
		}
	}

	auto* banlist = g_program->GetServer()->GetServerBanlist();
	if (banlist->IsBanned(playerName, hwid.empty() ? thisPtr->m_machineId.c_str() : hwid.c_str(), fp, accountId.empty() ? nullptr : accountId.c_str()))
	{
		if (fp) banlist->SpreadComponents(*fp, playerName);
		thisPtr->m_shouldDisconnect = true;
		thisPtr->m_disconnectReason = 0x4;
		thisPtr->m_reasonText = "Banned from server";
		return Orig_fb_ServerConnection_onCreatePlayerMsg(thisPtr, msg);
	}

	if (fp) banlist->SpreadComponents(*fp, playerName);

	int nameLen = strlen(playerName);
	if (nameLen < 3 || nameLen > 32)
	{
		thisPtr->m_shouldDisconnect = true;
		thisPtr->m_disconnectReason = 0x4;
		thisPtr->m_reasonText = "Invalid Username Length";
	}

	for (const char* p = playerName; *p != '\0'; ++p)
	{
		if (iscntrl(static_cast<unsigned char>(*p)))
		{
			thisPtr->m_shouldDisconnect = true;
			thisPtr->m_disconnectReason = 0x4;
			thisPtr->m_reasonText = "Invalid Characters in Username";
			break;
		}
	}

	auto* gc = fb::ServerGameContext::GetInstance();
	if (gc && gc->m_serverPlayerManager && gc->m_serverPlayerManager->findHumanByName(playerName))
	{
		CYPRESS_LOGTOSERVER(LogLevel::Warning, "Kicking {} - name already in use", playerName);
		thisPtr->m_shouldDisconnect = true;
		thisPtr->m_disconnectReason = 0x4;
		thisPtr->m_reasonText = "Name already in use";
	}

	CYPRESS_LOGTOSERVER(LogLevel::Info, "{} is trying to join from machine {}", playerName, thisPtr->m_machineId.c_str());
	return Orig_fb_ServerConnection_onCreatePlayerMsg(thisPtr, msg);
	}
	catch (...)
	{
		thisPtr->m_shouldDisconnect = true;
		thisPtr->m_disconnectReason = 0x4;
		thisPtr->m_reasonText = "Internal error";
		return nullptr;
	}
}
#endif

DEFINE_HOOK(
	fb_ServerPlayerManager_addPlayer,
	__fastcall,
	void*,

	fb::ServerPlayerManager* thisPtr,
	fb::ServerPlayer* player,
	const char* nickname
)
{
	if (!player->isAIPlayer() && nickname && nickname[0] != '\0')
	{
#if(HAS_DEDICATED_SERVER)
		if (g_program->IsServer())
			g_program->GetServer()->ClearPlayerMetadata(nickname, player->getPlayerId());
#endif
		CYPRESS_LOGTOSERVER(LogLevel::Info, "[Id: {}] {} has joined the server", player->getPlayerId(), nickname);
		if (Cypress_IsEmbeddedMode())
			Cypress_EmitJsonPlayerEvent("playerJoin", player->getPlayerId(), nickname);

		// Notify moderator clients via side-channel
#if(HAS_DEDICATED_SERVER)
		if (g_program->IsServer())
		{
			g_program->GetServer()->GetSideChannel()->Broadcast(
				{ {"type", "scPlayerJoin"}, {"id", player->getPlayerId()}, {"name", nickname} });

			// kick if they don't authenticate via side-channel within 15s
			// skip if proxied and tunnel isn't connected (can't reach us)
			bool isProxied = std::getenv("CYPRESS_PROXY_ADDRESS") != nullptr;
			bool tunnelUp = g_program->GetServer()->GetSideChannelTunnel()->IsRunning();
			if (!isProxied || tunnelUp)
			{
				std::string name(nickname);
				std::thread([name]() {
					Sleep(15000);
					if (!g_program->IsServer()) return;
					auto* sc = g_program->GetServer()->GetSideChannel();
					if (sc->HasPeerByName(name)) return;

					// if proxied, recheck tunnel, don't kick if tunnel dropped
					if (std::getenv("CYPRESS_PROXY_ADDRESS") &&
						!g_program->GetServer()->GetSideChannelTunnel()->IsRunning())
						return;

					auto* gc = fb::ServerGameContext::GetInstance();
					if (!gc || !gc->m_serverPlayerManager || !gc->m_serverPeer) return;
					auto* p = gc->m_serverPlayerManager->findHumanByName(name.c_str());
					if (!p) return;
					auto* conn = gc->m_serverPeer->connectionForPlayer(p);
					if (!conn) return;

					CYPRESS_LOGTOSERVER(LogLevel::Warning, "Kicking {} - no side-channel auth after 15s", name);
					conn->m_shouldDisconnect = true;
#ifdef CYPRESS_BFN
					conn->m_disconnectReason = fb::SecureReason_KickedOut;
					conn->m_reasonText = "Side-channel authentication required";
#else
					conn->m_disconnectReason = 0x4;
					conn->m_reasonText = "Side-channel authentication required";
#endif
				}).detach();
			}
		}
#endif
	}

#ifdef CYPRESS_GW2
	if (LoadoutValidator::getInstance().needsInit())
		LoadoutValidator::getInstance().init();
#endif

	void* result = Orig_fb_ServerPlayerManager_addPlayer(thisPtr, player, nickname);

#ifdef CYPRESS_BFN
#if(HAS_DEDICATED_SERVER)
	if (g_program->IsServer() && !player->isAIPlayer() && nickname && nickname[0] != '\0')
	{
		Cypress::Server* server = g_program->GetServer();
		server->TickPlayerMetadata();

		std::string joinedName(nickname);
		std::thread([joinedName]()
		{
			Sleep(2000);
			if (!g_program || !g_program->IsServer() || !g_program->GetServer())
				return;

			g_program->GetServer()->TickPlayerMetadata();
		}).detach();
	}
#endif
#endif

	return result;
}

#ifdef CYPRESS_BFN
DEFINE_HOOK(
	fb_ServerPlayer_disconnect,
	__fastcall,
	void,

	fb::ServerPlayer* thisPtr,
	fb::SecureReason reason,
	eastl::new_string* reasonText
)
{
	const char* reasonStr = "None provided";

	if (!reasonText->empty())
		reasonStr = reasonText->c_str();

	if (!thisPtr->isAIPlayer() && thisPtr->m_name && thisPtr->m_name[0] != '\0')
	{
#if(HAS_DEDICATED_SERVER)
		if (g_program->IsServer())
			g_program->GetServer()->ClearPlayerMetadata(thisPtr->m_name, thisPtr->getPlayerId());
#endif
		CYPRESS_LOGTOSERVER(LogLevel::Info, "[Id: {}] {} has left the server (Reason: {}, {})",
			thisPtr->getPlayerId(),
			thisPtr->m_name,
			reasonStr,
			fb::SecureReason_toString(reason));

		if (Cypress_IsEmbeddedMode())
			Cypress_EmitJsonPlayerEvent("playerLeave", thisPtr->getPlayerId(), thisPtr->m_name, reasonStr);

		// Notify moderator clients via side-channel
#if(HAS_DEDICATED_SERVER)
		if (g_program->IsServer())
		{
			g_program->GetServer()->GetSideChannel()->Broadcast(
				{ {"type", "scPlayerLeave"}, {"id", thisPtr->getPlayerId()}, {"name", std::string(thisPtr->m_name)} });
		}
#endif
	}

	Orig_fb_ServerPlayer_disconnect(thisPtr, reason, reasonText);
}
#else
DEFINE_HOOK(
	fb_ServerPlayer_disconnect,
	__fastcall,
	void,

	fb::ServerPlayer* thisPtr,
	fb::SecureReason reason,
	eastl::string& reasonText
)
{
	const char* reasonStr = reasonText.empty() ? "None provided" : reasonText.c_str();

	if (!thisPtr->isAIPlayer() && thisPtr->m_name && thisPtr->m_name[0] != '\0')
	{
#if(HAS_DEDICATED_SERVER)
		if (g_program->IsServer())
			g_program->GetServer()->ClearPlayerMetadata(thisPtr->m_name, thisPtr->getPlayerId());
#endif
		CYPRESS_LOGTOSERVER(LogLevel::Info, "[Id: {}] {} has left the server (Reason: {}, {})",
			thisPtr->getPlayerId(),
			thisPtr->m_name,
			reasonStr,
			fb::SecureReason_toString(reason));

		if (Cypress_IsEmbeddedMode())
			Cypress_EmitJsonPlayerEvent("playerLeave", thisPtr->getPlayerId(), thisPtr->m_name, reasonStr);

		// Notify moderator clients via side-channel
#if(HAS_DEDICATED_SERVER)
		if (g_program->IsServer())
		{
			g_program->GetServer()->GetSideChannel()->Broadcast(
				{ {"type", "scPlayerLeave"}, {"id", thisPtr->getPlayerId()}, {"name", std::string(thisPtr->m_name)} });
		}
#endif
	}

	Orig_fb_ServerPlayer_disconnect(thisPtr, reason, reasonText);
}

DEFINE_HOOK(
	fb_sub_140112AA0,
	__fastcall,
	__int64,

	void* a1,
	int a2
)
{
	if (g_program->IsServer())
		a2 = 0;
	return Orig_fb_sub_140112AA0(a1, a2);
}
#endif
#endif
