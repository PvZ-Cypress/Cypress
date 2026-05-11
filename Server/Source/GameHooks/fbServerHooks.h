#pragma once
#include <MemUtil.h>
#include <fb/Engine/Server.h>
#include <fb/Engine/LevelSetup.h>
#include <fb/Engine/ServerPlayer.h>
#include <fb/Engine/ServerPlayerManager.h>
#include <fb/Engine/ServerConnection.h>
#include <fb/SecureReason.h>
#include <Kyber/SocketManager.h>

#ifdef CYPRESS_BFN
#include <fb/Engine/Schematics.h>
#define OFFSET_FB_SERVERPVZROUNDCONTROL_EVENT 0x141E34DB0
#else
#define OFFSET_FB_SERVERPVZLEVELCONTROLENTITY_LOADLEVEL CYPRESS_GW_SELECT(0x14078C930, 0x140FB4210, 0)
#endif

#if(HAS_DEDICATED_SERVER)

#ifndef CYPRESS_BFN
DECLARE_HOOK(
	fb_Server_start,
	__fastcall,
	__int64,

	void* thisPtr,
	fb::ServerSpawnInfo* info,
	Kyber::ServerSpawnOverrides* spawnOverrides
);
#else
DECLARE_HOOK(
	fb_Server_start,
	__fastcall,
	__int64,

	void* thisPtr,
	fb::ServerSpawnInfo* info,
	Kyber::ServerSpawnOverrides* spawnOverrides,
	Kyber::SocketManager* socketManager
);
#endif

DECLARE_HOOK(
	fb_Server_update,
	__fastcall,
	bool,

	void* thisPtr,
	void* params
);

#ifndef CYPRESS_BFN
DECLARE_HOOK(
	fb_editBoxWndProcProxy,
	__cdecl,
	LRESULT,

	HWND hWnd,
	UINT msg,
	WPARAM wParam,
	LPARAM lParam
);
#endif

DECLARE_HOOK(
	fb_windowProcedure,
	__stdcall,
	__int64,

	HWND hWnd,
	UINT msg,
	WPARAM wParam,
	LPARAM lParam
);

#ifdef CYPRESS_BFN
DECLARE_HOOK(
	fb_ServerPVZRoundControl_event,
	__fastcall,
	void,

	void* thisPtr,
	struct fb::EntityEvent* event
);

#else
DECLARE_HOOK(
	fb_ServerPVZLevelControlEntity_loadLevel,
	__fastcall,
	void,

	void* thisPtr,
	const char* level,
	const char* inclusion
);

DECLARE_HOOK(
	fb_ServerLevelControlEntity_loadLevel,
	__fastcall,
	void,

	void* thisPtr,
	bool notifyLevelComplete
);

DECLARE_HOOK(
	fb_ServerLoadLevelMessage_post,
	__cdecl,
	void,

	fb::LevelSetup* levelSetup,
	bool fadeOut,
	bool forceReloadResources
);
#endif

DECLARE_HOOK(
	fb_ServerConnection_onCreatePlayerMsg,
	__fastcall,
	void*,

	fb::ServerConnection* thisPtr,
	void* msg
);

DECLARE_HOOK(
	fb_ServerPlayerManager_addPlayer,
	__fastcall,
	void*,

	fb::ServerPlayerManager* thisPtr,
	fb::ServerPlayer* player,
	const char* nickname
);

#ifdef CYPRESS_BFN
DECLARE_HOOK(
	fb_ServerPlayer_disconnect,
	__fastcall,
	void,

	fb::ServerPlayer* thisPtr,
	fb::SecureReason reason,
	eastl::new_string* reasonText
);
#else
DECLARE_HOOK(
	fb_ServerPlayer_disconnect,
	__fastcall,
	void,

	fb::ServerPlayer* thisPtr,
	fb::SecureReason reason,
	eastl::string& reasonText
);

DECLARE_HOOK(
	fb_sub_140112AA0,
	__fastcall,
	__int64,

	void* a1,
	int a2
);
#endif
#endif
