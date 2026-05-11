#include "pch.h"

#ifdef CYPRESS_BFN

#include <GameModules/BFNModule.h>
#include <GameHooks/fbMainHooks.h>
#include <GameHooks/fbEnginePeerHooks.h>
#include <GameHooks/fbServerHooks.h>
#include <GameHooks/fbClientHooks.h>
#include <Cypress/Core/Server.h>
#include <Cypress/Core/Program.h>
#include <Cypress/Core/Console/ConsoleFunctions.h>

#include <fb/Engine/ExecutionContext.h>

void Cypress::BFNModule::InitGameHooks()
{
	// main hooks
	INIT_HOOK(fb_Main_initSettings, OFFSET_FB_MAIN_INITSETTINGS);
	INIT_HOOK(fb_realMain, OFFSET_FB_REALMAIN);
	INIT_HOOK(fb_main_createWindow, OFFSET_FB_MAIN_CREATEWINDOW);
	INIT_HOOK(fb_Environment_getHostIdentifier, OFFSET_ENVIRONMENT_GETHOSTIDENTIFIER);
	INIT_HOOK(fb_Console_writeConsole, OFFSET_CONSOLE__WRITECONSOLE);

	// fb::EnginePeer hooks
	INIT_HOOK(fb_EnginePeer_init, OFFSET_FB_ENGINEPEER_INIT);

#if(HAS_DEDICATED_SERVER)
	// fb::Server hooks
	INIT_HOOK(fb_Server_start, OFFSET_FB_SERVER_START);
	INIT_HOOK(fb_Server_update, OFFSET_FB_SERVER_UPDATE);
	INIT_HOOK(fb_windowProcedure, OFFSET_WINDOWPROCEDURE);
	INIT_HOOK(fb_ServerPVZRoundControl_event, OFFSET_FB_SERVERPVZROUNDCONTROL_EVENT);
	INIT_HOOK(fb_ServerPlayerManager_addPlayer, OFFSET_SERVERPLAYERMANAGER_ADDPLAYER);
	INIT_HOOK(fb_ServerConnection_onCreatePlayerMsg, OFFSET_SERVERCONNECTION__ONCREATEPLAYERMESSAGE);
	INIT_HOOK(fb_ServerPlayer_disconnect, OFFSET_SERVERPLAYER_DISCONNECT);
	INIT_HOOK(fb_OnlineManager_connectToAddress, OFFSET_FB_ONLINEMANAGER_CONNECTTOADDRESS);
#endif

	// fb::Client hooks
	INIT_HOOK(fb_Client_enterState, OFFSET_FB_CLIENT_ENTERSTATE);
	INIT_HOOK(fb_OnlineManager_onGotDisconnected, OFFSET_FB_ONLINEMANAGER_ONGOTDISCONNECTED);
	INIT_HOOK(fb_EAUser_ctor, OFFSET_FB_EAUSER_CTOR);
}

void Cypress::BFNModule::InitMemPatches()
{
	MemSet(0x1422183D6, 0x90, 5); //dont override commandLine
	MemSet(0x142218419, 0xA7, 1); //allowCommandlineSettings

	MemSet(0x1417F93A1, 0xEB, 1); //prevent ac initialize
	
	//for low-end users. Stops overriding "Render.ResolutionScale" on window change
	uint8_t calcResScalePatch[]{ 0x0F, 0x57, 0xC0, 0xC3 };
	MemPatch(0x1415801F0, calcResScalePatch, 4);

	// set as online
	uint8_t retTrue[]{0xB0, 0x01, 0xC3};
	MemPatch(0x1415E55B0, retTrue, 3);

	uint8_t interactionsFix[]{ 0xE9, 0xBA, 0x00 };
	MemPatch(0x1417288A9, interactionsFix, 3);

	MemSet(0x1435DE82A, 0x00, 1); //change player name format str from %s_%u to %s
#if(HAS_DEDICATED_SERVER)
	MemSet(0x1404603BE, 0x90, 6); //unlock all commands
#endif

	uint8_t uiIsPersistentLocalPlayer[]{ 0xC6, 0x44, 0x24, 0x21, 01 };
	MemPatch(0x14178B4CD, uiIsPersistentLocalPlayer, 5);
}

void Cypress::BFNModule::InitDedicatedServerPatches(Cypress::Server* pServer)
{
	//set server mode in realMain
	MemSet(0x142219DD0, 0x01, 1);
	//dont update title
	MemSet(0x142219085, 0xEB, 1);

	uintptr_t initDedicatedServerPtr = reinterpret_cast<uintptr_t>(&Server::InitDedicatedServer);
	MemPatch(0x14321E750, (unsigned char*)&initDedicatedServerPtr, 8);

	if (fb::ExecutionContext::getOptionValue("allUnlocksUnlocked"))
	{
		uint8_t unlockAll[]{ 0xE9, 0xC1, 0x01, 0x00, 0x00, 0x90 };
		MemPatch(0x141D04148, unlockAll, 6);
	}
}

void Cypress::BFNModule::RegisterCommands()
{
	CYPRESS_REGISTER_CONSOLE_FUNCTION(Server::ServerRestartLevel, "Server.RestartLevel", 0);
	CYPRESS_REGISTER_CONSOLE_FUNCTION(Server::ServerLoadLevel, "Server.LoadLevel", 0);
	CYPRESS_REGISTER_CONSOLE_FUNCTION(Server::ServerLoadNextRound, "Server.LoadNextRound", 0);
	CYPRESS_REGISTER_CONSOLE_FUNCTION(Server::ServerLoadNextRound, "Server.LoadNextPlaylistSetup", 0);
	CYPRESS_REGISTER_CONSOLE_FUNCTION(Server::ServerKickPlayer, "Server.KickPlayer", 0);
	CYPRESS_REGISTER_CONSOLE_FUNCTION(Server::ServerKickPlayerById, "Server.KickPlayerById", 0);
	CYPRESS_REGISTER_CONSOLE_FUNCTION(Server::ServerBanPlayer, "Server.BanPlayer", 0);
	CYPRESS_REGISTER_CONSOLE_FUNCTION(Server::ServerBanPlayerById, "Server.BanPlayerById", 0);
	CYPRESS_REGISTER_CONSOLE_FUNCTION(Server::ServerUnbanPlayer, "Server.UnbanPlayer", 0);
	CYPRESS_REGISTER_CONSOLE_FUNCTION(Server::ServerAddBan, "Server.AddBan", 0);
}

#endif
