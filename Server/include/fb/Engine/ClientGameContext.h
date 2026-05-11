#pragma once
#include <Cypress/Core/Config.h>
#include <fb/Engine/MessageManager.h>

#define OFFSET_CLIENTGAMECONTEXT CYPRESS_GW_SELECT(0x141F000E0, 0x142B5BB90, 0x1445B5040)

namespace fb
{
    struct ClientGameContext
    {
#ifndef CYPRESS_GW1
        void* vftable;
#endif
        int m_realm;
#ifdef CYPRESS_BFN
		void* unk1;
		void* unk2;
#endif
        MessageManager m_messageManager;
        char pad[CYPRESS_GW_SELECT(0x60, 0x58, 0x90)];
        class ClientGameView* m_clientGameView;

        static ClientGameContext* GetInstance()
        {
            return *reinterpret_cast<ClientGameContext**>(OFFSET_CLIENTGAMECONTEXT);
        }
    };
}
