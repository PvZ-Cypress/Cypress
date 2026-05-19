#include "pch.h"
#include <SideChannel.h>
#include <Cypress/Core/Logging.h>
#include <Cypress/Core/Program.h>
#include <HWID.h>
#include <fstream>
#include <random>
#include <winhttp.h>
#pragma comment(lib, "winhttp.lib")

extern "C" {
#include <monocypher.h>
}

namespace Cypress
{
	// http/https GET via WinHTTP, takes a full URL string
	static std::string HttpGet(const std::string& fullUrl, int timeoutMs = 5000)
	{
		std::wstring wUrl(fullUrl.begin(), fullUrl.end());

		URL_COMPONENTS uc = {};
		uc.dwStructSize = sizeof(uc);
		wchar_t hostBuf[256] = {}, pathBuf[1024] = {};
		uc.lpszHostName = hostBuf;
		uc.dwHostNameLength = _countof(hostBuf);
		uc.lpszUrlPath = pathBuf;
		uc.dwUrlPathLength = _countof(pathBuf);
		if (!WinHttpCrackUrl(wUrl.c_str(), 0, 0, &uc))
			return "";

		HINTERNET hSession = WinHttpOpen(L"Cypress/1.0", WINHTTP_ACCESS_TYPE_DEFAULT_PROXY,
			WINHTTP_NO_PROXY_NAME, WINHTTP_NO_PROXY_BYPASS, 0);
		if (!hSession) return "";

		WinHttpSetTimeouts(hSession, timeoutMs, timeoutMs, timeoutMs, timeoutMs);

		HINTERNET hConnect = WinHttpConnect(hSession, hostBuf, uc.nPort, 0);
		if (!hConnect) { WinHttpCloseHandle(hSession); return ""; }

		DWORD flags = (uc.nScheme == INTERNET_SCHEME_HTTPS) ? WINHTTP_FLAG_SECURE : 0;
		HINTERNET hRequest = WinHttpOpenRequest(hConnect, L"GET", pathBuf, nullptr,
			WINHTTP_NO_REFERER, WINHTTP_DEFAULT_ACCEPT_TYPES, flags);
		if (!hRequest) { WinHttpCloseHandle(hConnect); WinHttpCloseHandle(hSession); return ""; }

		if (!WinHttpSendRequest(hRequest, WINHTTP_NO_ADDITIONAL_HEADERS, 0,
			WINHTTP_NO_REQUEST_DATA, 0, 0, 0) ||
			!WinHttpReceiveResponse(hRequest, nullptr))
		{
			WinHttpCloseHandle(hRequest);
			WinHttpCloseHandle(hConnect);
			WinHttpCloseHandle(hSession);
			return "";
		}

		std::string body;
		DWORD bytesAvailable = 0;
		while (WinHttpQueryDataAvailable(hRequest, &bytesAvailable) && bytesAvailable > 0)
		{
			std::vector<char> buf(bytesAvailable);
			DWORD bytesRead = 0;
			WinHttpReadData(hRequest, buf.data(), bytesAvailable, &bytesRead);
			body.append(buf.data(), bytesRead);
		}

		WinHttpCloseHandle(hRequest);
		WinHttpCloseHandle(hConnect);
		WinHttpCloseHandle(hSession);
		return body;
	}

	static std::string UrlEncode(const std::string& value)
	{
		std::string out;
		out.reserve(value.size() * 2);
		for (unsigned char c : value)
		{
			if ((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') ||
				c == '-' || c == '_' || c == '.' || c == '~')
				out += (char)c;
			else
			{
				char hex[4];
				snprintf(hex, sizeof(hex), "%%%02X", c);
				out += hex;
			}
		}
		return out;
	}

	// nonce+sig verification against master server, no raw token
	static bool ValidateModChallenge(const std::string& nonce, const std::string& sig)
	{
		char urlBuf[256] = {};
		if (GetEnvironmentVariableA("CYPRESS_MASTER_URL", urlBuf, sizeof(urlBuf)) == 0)
		{
			CYPRESS_LOGMESSAGE(LogLevel::Warning, "SideChannel: CYPRESS_MASTER_URL not set, can't validate mod");
			return false;
		}

		std::string url(urlBuf);
		if (!url.empty() && url.back() == '/') url.pop_back();

		std::string fullUrl = url + "/mod/verify?nonce=" + UrlEncode(nonce) + "&sig=" + UrlEncode(sig);
		std::string body = HttpGet(fullUrl);
		if (body.empty())
		{
			CYPRESS_LOGMESSAGE(LogLevel::Warning, "SideChannel: Mod verify HTTP request failed");
			return false;
		}

		try
		{
			auto j = nlohmann::json::parse(body);
			bool ok = j.value("ok", false);
			if (ok)
				CYPRESS_LOGMESSAGE(LogLevel::Debug, "SideChannel: Master server confirmed mod (user={})", j.value("username", "?"));
			return ok;
		}
		catch (...)
		{
			CYPRESS_LOGMESSAGE(LogLevel::Warning, "SideChannel: Failed to parse mod verify response: {}", body.substr(0, 128));
			return false;
		}
	}

	static std::string GenerateNonce()
	{
		BYTE buf[32];
		BCryptGenRandom(nullptr, buf, sizeof(buf), BCRYPT_USE_SYSTEM_PREFERRED_RNG);
		char hex[65];
		for (int i = 0; i < 32; ++i)
			snprintf(hex + i * 2, 3, "%02x", buf[i]);
		hex[64] = '\0';
		return std::string(hex);
	}

	static std::string HmacSha256(const std::string& key, const std::string& data)
	{
		BCRYPT_ALG_HANDLE hAlg = nullptr;
		BCRYPT_HASH_HANDLE hHash = nullptr;
		BYTE hash[32] = {};

		BCryptOpenAlgorithmProvider(&hAlg, BCRYPT_SHA256_ALGORITHM, nullptr, BCRYPT_ALG_HANDLE_HMAC_FLAG);
		BCryptCreateHash(hAlg, &hHash, nullptr, 0, (PUCHAR)key.data(), (ULONG)key.size(), 0);
		BCryptHashData(hHash, (PUCHAR)data.data(), (ULONG)data.size(), 0);
		BCryptFinishHash(hHash, hash, sizeof(hash), 0);
		BCryptDestroyHash(hHash);
		BCryptCloseAlgorithmProvider(hAlg, 0);

		char hex[65];
		for (int i = 0; i < 32; ++i)
			snprintf(hex + i * 2, 3, "%02x", hash[i]);
		hex[64] = '\0';
		return std::string(hex);
	}

	// hash an account id for storage
	static std::string HashAccountId(const std::string& accountId)
	{
		return detail::sha256hex("cypress-mod:" + accountId);
	}

	int GetSideChannelPort()
	{
		char buf[32] = {};
		if (GetEnvironmentVariableA("CYPRESS_SIDE_CHANNEL_PORT", buf, sizeof(buf)) > 0)
		{
			int port = atoi(buf);
			if (port > 0 && port < 65536) return port;
		}
		return SIDE_CHANNEL_DEFAULT_PORT;
	}

	void WriteDiscoveryFile(int port, const char* game, bool isServer)
	{
		char tempDir[MAX_PATH];
		DWORD len = GetTempPathA(MAX_PATH, tempDir);
		if (len == 0) return;

		DWORD pid = GetCurrentProcessId();
		std::string path = std::string(tempDir) + "cypress_" + std::to_string(pid) + ".port";

		// store process creation time so stale files with reused pids can be detected
		FILETIME ct = {}, dummy = {};
		GetProcessTimes(GetCurrentProcess(), &ct, &dummy, &dummy, &dummy);
		char createTimeHex[32];
		ULONGLONG ctVal = ((ULONGLONG)ct.dwHighDateTime << 32) | ct.dwLowDateTime;
		snprintf(createTimeHex, sizeof(createTimeHex), "%016llx", ctVal);

		nlohmann::json j = {
			{"port", port},
			{"pid", (int)pid},
			{"game", game},
			{"isServer", isServer},
			{"createTime", std::string(createTimeHex)}
		};

		std::ofstream f(path);
		if (f.is_open())
		{
			f << j.dump();
			CYPRESS_LOGMESSAGE(LogLevel::Debug, "SideChannel: Wrote discovery file {}", path);
		}
	}

	void DeleteDiscoveryFile()
	{
		char tempDir[MAX_PATH];
		DWORD len = GetTempPathA(MAX_PATH, tempDir);
		if (len == 0) return;

		DWORD pid = GetCurrentProcessId();
		std::string path = std::string(tempDir) + "cypress_" + std::to_string(pid) + ".port";
		DeleteFileA(path.c_str());
	}

	SideChannelServer::SideChannelServer() {}

	SideChannelServer::~SideChannelServer()
	{
		Stop(); // joins and clears m_clientThreads
	}

	bool SideChannelServer::Start(int port)
	{
		if (m_running) return true;

		if (port <= 0) port = GetSideChannelPort();
		m_port = port;

		m_listenSock = socket(AF_INET, SOCK_STREAM, IPPROTO_TCP);
		if (m_listenSock == INVALID_SOCKET)
		{
			CYPRESS_LOGMESSAGE(LogLevel::Error, "SideChannel: Failed to create listen socket ({})", WSAGetLastError());
			return false;
		}

		int optval = 1;
		setsockopt(m_listenSock, SOL_SOCKET, SO_REUSEADDR, (const char*)&optval, sizeof(optval));

		sockaddr_in addr = {};
		addr.sin_family = AF_INET;
		addr.sin_addr.s_addr = htonl(INADDR_ANY);
		addr.sin_port = htons(m_port);

		if (bind(m_listenSock, (sockaddr*)&addr, sizeof(addr)) == SOCKET_ERROR)
		{
			CYPRESS_LOGMESSAGE(LogLevel::Warning, "SideChannel: Port {} in use, trying fallback port", m_port);
			addr.sin_port = htons(0);
			if (bind(m_listenSock, (sockaddr*)&addr, sizeof(addr)) == SOCKET_ERROR)
			{
				CYPRESS_LOGMESSAGE(LogLevel::Error, "SideChannel: Bind failed ({})", WSAGetLastError());
				closesocket(m_listenSock);
				m_listenSock = INVALID_SOCKET;
				return false;
			}
			sockaddr_in bound = {};
			int boundLen = sizeof(bound);
			getsockname(m_listenSock, (sockaddr*)&bound, &boundLen);
			m_port = ntohs(bound.sin_port);
		}

		if (listen(m_listenSock, SOMAXCONN) == SOCKET_ERROR)
		{
			CYPRESS_LOGMESSAGE(LogLevel::Error, "SideChannel: Listen failed ({})", WSAGetLastError());
			closesocket(m_listenSock);
			m_listenSock = INVALID_SOCKET;
			return false;
		}

		m_running = true;
		char requireIdentityBuf[8] = {};
		m_requireIdentity = GetEnvironmentVariableA("CYPRESS_REQUIRE_IDENTITY", requireIdentityBuf, sizeof(requireIdentityBuf)) > 0
			&& strcmp(requireIdentityBuf, "1") == 0;
		char allowModsBuf[8] = {};
		m_allowGlobalMods = !(GetEnvironmentVariableA("CYPRESS_ALLOW_GLOBAL_MODS", allowModsBuf, sizeof(allowModsBuf)) > 0
			&& strcmp(allowModsBuf, "0") == 0);
		LoadMasterPubKey();
		StartBanListSync();
		m_acceptThread = std::thread(&SideChannelServer::AcceptLoop, this);
		CYPRESS_LOGMESSAGE(LogLevel::Info, "SideChannel: Server listening on port {}", m_port);
		return true;
	}

	void SideChannelServer::Stop()
	{
		if (!m_running) return;
		m_running = false;
		StopBanListSync();

		if (m_listenSock != INVALID_SOCKET)
		{
			closesocket(m_listenSock);
			m_listenSock = INVALID_SOCKET;
		}

		{
			std::lock_guard<std::recursive_mutex> lock(m_peersMutex);
			for (auto& [sock, peer] : m_peers)
			{
				closesocket(sock);
			}
			m_peers.clear();
		}

		if (m_acceptThread.joinable())
			m_acceptThread.join();

		// join all client threads now that their sockets are closed
		for (auto& t : m_clientThreads)
			if (t.joinable()) t.join();
		m_clientThreads.clear();
	}

	void SideChannelServer::LoadMasterPubKey()
	{
		// try env var first (hex-encoded 32-byte key)
		char pubKeyBuf[128] = {};
		if (GetEnvironmentVariableA("CYPRESS_MASTER_PUBKEY", pubKeyBuf, sizeof(pubKeyBuf)) > 0)
		{
			auto bytes = Cypress::Identity::detail::hex_decode(pubKeyBuf);
			if (bytes.size() == 32)
			{
				memcpy(m_masterPubKey, bytes.data(), 32);
				m_masterPubKeyLoaded = true;
				CYPRESS_LOGMESSAGE(LogLevel::Info, "SideChannel: Loaded master public key from env");
				return;
			}
		}

		// try fetching from master server
		char urlBuf[256] = {};
		if (GetEnvironmentVariableA("CYPRESS_MASTER_URL", urlBuf, sizeof(urlBuf)) == 0)
		{
			CYPRESS_LOGMESSAGE(LogLevel::Warning, "SideChannel: No master pubkey or URL configured, identity verification disabled");
			return;
		}

		std::string url(urlBuf);
		if (!url.empty() && url.back() == '/') url.pop_back();

		std::string body = HttpGet(url + "/auth/pubkey");
		if (body.empty())
		{
			CYPRESS_LOGMESSAGE(LogLevel::Warning, "SideChannel: Failed to fetch master pubkey, identity verification disabled");
			return;
		}

		try
		{
			auto j = nlohmann::json::parse(body);
			std::string pkHex = j.value("public_key", "");
			auto bytes = Cypress::Identity::detail::hex_decode(pkHex);
			if (bytes.size() == 32)
			{
				memcpy(m_masterPubKey, bytes.data(), 32);
				m_masterPubKeyLoaded = true;
				CYPRESS_LOGMESSAGE(LogLevel::Info, "SideChannel: Fetched master public key (fp={})", j.value("fingerprint", "?"));
			}
		}
		catch (...)
		{
			CYPRESS_LOGMESSAGE(LogLevel::Warning, "SideChannel: Failed to parse master pubkey response");
		}
	}

	void SideChannelServer::StartBanListSync()
	{
		if (!m_masterPubKeyLoaded) return;

		m_banListRunning = true;
		m_banListThread = std::thread([this]()
		{
			while (m_banListRunning)
			{
				FetchBanList();
				// sleep 5 minutes, check every second for shutdown
				for (int i = 0; i < 300 && m_banListRunning; ++i)
					std::this_thread::sleep_for(std::chrono::seconds(1));
			}
		});
	}

	void SideChannelServer::StopBanListSync()
	{
		m_banListRunning = false;
		if (m_banListThread.joinable())
			m_banListThread.join();
	}

	void SideChannelServer::FetchBanList()
	{
		char urlBuf[256] = {};
		if (GetEnvironmentVariableA("CYPRESS_MASTER_URL", urlBuf, sizeof(urlBuf)) == 0) return;

		std::string url(urlBuf);
		if (!url.empty() && url.back() == '/') url.pop_back();

		std::string body = HttpGet(url + "/auth/banlist");
		if (body.empty()) return;

		auto bl = Cypress::Identity::parse_banlist(body);
		{
			std::lock_guard<std::mutex> lock(m_banListMutex);
			m_banList = std::move(bl);
		}
		CYPRESS_LOGMESSAGE(LogLevel::Debug, "SideChannel: Updated ban list");

		// kick any connected peers that are now banned
		std::vector<SOCKET> toKick;
		{
			std::lock_guard<std::recursive_mutex> plock(m_peersMutex);
			std::lock_guard<std::mutex> block(m_banListMutex);
			for (auto& [sock, peer] : m_peers)
			{
				if (!peer.authenticated) continue;
				bool banned = (!peer.accountId.empty() && m_banList.is_account_banned(peer.accountId))
					|| (!peer.eaPid.empty() && m_banList.is_ea_pid_banned(peer.eaPid))
					|| (!peer.entid.empty() && m_banList.is_entid_banned(peer.entid));
					// || (!peer.hwid.empty() && m_banList.is_hwid_banned(peer.hwid)); disabled: hash collisions
				if (banned)
				{
					CYPRESS_LOGMESSAGE(LogLevel::Warning, "SideChannel: Kicking banned peer {}", peer.name);
					SendToPeer(peer, { {"type", "authResult"}, {"ok", false}, {"msg", "account banned"} });
					toKick.push_back(sock);
				}
			}
		}
		for (SOCKET s : toKick)
			closesocket(s);
	}

	void SideChannelServer::AcceptLoop()
	{
		static constexpr size_t MAX_UNAUTHED_PEERS = 32;

		while (m_running)
		{
			sockaddr_in clientAddr = {};
			int addrLen = sizeof(clientAddr);
			SOCKET clientSock = accept(m_listenSock, (sockaddr*)&clientAddr, &addrLen);

			if (clientSock == INVALID_SOCKET)
			{
				if (m_running)
					CYPRESS_LOGMESSAGE(LogLevel::Warning, "SideChannel: Accept failed ({})", WSAGetLastError());
				continue;
			}

			// reject if too many unauthenticated connections pending
			{
				std::lock_guard<std::recursive_mutex> lock(m_peersMutex);
				size_t unauthed = 0;
				for (const auto& [s, p] : m_peers)
					if (!p.authenticated) ++unauthed;
				if (unauthed >= MAX_UNAUTHED_PEERS)
				{
					closesocket(clientSock);
					continue;
				}
			}

			int nodelay = 1;
			setsockopt(clientSock, IPPROTO_TCP, TCP_NODELAY, (const char*)&nodelay, sizeof(nodelay));

			{
				std::lock_guard<std::recursive_mutex> lock(m_peersMutex);
				m_peers[clientSock] = SideChannelPeer{ clientSock };
			}

			char addrStr[INET_ADDRSTRLEN];
			inet_ntop(AF_INET, &clientAddr.sin_addr, addrStr, sizeof(addrStr));
			CYPRESS_DEBUG_LOGMESSAGE(LogLevel::Debug, "SideChannel: New connection from {}", addrStr);

			// reap finished threads before adding the new one
			m_clientThreads.erase(
				std::remove_if(m_clientThreads.begin(), m_clientThreads.end(),
					[](std::thread& t) -> bool {
						if (!t.joinable()) return true;
						HANDLE h = (HANDLE)t.native_handle();
						if (WaitForSingleObject(h, 0) == WAIT_OBJECT_0)
						{
							t.join();
							return true;
						}
						return false;
					}),
				m_clientThreads.end());

			m_clientThreads.emplace_back(&SideChannelServer::ClientLoop, this, clientSock);
		}
	}

	void SideChannelServer::ClientLoop(SOCKET clientSock)
	{
		static constexpr size_t MAX_RECV_BUF = 65536; // 64KB max pending data per peer
		static constexpr int AUTH_TIMEOUT_MS = 5000;   // 5s to authenticate or get dropped
		static constexpr int RECV_TIMEOUT_MS = 30000;  // 30s idle timeout after auth

		// tight timeout until authenticated
		setsockopt(clientSock, SOL_SOCKET, SO_RCVTIMEO, (const char*)&AUTH_TIMEOUT_MS, sizeof(AUTH_TIMEOUT_MS));

		// send challenge nonce for auth
		std::string challengeNonce = GenerateNonce();
		{
			std::lock_guard<std::recursive_mutex> lock(m_peersMutex);
			auto it = m_peers.find(clientSock);
			if (it != m_peers.end())
				it->second.challengeNonce = challengeNonce;
		}
		nlohmann::json challengeMsg = { {"type", "challenge"}, {"nonce", challengeNonce} };
		std::string challengeLine = challengeMsg.dump() + "\n";
		::send(clientSock, challengeLine.c_str(), (int)challengeLine.size(), 0);

		char buf[4096];
		while (m_running)
		{
			int bytesRead = recv(clientSock, buf, sizeof(buf) - 1, 0);
			if (bytesRead <= 0)
			{
				// on recv timeout, send keepalive ping to authenticated peers
				int err = WSAGetLastError();
				if (bytesRead == SOCKET_ERROR && err == WSAETIMEDOUT)
				{
					std::lock_guard<std::recursive_mutex> lock(m_peersMutex);
					auto it = m_peers.find(clientSock);
					if (it != m_peers.end() && it->second.authenticated)
					{
						nlohmann::json ping = { {"type", "ping"} };
						SendToPeer(it->second, ping);
						continue;
					}
				}
				break;
			}

			buf[bytesRead] = '\0';

			std::lock_guard<std::recursive_mutex> lock(m_peersMutex);
			auto it = m_peers.find(clientSock);
			if (it == m_peers.end()) break;

			SideChannelPeer& peer = it->second;
			peer.recvBuf += buf;

			// kill connection if buffer too large (no newline = likely malicious)
			if (peer.recvBuf.size() > MAX_RECV_BUF)
			{
				if (peer.authenticated)
					CYPRESS_LOGMESSAGE(LogLevel::Warning, "SideChannel: Dropping {} - recv buffer exceeded 64KB", peer.name.empty() ? "(unknown)" : peer.name);
				break;
			}

			size_t pos;
			while ((pos = peer.recvBuf.find('\n')) != std::string::npos)
			{
				std::string line = peer.recvBuf.substr(0, pos);
				peer.recvBuf.erase(0, pos + 1);
				if (!line.empty())
					ProcessLine(peer, line);
			}
		}

		RemovePeer(clientSock);
	}

	// called after hwid auth + identity verification (or directly if identity not enforced)
	void SideChannelServer::FinalizeAuth(SideChannelPeer& peer, bool claimMod)
	{
		// check persisted moderator list now that accountId is populated
		if (!peer.isModerator && !peer.accountId.empty())
			peer.isModerator = IsModerator(peer.accountId);

		// mod challenge: only if global mods are allowed on this server
		if (peer.authenticated && !peer.isModerator && claimMod && m_allowGlobalMods)
		{
			peer.modChallengeNonce = GenerateNonce();
			CYPRESS_LOGMESSAGE(LogLevel::Debug, "SideChannel: Issuing mod challenge to {}", peer.name);
			SendToPeer(peer, { {"type", "modChallenge"}, {"nonce", peer.modChallengeNonce} });
		}

		// auto-mod first player if env var is set
		if (peer.authenticated && !peer.isModerator)
		{
			char autoModBuf[8] = {};
			if (GetEnvironmentVariableA("CYPRESS_AUTO_MOD_HOST", autoModBuf, sizeof(autoModBuf)) > 0
				&& strcmp(autoModBuf, "1") == 0)
			{
				if (m_moderatorAccountIds.empty())
				{
					AddModerator(peer.accountId);
					SaveModerators("moderators.json");
					peer.isModerator = true;
					CYPRESS_LOGMESSAGE(LogLevel::Info, "SideChannel: Auto-modded first player {}", peer.name);
					if (Cypress_IsEmbeddedMode())
						Cypress_EmitJsonPlayerEvent("modListChanged", -1, "", nullptr);
				}
			}
		}

		nlohmann::json response = {
			{"type", "authResult"},
			{"ok", peer.authenticated},
			{"moderator", peer.isModerator}
		};
		SendToPeer(peer, response);

		if (peer.authenticated)
		{
			if (m_onAuth) m_onAuth(peer);

			// use a recv timeout so we can send periodic keepalive pings
			// this keeps the connection alive through relay TCP proxying
			int keepaliveTimeout = 30000;
			setsockopt(peer.sock, SOL_SOCKET, SO_RCVTIMEO, (const char*)&keepaliveTimeout, sizeof(keepaliveTimeout));

			// display name: nickname > username > game name
			std::string displayName = peer.name;
			if (!peer.identityNickname.empty())
				displayName = peer.identityNickname;
			else if (!peer.identityUsername.empty())
				displayName = peer.identityUsername;

			CYPRESS_LOGMESSAGE(LogLevel::Info, "SideChannel: {} authenticated{}{}{}",
				displayName.empty() ? peer.name : displayName,
				peer.isModerator ? " (mod)" : "",
				peer.identityVerified ? " (verified)" : "",
				displayName != peer.name ? " [" + peer.name + "]" : "");

			if (Cypress_IsEmbeddedMode())
			{
				nlohmann::json authEvent = {
					{"t", "sideChannelAuth"},
					{"id", -1},
					{"name", peer.name},
					{"display_name", displayName},
						{"extra", std::string(peer.isModerator ? "mod" : "player")},
					{"account_id", peer.accountId}
				};
				if (g_program && g_program->IsServer() && g_program->GetServer())
					g_program->GetServer()->AppendPlayerMetadata(authEvent, peer.name);
				Cypress_WriteRawStdout(authEvent.dump() + "\n");
			}

			// notify connected moderator clients about this player's auth data (full)
			std::string modDisplayName = displayName;
			if (!peer.identityNickname.empty() && !peer.identityUsername.empty())
				modDisplayName = peer.identityNickname + " (@" + peer.identityUsername + ")";

			nlohmann::json authMsg = {
				{"type", "scPlayerAuth"},
				{"name", peer.name},
				{"display_name", modDisplayName},
				{"username", peer.identityUsername},
				{"nickname", peer.identityNickname},
				{"ea_pid", peer.eaPid},
				{"hwid", peer.hwid},
				{"components", peer.fingerprint.toJson()},
				{"account_id", peer.accountId}
			};
			if (g_program && g_program->IsServer() && g_program->GetServer())
				g_program->GetServer()->AppendPlayerMetadata(authMsg, peer.name);
			BroadcastToMods(authMsg);

			if (peer.isModerator && m_onModeratorAuth)
			{
				m_onModeratorAuth(peer);
			}
		}
	}

	void SideChannelServer::ProcessLine(SideChannelPeer& peer, const std::string& line)
	{
		try
		{
			auto msg = nlohmann::json::parse(line);
			std::string type = msg.value("type", "");

			if (type == "pong") return;

			if (type == "serverInfo")
			{
				ServerInfo info = GetServerInfo();
				// use cached player names (snapshotted on main thread) to avoid race
				nlohmann::json playerNames = nlohmann::json::array();
				int playerCount = 0;
				if (m_playerNamesCb) {
					auto names = GetCachedPlayerNames();
					playerCount = (int)names.size();
					for (const auto& n : names)
						playerNames.push_back(n);
				} else {
					auto peers = GetConnectedPeers();
					playerCount = (int)peers.size();
					for (const auto& [name, hwid] : peers)
						playerNames.push_back(name);
				}
				nlohmann::json response = {
					{"type", "serverInfo"},
#ifdef CYPRESS_GW1
					{"game", "GW1"},
#elif defined(CYPRESS_BFN)
					{"game", "BFN"},
#else
					{"game", "GW2"},
#endif
					{"players", playerCount},
					{"playerNames", playerNames},
					{"port", m_port}
				};
				// include game port so clients can connect to the right port when multiple servers are running
				char gamePortBuf[16] = {};
				if (GetEnvironmentVariableA("CYPRESS_SERVER_PORT", gamePortBuf, sizeof(gamePortBuf)) > 0 && gamePortBuf[0] != '\0')
					response["gamePort"] = atoi(gamePortBuf);
				else
					response["gamePort"] = 25200;
				if (!info.motd.empty()) response["motd"] = info.motd;
				if (!info.icon.empty()) response["icon"] = info.icon;
				response["modded"] = info.modded;
				if (!info.modpackUrl.empty()) response["modpackUrl"] = info.modpackUrl;
				if (!info.level.empty()) response["level"] = info.level;
				if (!info.mode.empty()) response["mode"] = info.mode;
				SendToPeer(peer, response);
				return;
			}

			if (type == "auth")
			{
				std::string clientGame = msg.value("game", "");
				if (!clientGame.empty() && clientGame != CYPRESS_GAME_NAME)
				{
					CYPRESS_LOGMESSAGE(LogLevel::Warning, "SideChannel: Rejecting {} - wrong game (client={}, server={})",
						msg.value("name", "(unknown)"), clientGame, CYPRESS_GAME_NAME);
					SendToPeer(peer, { {"type", "authResult"}, {"ok", false}, {"msg", "wrong game, this server is running " + std::string(CYPRESS_GAME_NAME)} });
					return;
				}

				peer.name = msg.value("name", "");
				peer.hwid = msg.value("hwid", "");
				std::string proof = msg.value("proof", "");
				if (msg.contains("components"))
					peer.fingerprint = Cypress::HardwareFingerprint::fromJson(msg["components"]);

				bool proofValid = false;
				if (!peer.challengeNonce.empty() && !peer.hwid.empty() && !proof.empty())
				{
					std::string expected = HmacSha256(peer.challengeNonce, peer.hwid);
					proofValid = (proof == expected);
				}

				if (!proofValid)
				{
					CYPRESS_LOGMESSAGE(LogLevel::Warning, "SideChannel: Auth from {} failed challenge-response", peer.name.empty() ? "(unknown)" : peer.name);
					SendToPeer(peer, { {"type", "authResult"}, {"ok", false}, {"msg", "invalid proof"} });
					return;
				}

				peer.challengeNonce.clear();

				peer.authenticated = !peer.name.empty() && !peer.hwid.empty();

				// verify identity jwt, required when master pubkey is loaded and server opts in
				std::string jwt = msg.value("jwt", "");
				if (peer.authenticated && m_masterPubKeyLoaded && m_requireIdentity)
				{
					if (jwt.empty())
					{
						CYPRESS_LOGMESSAGE(LogLevel::Warning, "SideChannel: Rejecting {} - no identity JWT", peer.name);
						SendToPeer(peer, { {"type", "authResult"}, {"ok", false}, {"msg", "identity required"} });
						peer.authenticated = false;
						if (m_onAuthReject && !peer.name.empty()) m_onAuthReject(peer.name, "Cypress account required");
						return;
					}

					Cypress::Identity::JWTClaims claims;
					if (!Cypress::Identity::verify_jwt(jwt, m_masterPubKey, claims))
					{
						CYPRESS_LOGMESSAGE(LogLevel::Warning, "SideChannel: Rejecting {} - invalid identity JWT", peer.name);
						SendToPeer(peer, { {"type", "authResult"}, {"ok", false}, {"msg", "invalid identity"} });
						peer.authenticated = false;
						if (m_onAuthReject && !peer.name.empty()) m_onAuthReject(peer.name, "Invalid identity");
						return;
					}

					peer.accountId = claims.sub;
					peer.identityUsername = claims.username;
					peer.identityNickname = claims.nickname;
					peer.eaPid = claims.ea_pid;

					// pick entid for the game this server is running
					{
						const char* gameName = CYPRESS_GAME_NAME;
						if (strcmp(gameName, "GW1") == 0)
							peer.entid = claims.entid_gw1;
						else if (strcmp(gameName, "GW2") == 0)
							peer.entid = claims.entid_gw2;
						else if (strcmp(gameName, "BFN") == 0)
							peer.entid = claims.entid_bfn;
					}

					// reject if they don't own this game
					if (peer.entid.empty())
					{
						CYPRESS_LOGMESSAGE(LogLevel::Warning, "SideChannel: Rejecting {} - no entitlement for this game", peer.name);
						SendToPeer(peer, { {"type", "authResult"}, {"ok", false}, {"msg", "you do not own this game"} });
						peer.authenticated = false;
						if (m_onAuthReject && !peer.name.empty()) m_onAuthReject(peer.name, "Game not owned");
						return;
					}

					// check bans before issuing challenge
					{
						std::lock_guard<std::mutex> lock(m_banListMutex);
						if (m_banList.is_account_banned(claims.sub))
						{
							CYPRESS_LOGMESSAGE(LogLevel::Warning, "SideChannel: Rejecting {} - account banned", peer.name);
							SendToPeer(peer, { {"type", "authResult"}, {"ok", false}, {"msg", "account banned"} });
							peer.authenticated = false;
							if (m_onAuthReject && !peer.name.empty()) m_onAuthReject(peer.name, "Account banned");
							return;
						}
						if (!claims.ea_pid.empty() && m_banList.is_ea_pid_banned(claims.ea_pid))
						{
							CYPRESS_LOGMESSAGE(LogLevel::Warning, "SideChannel: Rejecting {} - account banned", peer.name);
							SendToPeer(peer, { {"type", "authResult"}, {"ok", false}, {"msg", "account banned"} });
							peer.authenticated = false;
							if (m_onAuthReject && !peer.name.empty()) m_onAuthReject(peer.name, "EA account banned");
							return;
						}
						if (!peer.entid.empty() && m_banList.is_entid_banned(peer.entid))
						{
							CYPRESS_LOGMESSAGE(LogLevel::Warning, "SideChannel: Rejecting {} - entitlement banned", peer.name);
							SendToPeer(peer, { {"type", "authResult"}, {"ok", false}, {"msg", "account banned"} });
							peer.authenticated = false;
							if (m_onAuthReject && !peer.name.empty()) m_onAuthReject(peer.name, "Account banned");
							return;
						}
					}

					// defer authResult until identity proof arrives
					peer.pendingIdentity = true;
					peer.pendingClaimMod = msg.value("claimMod", false);
					peer.identityChallengeNonce = GenerateNonce();
					SendToPeer(peer, { {"type", "identityChallenge"}, {"nonce", peer.identityChallengeNonce} });

					CYPRESS_LOGMESSAGE(LogLevel::Debug, "SideChannel: JWT verified for {}, issuing identity challenge", peer.name);
					return;
				}

				// no master pubkey loaded, identity not enforced, proceed normally
				FinalizeAuth(peer, msg.value("claimMod", false));
				return;
			}

			if (type == "identityProof")
			{
				if (!peer.authenticated || peer.identityChallengeNonce.empty())
				{
					SendToPeer(peer, { {"type", "error"}, {"msg", "No pending identity challenge"} });
					return;
				}

				std::string sigHex = msg.value("sig", "");
				std::string pubKeyHex = msg.value("public_key", "");

				auto pubKeyBytes = Cypress::Identity::detail::hex_decode(pubKeyHex);
				if (pubKeyBytes.size() != 32)
				{
					CYPRESS_LOGMESSAGE(LogLevel::Warning, "SideChannel: Rejecting {} - invalid identity public key", peer.name);
					SendToPeer(peer, { {"type", "authResult"}, {"ok", false}, {"msg", "invalid identity key"} });
					peer.authenticated = false;
					peer.pendingIdentity = false;
					peer.identityChallengeNonce.clear();
					if (m_onAuthReject && !peer.name.empty()) m_onAuthReject(peer.name, "Invalid identity key");
					return;
				}

				if (!Cypress::Identity::verify_challenge(pubKeyBytes.data(),
					peer.identityChallengeNonce, sigHex))
				{
					CYPRESS_LOGMESSAGE(LogLevel::Warning, "SideChannel: Rejecting {} - identity proof failed", peer.name);
					SendToPeer(peer, { {"type", "authResult"}, {"ok", false}, {"msg", "identity verification failed"} });
					peer.authenticated = false;
					peer.pendingIdentity = false;
					peer.identityChallengeNonce.clear();
					if (m_onAuthReject && !peer.name.empty()) m_onAuthReject(peer.name, "Identity verification failed");
					return;
				}

				peer.identityVerified = true;
				peer.identityPubKey = pubKeyBytes;
				peer.identityChallengeNonce.clear();
				peer.pendingIdentity = false;

				CYPRESS_LOGMESSAGE(LogLevel::Info, "SideChannel: {} identity verified", peer.name);
				SendToPeer(peer, { {"type", "identityResult"}, {"ok", true}, {"accountId", peer.accountId} });

				// identity confirmed, finalize auth
				FinalizeAuth(peer, peer.pendingClaimMod);
				return;
			}

			if (type == "subscribe")
			{
				if (!peer.authenticated)
				{
					SendToPeer(peer, { {"type", "error"}, {"msg", "Must authenticate before subscribing"} });
					return;
				}
				peer.subscribed = true;
				SendToPeer(peer, { {"type", "subscribed"}, {"ok", true} });
				CYPRESS_LOGMESSAGE(LogLevel::Debug, "SideChannel: {} subscribed to events", peer.name);
				return;
			}

			if (type == "modTokenUpdate")
			{
				if (!peer.authenticated)
				{
					SendToPeer(peer, { {"type", "error"}, {"msg", "Not authenticated"} });
					return;
				}
				if (!m_allowGlobalMods) return;
				if (peer.isModerator) return; // already a mod

				// rate limit: 30 seconds between challenges
				auto now = std::chrono::steady_clock::now();
				if (peer.modChallengeTime.time_since_epoch().count() > 0 &&
					std::chrono::duration_cast<std::chrono::seconds>(now - peer.modChallengeTime).count() < 30)
				{
					SendToPeer(peer, { {"type", "error"}, {"msg", "Too many mod challenge requests"} });
					return;
				}
				peer.modChallengeTime = now;

				// issue a fresh mod challenge
				peer.modChallengeNonce = GenerateNonce();
				CYPRESS_LOGMESSAGE(LogLevel::Debug, "SideChannel: Issuing mod challenge to {} (token update)", peer.name);
				SendToPeer(peer, { {"type", "modChallenge"}, {"nonce", peer.modChallengeNonce} });
				return;
			}

			if (type == "modChallengeResponse")
			{
				if (!peer.authenticated || peer.isModerator || !m_allowGlobalMods) return;
				if (peer.modChallengeNonce.empty())
				{
					SendToPeer(peer, { {"type", "error"}, {"msg", "No pending mod challenge"} });
					return;
				}

				std::string sig = msg.value("sig", "");
				std::string nonce = peer.modChallengeNonce;
				peer.modChallengeNonce.clear(); // consume nonce, prevent replay

				if (sig.empty() || !ValidateModChallenge(nonce, sig))
				{
					CYPRESS_LOGMESSAGE(LogLevel::Warning, "SideChannel: {} failed mod challenge", peer.name);
					SendToPeer(peer, { {"type", "error"}, {"msg", "Mod verification failed"} });
					return;
				}

				peer.isModerator = true;
				peer.isGlobalMod = true;
				CYPRESS_LOGMESSAGE(LogLevel::Info, "SideChannel: {} verified as global moderator", peer.name);
				SendToPeer(peer, { {"type", "moderatorStatus"}, {"moderator", true} });
				if (m_onModeratorAuth)
					m_onModeratorAuth(peer);
				return;
			}

			// mod can request full player list at any time (belt-and-suspenders for relay timing)
			if (type == "requestPlayerList")
			{
				if (!peer.authenticated || !peer.isModerator)
				{
					SendToPeer(peer, { {"type", "error"}, {"msg", "Not a moderator"} });
					return;
				}
				if (m_onModeratorAuth)
					m_onModeratorAuth(peer);
				return;
			}

			if (!peer.authenticated)
			{
				nlohmann::json err = { {"type", "error"}, {"msg", "Not authenticated"} };
				SendToPeer(peer, err);
				return;
			}

			{
				std::lock_guard<std::mutex> hlock(m_handlersMutex);
				auto hit = m_handlers.find(type);
				if (hit != m_handlers.end())
				{
					hit->second(msg, peer);
					return;
				}
			}
		}
		catch (const std::exception& e)
		{
			if (peer.authenticated)
				CYPRESS_LOGMESSAGE(LogLevel::Warning, "SideChannel: Bad message from {}: {}", peer.name, e.what());
		}
	}

	void SideChannelServer::RemovePeer(SOCKET sock)
	{
		std::lock_guard<std::recursive_mutex> lock(m_peersMutex);
		auto it = m_peers.find(sock);
		if (it != m_peers.end())
		{
			if (it->second.authenticated)
			{
				CYPRESS_LOGMESSAGE(LogLevel::Info, "SideChannel: {} disconnected", it->second.name);
				if (Cypress_IsEmbeddedMode())
					Cypress_EmitJsonPlayerEvent("sideChannelDisconnect", -1, it->second.name.c_str());
			}
			closesocket(sock);
			m_peers.erase(it);
		}
	}

	void SideChannelServer::Broadcast(const nlohmann::json& msg)
	{
		std::string line = msg.dump() + "\n";
		std::lock_guard<std::recursive_mutex> lock(m_peersMutex);
		for (auto& [sock, peer] : m_peers)
		{
			if (peer.authenticated)
				send(sock, line.c_str(), (int)line.size(), 0);
		}
	}

	void SideChannelServer::BroadcastEvent(const nlohmann::json& msg)
	{
		std::string line = msg.dump() + "\n";
		std::lock_guard<std::recursive_mutex> lock(m_peersMutex);
		for (auto& [sock, peer] : m_peers)
		{
			if (peer.subscribed)
				send(sock, line.c_str(), (int)line.size(), 0);
		}
	}

	void SideChannelServer::BroadcastToMods(const nlohmann::json& msg)
	{
		std::string line = msg.dump() + "\n";
		std::lock_guard<std::recursive_mutex> lock(m_peersMutex);
		for (auto& [sock, peer] : m_peers)
		{
			if (peer.authenticated && peer.isModerator)
				send(sock, line.c_str(), (int)line.size(), 0);
		}
	}

	void SideChannelServer::SendTo(const std::string& playerName, const nlohmann::json& msg)
	{
		std::string line = msg.dump() + "\n";
		std::lock_guard<std::recursive_mutex> lock(m_peersMutex);
		for (auto& [sock, peer] : m_peers)
		{
			if (peer.authenticated && peer.name == playerName)
			{
				send(sock, line.c_str(), (int)line.size(), 0);
				return;
			}
		}
	}

	void SideChannelServer::SendToPeer(SideChannelPeer& peer, const nlohmann::json& msg)
	{
		std::string line = msg.dump() + "\n";
		send(peer.sock, line.c_str(), (int)line.size(), 0);
	}

	void SideChannelServer::SetHandler(const std::string& type, SideChannelHandler handler)
	{
		std::lock_guard<std::mutex> lock(m_handlersMutex);
		m_handlers[type] = handler;
	}

	void SideChannelServer::AddModerator(const std::string& accountId)
	{
		std::string hashed = HashAccountId(accountId);
		for (const auto& h : m_moderatorAccountIds)
			if (h == hashed) return;
		m_moderatorAccountIds.push_back(hashed);

		// update connected peers
		std::lock_guard<std::recursive_mutex> lock(m_peersMutex);
		for (auto& [sock, peer] : m_peers)
		{
			if (peer.accountId == accountId)
			{
				peer.isModerator = true;
				SendToPeer(peer, { {"type", "moderatorStatus"}, {"moderator", true} });
			}
		}
	}

	void SideChannelServer::RemoveModerator(const std::string& accountId)
	{
		std::string hashed = HashAccountId(accountId);
		m_moderatorAccountIds.erase(
			std::remove(m_moderatorAccountIds.begin(), m_moderatorAccountIds.end(), hashed),
			m_moderatorAccountIds.end());

		std::lock_guard<std::recursive_mutex> lock(m_peersMutex);
		for (auto& [sock, peer] : m_peers)
		{
			if (peer.accountId == accountId)
			{
				peer.isModerator = false;
				SendToPeer(peer, { {"type", "moderatorStatus"}, {"moderator", false} });
			}
		}
	}

	bool SideChannelServer::IsModerator(const std::string& accountId) const
	{
		std::string hashed = HashAccountId(accountId);
		for (const auto& h : m_moderatorAccountIds)
			if (h == hashed) return true;
		return false;
	}

	bool SideChannelServer::LoadModerators(const std::string& path)
	{
		std::ifstream file(path);
		if (!file.is_open()) return false;

		try
		{
			auto j = nlohmann::json::parse(file);
			m_moderatorAccountIds.clear();
			for (const auto& entry : j)
			{
				if (entry.is_string())
					m_moderatorAccountIds.push_back(entry.get<std::string>());
			}
			CYPRESS_LOGMESSAGE(LogLevel::Info, "SideChannel: Loaded {} moderator(s)", m_moderatorAccountIds.size());
			return true;
		}
		catch (const std::exception& e)
		{
			CYPRESS_LOGMESSAGE(LogLevel::Error, "SideChannel: Failed to load moderators: {}", e.what());
			return false;
		}
	}

	bool SideChannelServer::SaveModerators(const std::string& path) const
	{
		std::ofstream file(path);
		if (!file.is_open()) return false;

		nlohmann::json j = m_moderatorAccountIds;
		file << j.dump(2);
		return file.good();
	}

	std::vector<std::pair<std::string, std::string>> SideChannelServer::GetConnectedPeers() const
	{
		std::vector<std::pair<std::string, std::string>> result;
		std::lock_guard<std::recursive_mutex> lock(m_peersMutex);
		for (const auto& [sock, peer] : m_peers)
		{
			if (peer.authenticated)
				result.emplace_back(peer.name, peer.hwid);
		}
		return result;
	}

	void SideChannelServer::UpdatePlayerNamesCache()
	{
		if (!m_playerNamesCb) return;
		auto names = m_playerNamesCb();
		std::lock_guard<std::mutex> lock(m_playerNamesMutex);
		m_cachedPlayerNames = std::move(names);
	}

	std::vector<std::string> SideChannelServer::GetCachedPlayerNames() const
	{
		std::lock_guard<std::mutex> lock(m_playerNamesMutex);
		return m_cachedPlayerNames;
	}

	std::optional<SideChannelPeer> SideChannelServer::FindPeerByName(const std::string& name)
	{
		std::lock_guard<std::recursive_mutex> lock(m_peersMutex);
		for (auto& [sock, peer] : m_peers)
		{
			if (peer.authenticated && peer.name == name)
				return peer;
		}
		return std::nullopt;
	}

	bool SideChannelServer::HasPeerByName(const std::string& name)
	{
		std::lock_guard<std::recursive_mutex> lock(m_peersMutex);
		for (auto& [sock, peer] : m_peers)
		{
			if (peer.authenticated && peer.name == name)
				return true;
		}
		return false;
	}

	SideChannelClient::SideChannelClient() {}

	SideChannelClient::~SideChannelClient()
	{
		Disconnect();
	}

	bool SideChannelClient::Connect(const std::string& serverIP, int port)
	{
		if (m_connected) return true;

		if (port <= 0) port = GetSideChannelPort();

		m_sock = socket(AF_INET, SOCK_STREAM, IPPROTO_TCP);
		if (m_sock == INVALID_SOCKET)
		{
			CYPRESS_LOGMESSAGE(LogLevel::Error, "SideChannel: Failed to create socket ({})", WSAGetLastError());
			return false;
		}

		int nodelay = 1;
		setsockopt(m_sock, IPPROTO_TCP, TCP_NODELAY, (const char*)&nodelay, sizeof(nodelay));

		// tcp keepalive so connection survives idle periods through relay/NAT
		BOOL keepAlive = TRUE;
		setsockopt(m_sock, SOL_SOCKET, SO_KEEPALIVE, (const char*)&keepAlive, sizeof(keepAlive));
		DWORD keepIdle = 30000, keepInterval = 10000;
		setsockopt(m_sock, IPPROTO_TCP, TCP_KEEPIDLE, (const char*)&keepIdle, sizeof(keepIdle));
		setsockopt(m_sock, IPPROTO_TCP, TCP_KEEPINTVL, (const char*)&keepInterval, sizeof(keepInterval));

		// 5s connection timeout
		DWORD timeout = 5000;
		setsockopt(m_sock, SOL_SOCKET, SO_RCVTIMEO, (const char*)&timeout, sizeof(timeout));
		setsockopt(m_sock, SOL_SOCKET, SO_SNDTIMEO, (const char*)&timeout, sizeof(timeout));

		sockaddr_in addr = {};
		addr.sin_family = AF_INET;
		addr.sin_port = htons(port);

		if (inet_pton(AF_INET, serverIP.c_str(), &addr.sin_addr) != 1)
		{
			// try hostname resolution
			addrinfo hints = {};
			hints.ai_family = AF_INET;
			hints.ai_socktype = SOCK_STREAM;
			addrinfo* result = nullptr;
			if (getaddrinfo(serverIP.c_str(), std::to_string(port).c_str(), &hints, &result) != 0 || !result)
			{
				CYPRESS_LOGMESSAGE(LogLevel::Error, "SideChannel: Cannot resolve {}", serverIP);
				closesocket(m_sock);
				m_sock = INVALID_SOCKET;
				return false;
			}
			addr = *(sockaddr_in*)result->ai_addr;
			freeaddrinfo(result);
		}

		if (connect(m_sock, (sockaddr*)&addr, sizeof(addr)) == SOCKET_ERROR)
		{
			CYPRESS_LOGMESSAGE(LogLevel::Warning, "SideChannel: Connect to {}:{} failed ({})", serverIP, port, WSAGetLastError());
			closesocket(m_sock);
			m_sock = INVALID_SOCKET;
			return false;
		}

		// back to blocking for recv
		timeout = 0;
		setsockopt(m_sock, SOL_SOCKET, SO_RCVTIMEO, (const char*)&timeout, sizeof(timeout));

		m_connected = true;
		m_recvThread = std::thread(&SideChannelClient::RecvLoop, this);

		CYPRESS_LOGMESSAGE(LogLevel::Info, "SideChannel: Connected to {}:{}", serverIP, port);
		return true;
	}

	void SideChannelClient::Disconnect()
	{
		if (!m_connected) return;
		m_connected = false;
		m_isModerator = false;

		if (m_sock != INVALID_SOCKET)
		{
			closesocket(m_sock);
			m_sock = INVALID_SOCKET;
		}

		if (m_recvThread.joinable())
			m_recvThread.join();
	}

	void SideChannelClient::ForceClose()
	{
		m_connected = false;
		m_isModerator = false;
		if (m_sock != INVALID_SOCKET)
		{
			closesocket(m_sock);
			m_sock = INVALID_SOCKET;
		}
	}

	void SideChannelClient::Send(const nlohmann::json& msg)
	{
		if (!m_connected || m_sock == INVALID_SOCKET) return;
		std::string line = msg.dump() + "\n";
		std::lock_guard<std::mutex> lock(m_sendMutex);
		::send(m_sock, line.c_str(), (int)line.size(), 0);
	}

	void SideChannelClient::SetHandler(const std::string& type, SideChannelHandler handler)
	{
		std::lock_guard<std::mutex> lock(m_handlersMutex);
		m_handlers[type] = handler;
	}

	void SideChannelClient::RecvLoop()
	{
		char buf[4096];
		std::string recvBuf;

		while (m_connected)
		{
			int bytesRead = recv(m_sock, buf, sizeof(buf) - 1, 0);
			if (bytesRead <= 0) break;

			buf[bytesRead] = '\0';
			recvBuf += buf;

			size_t pos;
			while ((pos = recvBuf.find('\n')) != std::string::npos)
			{
				std::string line = recvBuf.substr(0, pos);
				recvBuf.erase(0, pos + 1);
				if (!line.empty())
					ProcessLine(line);
			}
		}

		m_connected = false;
		CYPRESS_LOGMESSAGE(LogLevel::Info, "SideChannel: Disconnected from server");
	}

	void SideChannelClient::ProcessLine(const std::string& line)
	{
		try
		{
			auto msg = nlohmann::json::parse(line);
			std::string type = msg.value("type", "");

			// keepalive ping from server
			if (type == "ping")
			{
				Send({ {"type", "pong"} });
				return;
			}

			// handle challenge from server, compute proof and send deferred auth
			if (type == "challenge")
			{
				std::string nonce = msg.value("nonce", "");
				if (!nonce.empty() && m_pendingAuth.contains("type"))
				{
					std::string hwid = m_pendingAuth.value("hwid", "");
					std::string proof = HmacSha256(nonce, hwid);
					m_pendingAuth["proof"] = proof;
					Send(m_pendingAuth);
					m_pendingAuth = {}; // consumed
					CYPRESS_LOGMESSAGE(LogLevel::Debug, "SideChannel: Responded to auth challenge");
				}
				return;
			}

			// identity challenge, sign nonce with ed25519 private key to prove ownership
			if (type == "identityChallenge")
			{
				std::string nonce = msg.value("nonce", "");
				if (nonce.empty()) return;

				char keyBuf[256] = {};
				if (GetEnvironmentVariableA("CYPRESS_IDENTITY_KEY", keyBuf, sizeof(keyBuf)) == 0)
				{
					CYPRESS_LOGMESSAGE(LogLevel::Warning, "SideChannel: Got identity challenge but no key available");
					return;
				}

				auto keyBytes = Cypress::Identity::detail::hex_decode(keyBuf);
				// nsec exports 32-byte seed, ed25519 needs 64-byte expanded key
				// monocypher can sign with seed via crypto_eddsa_key_pair but we need to use the proper ed25519 variant
				// instead, compute public key from seed and sign with raw ed25519
				if (keyBytes.size() == 32)
				{
					// derive keypair from seed, sha512 then clamp
					uint8_t expanded[64] = {};
					Cypress::Identity::detail::sha512(keyBytes.data(), 32, expanded);
					// clamp
					expanded[0] &= 248;
					expanded[31] &= 127;
					expanded[31] |= 64;

					// compute public key from clamped scalar
					uint8_t pubKey[32] = {};
					crypto_eddsa_scalarbase(pubKey, expanded);

					// ed25519 sign: (R, S) where R = r*B, S = r + H(R||A||M)*a mod L
					// use SHA-512 for ed25519
					// r = SHA-512(expanded[32..63] || message)
					uint8_t rHash[64] = {};
					{
						BCRYPT_ALG_HANDLE hAlg = nullptr;
						BCRYPT_HASH_HANDLE hHash = nullptr;
						BCryptOpenAlgorithmProvider(&hAlg, BCRYPT_SHA512_ALGORITHM, nullptr, 0);
						BCryptCreateHash(hAlg, &hHash, nullptr, 0, nullptr, 0, 0);
						BCryptHashData(hHash, expanded + 32, 32, 0);
						BCryptHashData(hHash, (PUCHAR)nonce.data(), (ULONG)nonce.size(), 0);
						BCryptFinishHash(hHash, rHash, 64, 0);
						BCryptDestroyHash(hHash);
						BCryptCloseAlgorithmProvider(hAlg, 0);
					}

					uint8_t rReduced[32];
					crypto_eddsa_reduce(rReduced, rHash);

					uint8_t R[32];
					crypto_eddsa_scalarbase(R, rReduced);

					// H(R || A || M)
					uint8_t hram[64] = {};
					{
						BCRYPT_ALG_HANDLE hAlg = nullptr;
						BCRYPT_HASH_HANDLE hHash = nullptr;
						BCryptOpenAlgorithmProvider(&hAlg, BCRYPT_SHA512_ALGORITHM, nullptr, 0);
						BCryptCreateHash(hAlg, &hHash, nullptr, 0, nullptr, 0, 0);
						BCryptHashData(hHash, R, 32, 0);
						BCryptHashData(hHash, pubKey, 32, 0);
						BCryptHashData(hHash, (PUCHAR)nonce.data(), (ULONG)nonce.size(), 0);
						BCryptFinishHash(hHash, hram, 64, 0);
						BCryptDestroyHash(hHash);
						BCryptCloseAlgorithmProvider(hAlg, 0);
					}

					uint8_t hramReduced[32];
					crypto_eddsa_reduce(hramReduced, hram);

					// S = (r + H(R||A||M) * a) mod L
					uint8_t sig[64];
					memcpy(sig, R, 32);
					crypto_eddsa_mul_add(sig + 32, hramReduced, expanded, rReduced);

					std::string sigHex = Cypress::Identity::detail::hex_encode(sig, 64);
					std::string pubKeyHex = Cypress::Identity::detail::hex_encode(pubKey, 32);

					Send({ {"type", "identityProof"}, {"sig", sigHex}, {"public_key", pubKeyHex} });
					CYPRESS_LOGMESSAGE(LogLevel::Debug, "SideChannel: Responded to identity challenge");

					// clear the key from memory
					SecureZeroMemory(keyBuf, sizeof(keyBuf));
					SecureZeroMemory(keyBytes.data(), keyBytes.size());
					SecureZeroMemory(expanded, sizeof(expanded));
				}
				return;
			}

			// mod challenge from server, HMAC our token with their nonce, never send the raw token
			if (type == "modChallenge")
			{
				std::string nonce = msg.value("nonce", "");
				if (nonce.empty() || m_modToken.empty())
				{
					CYPRESS_LOGMESSAGE(LogLevel::Warning, "SideChannel: Got mod challenge but no token available");
					return;
				}
				std::string sig = HmacSha256(m_modToken, nonce);
				Send({ {"type", "modChallengeResponse"}, {"sig", sig} });
				CYPRESS_LOGMESSAGE(LogLevel::Debug, "SideChannel: Responded to mod challenge");
				return;
			}

			if (type == "authResult")
			{
				bool ok = msg.value("ok", false);
				m_isModerator = msg.value("moderator", false);
				if (ok)
					CYPRESS_LOGMESSAGE(LogLevel::Info, "SideChannel: Authenticated{}", m_isModerator ? " (moderator)" : "");
				else
				{
					std::string reason = msg.value("msg", "unknown");
					CYPRESS_LOGMESSAGE(LogLevel::Warning, "SideChannel: Authentication failed: {}", reason);
				}

				// tell launcher about mod status
				if (Cypress_IsEmbeddedMode())
					Cypress_EmitJsonPlayerEvent("modStatus", m_isModerator ? 1 : 0, "", nullptr);

				// if we have a stored mod token but server didn't recognize us as mod,
				// trigger the challenge-response flow now that we're authenticated
				if (ok && !m_isModerator && !m_modToken.empty())
				{
					CYPRESS_LOGMESSAGE(LogLevel::Info, "SideChannel: Have stored mod token, requesting challenge");
					Send({ {"type", "modTokenUpdate"} });
				}

				// if we're a mod, explicitly request the full player list
				if (ok && m_isModerator)
					Send({ {"type", "requestPlayerList"} });
			}
			else if (type == "moderatorStatus")
			{
				m_isModerator = msg.value("moderator", false);
				CYPRESS_LOGMESSAGE(LogLevel::Info, "SideChannel: Moderator status changed: {}", m_isModerator);

				if (Cypress_IsEmbeddedMode())
					Cypress_EmitJsonPlayerEvent("modStatus", m_isModerator ? 1 : 0, "", nullptr);

				if (m_isModerator)
					Send({ {"type", "requestPlayerList"} });
			}
			// forward player events to launcher for mod ui
			else if (type == "scPlayerJoin" || type == "scPlayerLeave" || type == "scPlayerList" || type == "scPlayerAuth" || type == "scPlayerState" || type == "scModBans")
			{
				if (Cypress_IsEmbeddedMode())
				{
					// launcher expects "t" not "type"
					nlohmann::json reMsg = msg;
					reMsg["t"] = reMsg["type"];
					reMsg.erase("type");
					std::string reEmit = reMsg.dump() + "\n";
					Cypress_WriteRawStdout(reEmit);
				}
			}

			// dispatch to handlers
			{
				std::lock_guard<std::mutex> lock(m_handlersMutex);
				auto it = m_handlers.find(type);
				if (it != m_handlers.end())
				{
					it->second(msg, m_self);
				}
			}
		}
		catch (const std::exception& e)
		{
			CYPRESS_LOGMESSAGE(LogLevel::Warning, "SideChannel: Bad message from server: {}", e.what());
		}
	}

	SideChannelClientListener::SideChannelClientListener() {}

	SideChannelClientListener::~SideChannelClientListener()
	{
		Stop();
		for (auto& t : m_clientThreads)
		{
			if (t.joinable()) t.detach();
		}
	}

	bool SideChannelClientListener::Start(int port)
	{
		if (m_running) return true;

		m_listenSock = socket(AF_INET, SOCK_STREAM, IPPROTO_TCP);
		if (m_listenSock == INVALID_SOCKET) return false;

		// always use os-assigned port to avoid colliding with a server on the same machine
		// the discovery file tells the launcher what port we actually got
		sockaddr_in addr = {};
		addr.sin_family = AF_INET;
		addr.sin_addr.s_addr = htonl(INADDR_LOOPBACK);
		addr.sin_port = htons(0);

		if (bind(m_listenSock, (sockaddr*)&addr, sizeof(addr)) == SOCKET_ERROR)
		{
			closesocket(m_listenSock);
			m_listenSock = INVALID_SOCKET;
			return false;
		}

		sockaddr_in bound = {};
		int boundLen = sizeof(bound);
		getsockname(m_listenSock, (sockaddr*)&bound, &boundLen);
		m_port = ntohs(bound.sin_port);

		if (listen(m_listenSock, 4) == SOCKET_ERROR)
		{
			closesocket(m_listenSock);
			m_listenSock = INVALID_SOCKET;
			return false;
		}

		m_running = true;
		m_acceptThread = std::thread(&SideChannelClientListener::AcceptLoop, this);
		CYPRESS_LOGMESSAGE(LogLevel::Info, "SideChannel: Client listener on port {}", m_port);
		return true;
	}

	void SideChannelClientListener::Stop()
	{
		if (!m_running) return;
		m_running = false;

		if (m_listenSock != INVALID_SOCKET)
		{
			closesocket(m_listenSock);
			m_listenSock = INVALID_SOCKET;
		}

		{
			std::lock_guard<std::recursive_mutex> lock(m_peersMutex);
			for (auto& [sock, peer] : m_peers)
				closesocket(sock);
			m_peers.clear();
		}

		if (m_acceptThread.joinable())
			m_acceptThread.join();
	}

	void SideChannelClientListener::AcceptLoop()
	{
		while (m_running)
		{
			SOCKET clientSock = accept(m_listenSock, nullptr, nullptr);
			if (clientSock == INVALID_SOCKET) continue;

			int nodelay = 1;
			setsockopt(clientSock, IPPROTO_TCP, TCP_NODELAY, (const char*)&nodelay, sizeof(nodelay));

			{
				std::lock_guard<std::recursive_mutex> lock(m_peersMutex);
				m_peers[clientSock] = SideChannelPeer{ clientSock };
			}

			m_clientThreads.emplace_back(&SideChannelClientListener::ClientLoop, this, clientSock);
		}
	}

	void SideChannelClientListener::ClientLoop(SOCKET clientSock)
	{
		char buf[4096];
		while (m_running)
		{
			int bytesRead = recv(clientSock, buf, sizeof(buf) - 1, 0);
			if (bytesRead <= 0) break;

			buf[bytesRead] = '\0';

			std::lock_guard<std::recursive_mutex> lock(m_peersMutex);
			auto it = m_peers.find(clientSock);
			if (it == m_peers.end()) break;

			SideChannelPeer& peer = it->second;
			peer.recvBuf += buf;

			size_t pos;
			while ((pos = peer.recvBuf.find('\n')) != std::string::npos)
			{
				std::string line = peer.recvBuf.substr(0, pos);
				peer.recvBuf.erase(0, pos + 1);
				if (!line.empty())
					ProcessLine(peer, line);
			}
		}

		RemovePeer(clientSock);
	}

	void SideChannelClientListener::ProcessLine(SideChannelPeer& peer, const std::string& line)
	{
		try
		{
			auto msg = nlohmann::json::parse(line);
			std::string type = msg.value("type", "");

			// serverInfo, no auth needed
			if (type == "serverInfo")
			{
				nlohmann::json response = {
					{"type", "serverInfo"},
#ifdef CYPRESS_GW1
					{"game", "GW1"},
#elif defined(CYPRESS_BFN)
					{"game", "BFN"},
#else
					{"game", "GW2"},
#endif
					{"isClient", true},
					{"port", m_port},
					{"isModerator", m_client ? m_client->IsModerator() : false}
				};
				std::string out = response.dump() + "\n";
				send(peer.sock, out.c_str(), (int)out.size(), 0);
				return;
			}

			// subscribe to events
			if (type == "subscribe")
			{
				peer.subscribed = true;
				std::string out = "{\"type\":\"subscribed\",\"ok\":true}\n";
				send(peer.sock, out.c_str(), (int)out.size(), 0);
				return;
			}

			// forward mod commands to server
			if (m_client && m_client->IsConnected() && m_client->IsModerator())
			{
				if (type == "modKick" || type == "modBan" || type == "modCommand" || type == "modFreecam"
					|| type == "addMod" || type == "removeMod")
				{
					m_client->Send(msg);
					return;
				}
			}
		}
		catch (const std::exception& e)
		{
			CYPRESS_LOGMESSAGE(LogLevel::Warning, "SideChannel: ClientListener bad message: {}", e.what());
		}
	}

	void SideChannelClientListener::RemovePeer(SOCKET sock)
	{
		std::lock_guard<std::recursive_mutex> lock(m_peersMutex);
		auto it = m_peers.find(sock);
		if (it != m_peers.end())
		{
			closesocket(sock);
			m_peers.erase(it);
		}
	}

	void SideChannelClientListener::BroadcastEvent(const nlohmann::json& msg)
	{
		std::string line = msg.dump() + "\n";
		std::lock_guard<std::recursive_mutex> lock(m_peersMutex);
		for (auto& [sock, peer] : m_peers)
		{
			if (peer.subscribed)
				send(sock, line.c_str(), (int)line.size(), 0);
		}
	}

	// -- SideChannelTunnel --
	// frame format: [1B cmd][4B client_id BE][4B data_len BE][data]
	// cmd: 1=OPEN, 2=DATA, 3=CLOSE

	static constexpr uint8_t TCMD_OPEN = 1;
	static constexpr uint8_t TCMD_DATA = 2;
	static constexpr uint8_t TCMD_CLOSE = 3;
	static constexpr uint8_t TCMD_UDP = 4;
	static constexpr uint8_t TCMD_PING = 5;
	static constexpr uint8_t TCMD_PONG = 6;

	SideChannelTunnel::SideChannelTunnel() {}
	SideChannelTunnel::~SideChannelTunnel() { Stop(); }

	bool SideChannelTunnel::RecvExact(char* buf, int len)
	{
		int total = 0;
		while (total < len)
		{
			int n = recv(m_tunnelSock, buf + total, len - total, 0);
			if (n <= 0) return false;
			total += n;
		}
		return true;
	}

	void SideChannelTunnel::SendFrame(uint8_t cmd, uint32_t clientId, const char* data, uint32_t dataLen)
	{
		uint8_t header[9];
		header[0] = cmd;
		uint32_t cidBE = htonl(clientId);
		uint32_t lenBE = htonl(dataLen);
		memcpy(header + 1, &cidBE, 4);
		memcpy(header + 5, &lenBE, 4);

		std::lock_guard<std::mutex> lock(m_writeMutex);
		::send(m_tunnelSock, (const char*)header, 9, 0);
		if (data && dataLen > 0)
			::send(m_tunnelSock, data, dataLen, 0);
	}

	bool SideChannelTunnel::Start(const std::string& relayHost, int relayPort,
		const std::string& proxyKey, int localPort)
	{
		if (m_running) return true;
		m_localPort = localPort;

		m_tunnelSock = socket(AF_INET, SOCK_STREAM, IPPROTO_TCP);
		if (m_tunnelSock == INVALID_SOCKET)
		{
			CYPRESS_LOGMESSAGE(LogLevel::Error, "SideChannelTunnel: Failed to create socket ({})", WSAGetLastError());
			return false;
		}

		int nodelay = 1;
		setsockopt(m_tunnelSock, IPPROTO_TCP, TCP_NODELAY, (const char*)&nodelay, sizeof(nodelay));

		// enable tcp keepalive so the OS detects dead connections
		BOOL keepAlive = TRUE;
		setsockopt(m_tunnelSock, SOL_SOCKET, SO_KEEPALIVE, (const char*)&keepAlive, sizeof(keepAlive));
		DWORD keepIdle = 30000, keepInterval = 10000;
		setsockopt(m_tunnelSock, IPPROTO_TCP, TCP_KEEPIDLE, (const char*)&keepIdle, sizeof(keepIdle));
		setsockopt(m_tunnelSock, IPPROTO_TCP, TCP_KEEPINTVL, (const char*)&keepInterval, sizeof(keepInterval));

		DWORD timeout = 5000;
		setsockopt(m_tunnelSock, SOL_SOCKET, SO_RCVTIMEO, (const char*)&timeout, sizeof(timeout));
		setsockopt(m_tunnelSock, SOL_SOCKET, SO_SNDTIMEO, (const char*)&timeout, sizeof(timeout));

		sockaddr_in addr = {};
		addr.sin_family = AF_INET;
		addr.sin_port = htons(relayPort);

		if (inet_pton(AF_INET, relayHost.c_str(), &addr.sin_addr) != 1)
		{
			addrinfo hints = {};
			hints.ai_family = AF_INET;
			hints.ai_socktype = SOCK_STREAM;
			addrinfo* result = nullptr;
			if (getaddrinfo(relayHost.c_str(), std::to_string(relayPort).c_str(), &hints, &result) != 0 || !result)
			{
				CYPRESS_LOGMESSAGE(LogLevel::Error, "SideChannelTunnel: Cannot resolve {}", relayHost);
				closesocket(m_tunnelSock);
				m_tunnelSock = INVALID_SOCKET;
				return false;
			}
			addr = *(sockaddr_in*)result->ai_addr;
			freeaddrinfo(result);
		}

		if (connect(m_tunnelSock, (sockaddr*)&addr, sizeof(addr)) == SOCKET_ERROR)
		{
			CYPRESS_LOGMESSAGE(LogLevel::Error, "SideChannelTunnel: Connect to {}:{} failed ({})",
				relayHost, relayPort, WSAGetLastError());
			closesocket(m_tunnelSock);
			m_tunnelSock = INVALID_SOCKET;
			return false;
		}

		// back to blocking for recv
		timeout = 0;
		setsockopt(m_tunnelSock, SOL_SOCKET, SO_RCVTIMEO, (const char*)&timeout, sizeof(timeout));

		// send register handshake (JSON line, then binary frames)
		nlohmann::json handshake = { {"type", "register"}, {"key", proxyKey} };
		std::string line = handshake.dump() + "\n";
		::send(m_tunnelSock, line.c_str(), (int)line.size(), 0);

		// open a local UDP socket so the SocketManager can send game data through the tunnel
		m_udpBridgeSock = socket(AF_INET, SOCK_DGRAM, IPPROTO_UDP);
		if (m_udpBridgeSock != INVALID_SOCKET)
		{
			sockaddr_in bridgeAddr = {};
			bridgeAddr.sin_family = AF_INET;
			bridgeAddr.sin_addr.s_addr = htonl(INADDR_ANY);
			bridgeAddr.sin_port = 0; // let OS pick
			if (bind(m_udpBridgeSock, (sockaddr*)&bridgeAddr, sizeof(bridgeAddr)) == 0)
			{
				sockaddr_in bound = {};
				int boundLen = sizeof(bound);
				getsockname(m_udpBridgeSock, (sockaddr*)&bound, &boundLen);
				m_udpBridgePort = ntohs(bound.sin_port);

				// figure out where the game's SocketManager will listen
				m_gameUdpPort = 25200;
				{
					char* ppVal = nullptr;
					size_t ppLen = 0;
					if (_dupenv_s(&ppVal, &ppLen, "CYPRESS_PROXY_PORT") == 0 && ppVal)
					{
						int parsed = atoi(ppVal);
						if (parsed > 0) m_gameUdpPort = parsed;
						free(ppVal);
					}
				}

				// redirect the SocketManager to send to our local bridge instead of the remote relay
				// the server socket binds to a specific interface (e.g. 172.30.160.1)
				// and can only sendto addresses on that same subnet.
				// we don't know the bind address yet, so we set a flag env var.
				// GetProxyAddress will detect it and use the socket's own bind address.
				char portBuf[16];
				snprintf(portBuf, sizeof(portBuf), "%d", m_udpBridgePort);
				_putenv_s("CYPRESS_BRIDGE_PORT", portBuf);

				CYPRESS_LOGMESSAGE(LogLevel::Info, "SideChannelTunnel: UDP bridge on 0.0.0.0:{}", m_udpBridgePort);

				_putenv_s("CYPRESS_PROXY_PORT", portBuf);
			}
			else
			{
				closesocket(m_udpBridgeSock);
				m_udpBridgeSock = INVALID_SOCKET;
			}
		}

		m_running = true;
		m_thread = std::thread(&SideChannelTunnel::TunnelLoop, this);
		if (m_udpBridgeSock != INVALID_SOCKET)
			m_udpBridgeThread = std::thread(&SideChannelTunnel::UdpBridgeLoop, this);

		CYPRESS_LOGMESSAGE(LogLevel::Info, "SideChannelTunnel: Connected to {}:{}", relayHost, relayPort);
		return true;
	}

	void SideChannelTunnel::Stop()
	{
		m_running = false;

		if (m_tunnelSock != INVALID_SOCKET)
		{
			closesocket(m_tunnelSock);
			m_tunnelSock = INVALID_SOCKET;
		}

		if (m_udpBridgeSock != INVALID_SOCKET)
		{
			closesocket(m_udpBridgeSock);
			m_udpBridgeSock = INVALID_SOCKET;
		}

		if (m_udpInjectSock != INVALID_SOCKET)
		{
			closesocket(m_udpInjectSock);
			m_udpInjectSock = INVALID_SOCKET;
		}

		// close all local sockets so reader threads exit
		{
			std::lock_guard<std::mutex> lock(m_clientsMutex);
			for (auto& [id, sock] : m_localClients)
				closesocket(sock);
			m_localClients.clear();
		}

		if (m_thread.joinable()) m_thread.join();
		if (m_udpBridgeThread.joinable()) m_udpBridgeThread.join();
	}

	void SideChannelTunnel::TunnelLoop()
	{
		while (m_running)
		{
			uint8_t header[9];
			if (!RecvExact((char*)header, 9))
				break;

			uint8_t cmd = header[0];
			uint32_t clientId, dataLen;
			memcpy(&clientId, header + 1, 4);
			memcpy(&dataLen, header + 5, 4);
			clientId = ntohl(clientId);
			dataLen = ntohl(dataLen);

			std::vector<char> data;
			if (dataLen > 0)
			{
				if (dataLen > 1024 * 1024) break; // sanity limit
				data.resize(dataLen);
				if (!RecvExact(data.data(), dataLen))
					break;
			}

			switch (cmd)
			{
			case TCMD_OPEN:
			{
				// create local tcp connection to side-channel server
				SOCKET localSock = socket(AF_INET, SOCK_STREAM, IPPROTO_TCP);
				if (localSock == INVALID_SOCKET)
				{
					SendFrame(TCMD_CLOSE, clientId);
					break;
				}

				int nd = 1;
				setsockopt(localSock, IPPROTO_TCP, TCP_NODELAY, (const char*)&nd, sizeof(nd));

				sockaddr_in localAddr = {};
				localAddr.sin_family = AF_INET;
				localAddr.sin_port = htons(m_localPort);
				localAddr.sin_addr.s_addr = htonl(INADDR_LOOPBACK);

				if (connect(localSock, (sockaddr*)&localAddr, sizeof(localAddr)) == SOCKET_ERROR)
				{
					CYPRESS_LOGMESSAGE(LogLevel::Warning, "SideChannelTunnel: Local connect failed for cid={}", clientId);
					closesocket(localSock);
					SendFrame(TCMD_CLOSE, clientId);
					break;
				}

				{
					std::lock_guard<std::mutex> lock(m_clientsMutex);
					m_localClients[clientId] = localSock;
				}

				// spawn reader thread (detached, self-managing)
				std::thread(&SideChannelTunnel::ClientReadLoop, this, clientId, localSock).detach();
				CYPRESS_DEBUG_LOGMESSAGE(LogLevel::Debug, "SideChannelTunnel: Opened local connection for cid={}", clientId);
				break;
			}
			case TCMD_DATA:
			{
				std::lock_guard<std::mutex> lock(m_clientsMutex);
				auto it = m_localClients.find(clientId);
				if (it != m_localClients.end())
					::send(it->second, data.data(), (int)dataLen, 0);
				break;
			}
			case TCMD_CLOSE:
			{
				SOCKET sock = INVALID_SOCKET;
				{
					std::lock_guard<std::mutex> lock(m_clientsMutex);
					auto it = m_localClients.find(clientId);
					if (it != m_localClients.end())
					{
						sock = it->second;
						m_localClients.erase(it);
					}
				}
				if (sock != INVALID_SOCKET)
					closesocket(sock);
				break;
			}
			case TCMD_UDP:
			{
				// relay forwarded a client's game UDP packet to us, inject into local SocketManager
				if (dataLen > 0 && m_haveGameServerAddr)
				{
					if (m_udpInjectSock == INVALID_SOCKET)
						m_udpInjectSock = socket(AF_INET, SOCK_DGRAM, IPPROTO_UDP);
					if (m_udpInjectSock != INVALID_SOCKET)
					{
						int sent = sendto(m_udpInjectSock, data.data(), (int)dataLen, 0,
							(sockaddr*)&m_gameServerAddr, sizeof(m_gameServerAddr));
						static int udpInjectCount = 0;
						udpInjectCount++;
						if (udpInjectCount <= 10)
						{
							char ipBuf[INET_ADDRSTRLEN] = {};
							inet_ntop(AF_INET, &m_gameServerAddr.sin_addr, ipBuf, sizeof(ipBuf));
							CYPRESS_LOGMESSAGE(LogLevel::Info, "TCMD_UDP inject #{}: {} bytes -> {}:{}, sent={}",
								udpInjectCount, dataLen, ipBuf, ntohs(m_gameServerAddr.sin_port), sent);
						}
					}
				}
				else
				{
					CYPRESS_LOGMESSAGE(LogLevel::Warning, "TCMD_UDP skipped: dataLen={} haveAddr={}", dataLen, m_haveGameServerAddr);
				}
				break;
			}
			case TCMD_PING:
			{
				// respond with pong to keep connection alive
				SendFrame(TCMD_PONG, 0);
				break;
			}
			}
		}

		m_running = false;
		CYPRESS_LOGMESSAGE(LogLevel::Info, "SideChannelTunnel: Disconnected");
	}

	void SideChannelTunnel::ClientReadLoop(uint32_t clientId, SOCKET localSock)
	{
		char buf[4096];
		while (m_running)
		{
			int n = recv(localSock, buf, sizeof(buf), 0);
			if (n <= 0) break;
			SendFrame(TCMD_DATA, clientId, buf, (uint32_t)n);
		}

		// tell relay this client is done
		if (m_running)
			SendFrame(TCMD_CLOSE, clientId);

		// clean up
		{
			std::lock_guard<std::mutex> lock(m_clientsMutex);
			auto it = m_localClients.find(clientId);
			if (it != m_localClients.end())
			{
				closesocket(it->second);
				m_localClients.erase(it);
			}
		}
	}

	void SideChannelTunnel::UdpBridgeLoop()
	{
		CYPRESS_LOGMESSAGE(LogLevel::Info, "SideChannelTunnel: UdpBridgeLoop started, sock={}, port={}", (int)m_udpBridgeSock, m_udpBridgePort);
		char buf[65536];
		static const char regPrefix[] = "CYPRESS_PROXY_REGISTER|SERVER|";
		constexpr int regPrefixLen = sizeof(regPrefix) - 1;
		while (m_running && m_udpBridgeSock != INVALID_SOCKET)
		{
			sockaddr_in from = {};
			int fromLen = sizeof(from);
			int n = recvfrom(m_udpBridgeSock, buf, sizeof(buf), 0, (sockaddr*)&from, &fromLen);
			if (n <= 0)
			{
				if (!m_running) break;
				continue;
			}

			// learn the SocketManager's real address from first packet
			if (!m_haveGameServerAddr)
			{
				m_gameServerAddr = from;
				m_haveGameServerAddr = true;
				char ipBuf[INET_ADDRSTRLEN] = {};
				inet_ntop(AF_INET, &from.sin_addr, ipBuf, sizeof(ipBuf));
				CYPRESS_LOGMESSAGE(LogLevel::Info, "SideChannelTunnel: learned game server addr {}:{}", ipBuf, ntohs(from.sin_port));
			}

			// intercept registration packets, respond with ACK locally
			if (n >= regPrefixLen && memcmp(buf, regPrefix, regPrefixLen) == 0)
			{
				const char* ack = "CYPRESS_PROXY_ACK";
				sendto(m_udpBridgeSock, ack, (int)strlen(ack), 0, (sockaddr*)&from, fromLen);
				continue;
			}

			SendFrame(TCMD_UDP, 0, buf, (uint32_t)n);
		}
	}
}
