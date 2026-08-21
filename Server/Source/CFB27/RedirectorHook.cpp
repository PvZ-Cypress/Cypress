#include <winsock2.h>
#include <ws2tcpip.h>
#include <winhttp.h>

#include <CFB27/BridgeConfig.h>
#include <CFB27/BridgeLog.h>
#include <CFB27/RedirectorHook.h>
#include <MinHook.h>

#include <algorithm>
#include <array>
#include <atomic>
#include <cctype>
#include <cstdlib>
#include <fstream>
#include <mutex>
#include <regex>
#include <sstream>
#include <string>
#include <unordered_map>
#include <vector>

namespace
{
	using GetAddrInfoAFunction = INT(WSAAPI*)(PCSTR, PCSTR, const ADDRINFOA*, PADDRINFOA*);
	using GetAddrInfoWFunction = INT(WSAAPI*)(PCWSTR, PCWSTR, const ADDRINFOW*, PADDRINFOW*);
	using ConnectFunction = INT(WSAAPI*)(SOCKET, const sockaddr*, int);
	using CloseSocketFunction = int(WSAAPI*)(SOCKET);
	using WSAConnectFunction = INT(WSAAPI*)(
		SOCKET,
		const sockaddr*,
		int,
		LPWSABUF,
		LPWSABUF,
		LPQOS,
		LPQOS);
	using SendFunction = int(WSAAPI*)(SOCKET, const char*, int, int);
	using RecvFunction = int(WSAAPI*)(SOCKET, char*, int, int);
	using WSASendFunction = int(WSAAPI*)(
		SOCKET,
		LPWSABUF,
		DWORD,
		LPDWORD,
		DWORD,
		LPWSAOVERLAPPED,
		LPWSAOVERLAPPED_COMPLETION_ROUTINE);
	using WSARecvFunction = int(WSAAPI*)(SOCKET, LPWSABUF, DWORD, LPDWORD, LPDWORD, LPWSAOVERLAPPED, LPWSAOVERLAPPED_COMPLETION_ROUTINE);
	using WinHttpConnectFunction = HINTERNET(WINAPI*)(HINTERNET, LPCWSTR, INTERNET_PORT, DWORD);
	using WinHttpOpenRequestFunction = HINTERNET(WINAPI*)(HINTERNET, LPCWSTR, LPCWSTR, LPCWSTR, LPCWSTR, LPCWSTR*, DWORD);
	using WinHttpSendRequestFunction = BOOL(WINAPI*)(
		HINTERNET,
		LPCWSTR,
		DWORD,
		LPVOID,
		DWORD,
		DWORD,
		DWORD_PTR);
	using WinHttpReceiveResponseFunction = BOOL(WINAPI*)(HINTERNET, LPVOID);

	GetAddrInfoAFunction s_originalGetAddrInfoA = nullptr;
	GetAddrInfoWFunction s_originalGetAddrInfoW = nullptr;
	ConnectFunction s_originalConnect = nullptr;
	CloseSocketFunction s_originalCloseSocket = nullptr;
	WSAConnectFunction s_originalWSAConnect = nullptr;
	SendFunction s_originalSend = nullptr;
	RecvFunction s_originalRecv = nullptr;
	WSASendFunction s_originalWSASend = nullptr;
	WSARecvFunction s_originalWSARecv = nullptr;
	WinHttpConnectFunction s_originalWinHttpConnect = nullptr;
	WinHttpOpenRequestFunction s_originalWinHttpOpenRequest = nullptr;
	WinHttpSendRequestFunction s_originalWinHttpSendRequest = nullptr;
	WinHttpReceiveResponseFunction s_originalWinHttpReceiveResponse = nullptr;
	std::string s_bridgeHost = "127.0.0.1";
	std::wstring s_bridgeHostWide = L"127.0.0.1";
	std::uint16_t s_bridgePort = 27920;
	Cypress::CFB27::BridgeLog* s_log = nullptr;
	std::vector<Cypress::CFB27::CandidateEndpoint> s_candidateEndpoints;
	std::once_flag s_dnsLog;
	std::once_flag s_connectLog;
	std::atomic_uint s_dnsTraceCount = 0;
	std::atomic_uint s_connectTraceCount = 0;
	std::atomic_uint s_redirectTraceCount = 0;
	std::atomic_uint s_tlsAlertTraceCount = 0;
	std::atomic_uint s_winHttpTraceCount = 0;
	std::atomic_uint s_closeTraceCount = 0;
	std::atomic_uint s_receiveTraceCount = 0;
	std::mutex s_redirectedSocketsMutex;
	std::unordered_map<SOCKET, std::string> s_redirectedSockets;
	bool s_enableCandidateEndpointRedirects = false;
	bool s_enableRedirectorNetworkRedirect = true;

	std::string Narrow(const wchar_t* value)
	{
		if (!value)
			return {};
		const int required = WideCharToMultiByte(CP_UTF8, 0, value, -1, nullptr, 0, nullptr, nullptr);
		if (required <= 1)
			return {};
		std::string result(static_cast<std::size_t>(required), '\0');
		WideCharToMultiByte(CP_UTF8, 0, value, -1, result.data(), required, nullptr, nullptr);
		result.resize(static_cast<std::size_t>(required - 1));
		return result;
	}

	std::wstring Widen(const std::string& value)
	{
		const int required = MultiByteToWideChar(CP_UTF8, 0, value.c_str(), -1, nullptr, 0);
		if (required <= 1)
			return {};
		std::wstring result(static_cast<std::size_t>(required), L'\0');
		MultiByteToWideChar(CP_UTF8, 0, value.c_str(), -1, result.data(), required);
		result.resize(static_cast<std::size_t>(required - 1));
		return result;
	}

	std::string FormatService(PCSTR serviceName)
	{
		return serviceName ? serviceName : "";
	}

	std::string FormatService(PCWSTR serviceName)
	{
		return Narrow(serviceName);
	}

	std::string FormatIPv4(const sockaddr_in& address)
	{
		char buffer[INET_ADDRSTRLEN] = {};
		inet_ntop(AF_INET, const_cast<IN_ADDR*>(&address.sin_addr), buffer, sizeof(buffer));
		return std::string(buffer) + ":" + std::to_string(ntohs(address.sin_port));
	}

	std::string FormatIPv6(const sockaddr_in6& address)
	{
		char buffer[INET6_ADDRSTRLEN] = {};
		inet_ntop(AF_INET6, const_cast<IN6_ADDR*>(&address.sin6_addr), buffer, sizeof(buffer));
		return "[" + std::string(buffer) + "]:" + std::to_string(ntohs(address.sin6_port));
	}

	void TraceDns(const std::string& nodeName, const std::string& serviceName)
	{
		const unsigned int count = s_dnsTraceCount.fetch_add(1);
		if (!s_log || count >= 128)
			return;
		s_log->Write(
			"dns[" + std::to_string(count + 1) + "] node=" +
			(nodeName.empty() ? "<null>" : nodeName) +
			" service=" + (serviceName.empty() ? "<null>" : serviceName));
	}

	void TraceConnect(const char* apiName, const sockaddr* name, const int nameLength)
	{
		const unsigned int count = s_connectTraceCount.fetch_add(1);
		if (!s_log || count >= 256)
			return;
		std::string target = "<unknown>";
		if (name && nameLength >= static_cast<int>(sizeof(sockaddr_in)) && name->sa_family == AF_INET)
			target = FormatIPv4(*reinterpret_cast<const sockaddr_in*>(name));
		else if (name && nameLength >= static_cast<int>(sizeof(sockaddr_in6)) && name->sa_family == AF_INET6)
			target = FormatIPv6(*reinterpret_cast<const sockaddr_in6*>(name));
		s_log->Write(std::string("connect[") + std::to_string(count + 1) + "] api=" + apiName + " target=" + target);
	}

	void RememberRedirectedSocket(SOCKET socket, const std::string& info)
	{
		Cypress::CFB27::RegisterRedirectedSocketForDiagnostics(static_cast<std::uintptr_t>(socket), info);
	}

	std::string ForgetRedirectedSocket(SOCKET socket)
	{
		std::lock_guard lock(s_redirectedSocketsMutex);
		const auto it = s_redirectedSockets.find(socket);
		if (it == s_redirectedSockets.end())
			return {};
		std::string info = it->second;
		s_redirectedSockets.erase(it);
		return info;
	}

	bool ShouldTraceSocketReceive(SOCKET socket)
	{
		return Cypress::CFB27::IsRedirectedSocketForDiagnostics(static_cast<std::uintptr_t>(socket));
	}

	std::string FormatAddressRva(void* address);

	void TraceSocketReceive(const char* apiName, SOCKET socket, int result, int error, const char* data, std::size_t length)
	{
		if (!s_log || !ShouldTraceSocketReceive(socket))
			return;
		const unsigned int count = s_receiveTraceCount.fetch_add(1);
		if (count >= 96)
			return;
		std::string preview;
		const std::size_t previewLength = std::min<std::size_t>(length, 24);
		constexpr char hex[] = "0123456789ABCDEF";
		preview.reserve(previewLength * 2);
		for (std::size_t index = 0; index < previewLength; ++index)
		{
			const auto value = static_cast<unsigned char>(data[index]);
			preview.push_back(hex[value >> 4]);
			preview.push_back(hex[value & 0x0F]);
		}
		s_log->Write(
			"receive[" + std::to_string(count + 1) + "] api=" + apiName +
			" socket=" + std::to_string(static_cast<unsigned long long>(socket)) +
			" result=" + std::to_string(result) +
			" wsa=" + std::to_string(error) +
			(preview.empty() ? "" : " bytes=" + preview));

		if (Cypress::CFB27::IsTlsServerHelloDone(
			reinterpret_cast<const std::uint8_t*>(data), length))
		{
			void* frames[32] = {};
			const USHORT captured = CaptureStackBackTrace(0, static_cast<DWORD>(std::size(frames)), frames, nullptr);
			std::string stack;
			for (USHORT index = 0; index < captured; ++index)
			{
				if (!stack.empty())
					stack += ",";
				stack += FormatAddressRva(frames[index]);
			}
			s_log->Write("tls-server-hello-done recv-stack=" + stack);
		}
	}

	bool IsConfiguredCandidateEndpoint(const std::uint32_t address, const std::uint16_t port)
	{
		if (!s_enableCandidateEndpointRedirects)
			return false;
		return std::any_of(
			s_candidateEndpoints.begin(),
			s_candidateEndpoints.end(),
			[&](const Cypress::CFB27::CandidateEndpoint& endpoint)
			{
				return endpoint.hostOrderAddress == address && endpoint.port == port;
			});
	}

	bool IsLoopbackRedirectPort(const std::uint16_t port)
	{
		return port == 443 || port == 44325 || port == 42230;
	}

	bool ShouldRedirectIPv4(const sockaddr* name, const int nameLength, std::uint32_t& address, std::uint16_t& port, std::string& reason)
	{
		if (!name || nameLength < static_cast<int>(sizeof(sockaddr_in)) || name->sa_family != AF_INET)
			return false;
		const auto* incoming = reinterpret_cast<const sockaddr_in*>(name);
		address = ntohl(incoming->sin_addr.s_addr);
		port = ntohs(incoming->sin_port);
		if (s_enableRedirectorNetworkRedirect && IsLoopbackRedirectPort(port) && address == INADDR_LOOPBACK)
		{
			reason = "patched-hostname-loopback";
			return true;
		}
		if (s_enableRedirectorNetworkRedirect && port == 443 && Cypress::CFB27::IsKnownRedirectorIPv4(address))
		{
			reason = "known-redirector";
			return true;
		}
		if (IsConfiguredCandidateEndpoint(address, port))
		{
			reason = "candidate-json";
			return true;
		}
		return false;
	}

	bool TryReadIPv4MappedIPv6(
		const sockaddr* name,
		const int nameLength,
		std::uint32_t& address,
		std::uint16_t& port)
	{
		if (!name || nameLength < static_cast<int>(sizeof(sockaddr_in6)) || name->sa_family != AF_INET6)
			return false;
		const auto* incoming = reinterpret_cast<const sockaddr_in6*>(name);
		const auto& bytes = incoming->sin6_addr.u.Byte;
		for (int index = 0; index < 10; ++index)
		{
			if (bytes[index] != 0)
				return false;
		}
		if (bytes[10] != 0xFF || bytes[11] != 0xFF)
			return false;
		address =
			(static_cast<std::uint32_t>(bytes[12]) << 24) |
			(static_cast<std::uint32_t>(bytes[13]) << 16) |
			(static_cast<std::uint32_t>(bytes[14]) << 8) |
			static_cast<std::uint32_t>(bytes[15]);
		port = ntohs(incoming->sin6_port);
		return true;
	}

	bool ShouldRedirectIPv4MappedIPv6(
		const sockaddr* name,
		const int nameLength,
		std::uint32_t& address,
		std::uint16_t& port,
		std::string& reason)
	{
		if (!TryReadIPv4MappedIPv6(name, nameLength, address, port))
			return false;
		if (s_enableRedirectorNetworkRedirect && IsLoopbackRedirectPort(port) && address == INADDR_LOOPBACK)
		{
			reason = "patched-hostname-loopback-ipv6";
			return true;
		}
		if (s_enableRedirectorNetworkRedirect && port == 443 && Cypress::CFB27::IsKnownRedirectorIPv4(address))
		{
			reason = "known-redirector-ipv6";
			return true;
		}
		if (IsConfiguredCandidateEndpoint(address, port))
		{
			reason = "candidate-json-ipv6";
			return true;
		}
		return false;
	}

	sockaddr_in BuildLoopbackRedirect()
	{
		sockaddr_in redirected{};
		redirected.sin_family = AF_INET;
		redirected.sin_addr.s_addr = htonl(INADDR_LOOPBACK);
		redirected.sin_port = htons(s_bridgePort);
		return redirected;
	}

	sockaddr_in6 BuildLoopbackRedirect6()
	{
		sockaddr_in6 redirected{};
		redirected.sin6_family = AF_INET6;
		redirected.sin6_port = htons(s_bridgePort);
		redirected.sin6_addr.u.Byte[15] = 1;
		return redirected;
	}

	void TraceRedirect(
		const char* apiName,
		const sockaddr_in& original,
		const std::string& reason,
		const std::string& target = {})
	{
		const unsigned int count = s_redirectTraceCount.fetch_add(1);
		if (!s_log || count >= 128)
			return;
		s_log->Write(
			std::string("redirect[") + std::to_string(count + 1) + "] api=" + apiName +
			" reason=" + reason +
			" from=" + FormatIPv4(original) +
			" to=" + (target.empty() ? "127.0.0.1:" + std::to_string(s_bridgePort) : target));
	}

	void TraceRedirectResult(const char* apiName, const int result)
	{
		if (!s_log)
			return;
		if (result == SOCKET_ERROR)
			s_log->Write(std::string("redirect result api=") + apiName + " status=SOCKET_ERROR wsa=" + std::to_string(WSAGetLastError()));
		else
			s_log->Write(std::string("redirect result api=") + apiName + " status=ok");
	}

	void TraceWinHttp(const std::string& message)
	{
		const unsigned int count = s_winHttpTraceCount.fetch_add(1);
		if (!s_log || count >= 64)
			return;
		s_log->Write("winhttp[" + std::to_string(count + 1) + "] " + message);
	}

	void AllowLocalWinHttpCertificate(HINTERNET request, const char* source)
	{
		if (!request)
			return;

		DWORD flags =
			SECURITY_FLAG_IGNORE_UNKNOWN_CA |
			SECURITY_FLAG_IGNORE_CERT_CN_INVALID |
			SECURITY_FLAG_IGNORE_CERT_DATE_INVALID |
			SECURITY_FLAG_IGNORE_CERT_WRONG_USAGE;
		if (WinHttpSetOption(request, WINHTTP_OPTION_SECURITY_FLAGS, &flags, sizeof(flags)))
			TraceWinHttp(std::string(source) + " set local cert ignore flags");
		else
			TraceWinHttp(std::string(source) + " failed to set local cert ignore flags error=" + std::to_string(GetLastError()));
	}

	void* ResolveWinHttpProc(const char* name)
	{
		HMODULE module = GetModuleHandleW(L"winhttp.dll");
		if (!module)
			module = LoadLibraryW(L"winhttp.dll");
		return module ? reinterpret_cast<void*>(GetProcAddress(module, name)) : nullptr;
	}

	std::string FormatAddressRva(void* address)
	{
		auto* base = reinterpret_cast<std::uint8_t*>(GetModuleHandleA(nullptr));
		if (!base || !address)
			return "0";
		auto* dos = reinterpret_cast<IMAGE_DOS_HEADER*>(base);
		if (dos->e_magic != IMAGE_DOS_SIGNATURE)
			return "0";
		auto* headers = reinterpret_cast<IMAGE_NT_HEADERS64*>(base + dos->e_lfanew);
		if (headers->Signature != IMAGE_NT_SIGNATURE)
			return "0";
		auto* value = reinterpret_cast<std::uint8_t*>(address);
		if (value < base || value >= base + headers->OptionalHeader.SizeOfImage)
		{
			char buffer[32] = {};
			sprintf_s(buffer, "%p", address);
			return buffer;
		}
		char buffer[32] = {};
		sprintf_s(buffer, "0x%llX", static_cast<unsigned long long>(value - base));
		return buffer;
	}

	void TraceTlsAlert(const char* apiName, const void* buffer, const int length)
	{
		if (!s_log || !buffer || length < 7)
			return;
		const auto* bytes = static_cast<const std::uint8_t*>(buffer);
		if (bytes[0] != 0x15 || bytes[3] != 0x00 || bytes[4] < 0x02)
			return;
		const std::uint8_t level = bytes[5];
		const std::uint8_t description = bytes[6];

		const unsigned int count = s_tlsAlertTraceCount.fetch_add(1);
		if (count >= 32)
			return;

		void* frames[32] = {};
		const USHORT captured = CaptureStackBackTrace(0, static_cast<DWORD>(sizeof(frames) / sizeof(frames[0])), frames, nullptr);
		std::string stack;
		for (USHORT index = 0; index < captured; ++index)
		{
			if (!stack.empty())
				stack += ",";
			stack += FormatAddressRva(frames[index]);
		}
		s_log->Write(
			std::string("tls-alert[") + std::to_string(count + 1) + "] api=" + apiName +
			" level=" + std::to_string(level) +
			" desc=" + std::to_string(description) +
			" stack=" + stack);
	}

	void TraceRedirectedSocketClose(SOCKET socket)
	{
		const std::string info = ForgetRedirectedSocket(socket);
		if (info.empty())
			return;
		const unsigned int count = s_closeTraceCount.fetch_add(1);
		if (!s_log || count >= 64)
			return;

		void* frames[16] = {};
		const USHORT captured = CaptureStackBackTrace(0, static_cast<DWORD>(sizeof(frames) / sizeof(frames[0])), frames, nullptr);
		std::string stack;
		for (USHORT index = 0; index < captured; ++index)
		{
			if (!stack.empty())
				stack += ",";
			stack += FormatAddressRva(frames[index]);
		}
		s_log->Write(
			std::string("close[") + std::to_string(count + 1) + "] socket=" +
			std::to_string(static_cast<unsigned long long>(socket)) +
			" " + info +
			" stack=" + stack);
	}

	std::vector<Cypress::CFB27::CandidateEndpoint> LoadCandidateEndpoints(
		const std::string& path,
		Cypress::CFB27::BridgeLog& log)
	{
		if (path.empty())
			return {};
		std::ifstream input(path, std::ios::binary);
		if (!input)
		{
			log.Write("candidate endpoint file not found: " + path);
			return {};
		}
		const std::string json(
			(std::istreambuf_iterator<char>(input)),
			std::istreambuf_iterator<char>());
		auto endpoints = Cypress::CFB27::ParseCandidateEndpointsJson(json);
		log.Write("loaded candidate endpoints from " + path + " count=" + std::to_string(endpoints.size()));
		return endpoints;
	}

	INT WSAAPI HookGetAddrInfoA(
		PCSTR nodeName,
		PCSTR serviceName,
		const ADDRINFOA* hints,
		PADDRINFOA* result)
	{
		TraceDns(nodeName ? nodeName : "", FormatService(serviceName));
		if (s_enableRedirectorNetworkRedirect && nodeName && Cypress::CFB27::IsBlazeRedirectorHost(nodeName))
		{
			std::call_once(s_dnsLog, []
			{
				if (s_log)
					s_log->Write("redirecting Blaze redirector DNS to " + s_bridgeHost);
			});
			return s_originalGetAddrInfoA(s_bridgeHost.c_str(), serviceName, hints, result);
		}
		return s_originalGetAddrInfoA(nodeName, serviceName, hints, result);
	}

	INT WSAAPI HookGetAddrInfoW(
		PCWSTR nodeName,
		PCWSTR serviceName,
		const ADDRINFOW* hints,
		PADDRINFOW* result)
	{
		const std::string narrowed = Narrow(nodeName);
		TraceDns(narrowed, FormatService(serviceName));
		if (s_enableRedirectorNetworkRedirect && nodeName && Cypress::CFB27::IsBlazeRedirectorHost(narrowed))
		{
			std::call_once(s_dnsLog, []
			{
				if (s_log)
					s_log->Write("redirecting Blaze redirector DNS to " + s_bridgeHost);
			});
			return s_originalGetAddrInfoW(s_bridgeHostWide.c_str(), serviceName, hints, result);
		}
		return s_originalGetAddrInfoW(nodeName, serviceName, hints, result);
	}

	INT WSAAPI HookConnect(SOCKET socket, const sockaddr* name, const int nameLength)
	{
		TraceConnect("connect", name, nameLength);
		std::uint32_t address = 0;
		std::uint16_t port = 0;
		std::string reason;
		if (ShouldRedirectIPv4(name, nameLength, address, port, reason))
		{
			const auto* incoming = reinterpret_cast<const sockaddr_in*>(name);
			sockaddr_in redirected = BuildLoopbackRedirect();
			TraceRedirect("connect", *incoming, reason);
			std::call_once(s_connectLog, []
			{
				if (s_log)
					s_log->Write("redirecting Blaze redirector TCP to 127.0.0.1:" + std::to_string(s_bridgePort));
			});
			const int result = s_originalConnect(socket, reinterpret_cast<const sockaddr*>(&redirected), sizeof(redirected));
			RememberRedirectedSocket(socket, "connect " + reason + " from=" + FormatIPv4(*incoming));
			TraceRedirectResult("connect", result);
			return result;
		}
		if (ShouldRedirectIPv4MappedIPv6(name, nameLength, address, port, reason))
		{
			const sockaddr_in6 redirected = BuildLoopbackRedirect6();
			sockaddr_in original{};
			original.sin_family = AF_INET;
			original.sin_addr.s_addr = htonl(address);
			original.sin_port = htons(port);
			TraceRedirect("connect", original, reason, "[::1]:" + std::to_string(s_bridgePort));
			std::call_once(s_connectLog, []
			{
				if (s_log)
					s_log->Write("redirecting Blaze redirector TCP to loopback:" + std::to_string(s_bridgePort));
			});
			const int result = s_originalConnect(socket, reinterpret_cast<const sockaddr*>(&redirected), sizeof(redirected));
			RememberRedirectedSocket(socket, "connect " + reason + " from=" + FormatIPv4(original));
			TraceRedirectResult("connect-ipv6", result);
			return result;
		}
		return s_originalConnect(socket, name, nameLength);
	}

	INT WSAAPI HookWSAConnect(
		SOCKET socket,
		const sockaddr* name,
		const int nameLength,
		LPWSABUF callerData,
		LPWSABUF calleeData,
		LPQOS socketQos,
		LPQOS groupQos)
	{
		TraceConnect("WSAConnect", name, nameLength);
		std::uint32_t address = 0;
		std::uint16_t port = 0;
		std::string reason;
		if (ShouldRedirectIPv4(name, nameLength, address, port, reason))
		{
			const auto* incoming = reinterpret_cast<const sockaddr_in*>(name);
			sockaddr_in redirected = BuildLoopbackRedirect();
			TraceRedirect("WSAConnect", *incoming, reason);
			std::call_once(s_connectLog, []
			{
				if (s_log)
					s_log->Write("redirecting Blaze redirector TCP to 127.0.0.1:" + std::to_string(s_bridgePort));
			});
			const int result = s_originalWSAConnect(
				socket,
				reinterpret_cast<const sockaddr*>(&redirected),
				sizeof(redirected),
				callerData,
				calleeData,
				socketQos,
				groupQos);
			RememberRedirectedSocket(socket, "WSAConnect " + reason + " from=" + FormatIPv4(*incoming));
			TraceRedirectResult("WSAConnect", result);
			return result;
		}
		if (ShouldRedirectIPv4MappedIPv6(name, nameLength, address, port, reason))
		{
			const sockaddr_in6 redirected = BuildLoopbackRedirect6();
			sockaddr_in original{};
			original.sin_family = AF_INET;
			original.sin_addr.s_addr = htonl(address);
			original.sin_port = htons(port);
			TraceRedirect("WSAConnect", original, reason, "[::1]:" + std::to_string(s_bridgePort));
			std::call_once(s_connectLog, []
			{
				if (s_log)
					s_log->Write("redirecting Blaze redirector TCP to loopback:" + std::to_string(s_bridgePort));
			});
			const int result = s_originalWSAConnect(
				socket,
				reinterpret_cast<const sockaddr*>(&redirected),
				sizeof(redirected),
				callerData,
				calleeData,
				socketQos,
				groupQos);
			RememberRedirectedSocket(socket, "WSAConnect " + reason + " from=" + FormatIPv4(original));
			TraceRedirectResult("WSAConnect-ipv6", result);
			return result;
		}
		return s_originalWSAConnect(socket, name, nameLength, callerData, calleeData, socketQos, groupQos);
	}

	int WSAAPI HookCloseSocket(SOCKET socket)
	{
		TraceRedirectedSocketClose(socket);
		return s_originalCloseSocket(socket);
	}

	int WSAAPI HookSend(SOCKET socket, const char* buffer, const int length, const int flags)
	{
		TraceTlsAlert("send", buffer, length);
		return s_originalSend(socket, buffer, length, flags);
	}

	int WSAAPI HookRecv(SOCKET socket, char* buffer, const int length, const int flags)
	{
		const int result = s_originalRecv(socket, buffer, length, flags);
		const int error = result == SOCKET_ERROR ? WSAGetLastError() : 0;
		TraceSocketReceive("recv", socket, result, error, buffer, result > 0 ? static_cast<std::size_t>(result) : 0);
		if (result == SOCKET_ERROR)
			WSASetLastError(error);
		return result;
	}

	int WSAAPI HookWSARecv(SOCKET socket, LPWSABUF buffers, DWORD bufferCount, LPDWORD bytesReceived, LPDWORD flags, LPWSAOVERLAPPED overlapped, LPWSAOVERLAPPED_COMPLETION_ROUTINE completionRoutine)
	{
		const int result = s_originalWSARecv(socket, buffers, bufferCount, bytesReceived, flags, overlapped, completionRoutine);
		const int error = result == SOCKET_ERROR ? WSAGetLastError() : 0;
		const std::size_t received = result == 0 && bytesReceived ? *bytesReceived : 0;
		const char* data = buffers && bufferCount > 0 && buffers[0].buf ? buffers[0].buf : "";
		TraceSocketReceive("WSARecv", socket, result, error, data, received);
		if (result == SOCKET_ERROR)
			WSASetLastError(error);
		return result;
	}

	int WSAAPI HookWSASend(
		SOCKET socket,
		LPWSABUF buffers,
		const DWORD bufferCount,
		LPDWORD bytesSent,
		const DWORD flags,
		LPWSAOVERLAPPED overlapped,
		LPWSAOVERLAPPED_COMPLETION_ROUTINE completionRoutine)
	{
		if (buffers && bufferCount > 0)
		{
			for (DWORD index = 0; index < bufferCount; ++index)
				TraceTlsAlert("WSASend", buffers[index].buf, static_cast<int>(buffers[index].len));
		}
		return s_originalWSASend(socket, buffers, bufferCount, bytesSent, flags, overlapped, completionRoutine);
	}

	BOOL WINAPI HookWinHttpSendRequest(
		HINTERNET request,
		LPCWSTR headers,
		DWORD headersLength,
		LPVOID optional,
		DWORD optionalLength,
		DWORD totalLength,
		DWORD_PTR context)
	{
		AllowLocalWinHttpCertificate(request, "send-request");
		return s_originalWinHttpSendRequest(
			request,
			headers,
			headersLength,
			optional,
			optionalLength,
			totalLength,
			context);
	}

	BOOL WINAPI HookWinHttpReceiveResponse(HINTERNET request, LPVOID reserved)
	{
		BOOL result = s_originalWinHttpReceiveResponse(request, reserved);
		if (!result && GetLastError() == ERROR_WINHTTP_SECURE_FAILURE)
		{
			TraceWinHttp("receive-response secure failure; retrying with local cert ignore flags");
			AllowLocalWinHttpCertificate(request, "receive-response");
			result = s_originalWinHttpReceiveResponse(request, reserved);
		}
		return result;
	}

	HINTERNET WINAPI HookWinHttpConnect(HINTERNET session, LPCWSTR serverName, INTERNET_PORT serverPort, DWORD reserved)
	{
		TraceWinHttp(
			"connect host=" + (serverName ? Narrow(serverName) : std::string("<null>")) +
			" port=" + std::to_string(serverPort));
		return s_originalWinHttpConnect(session, serverName, serverPort, reserved);
	}

	HINTERNET WINAPI HookWinHttpOpenRequest(
		HINTERNET connect,
		LPCWSTR verb,
		LPCWSTR objectName,
		LPCWSTR version,
		LPCWSTR referrer,
		LPCWSTR* acceptTypes,
		DWORD flags)
	{
		TraceWinHttp(
			"open-request verb=" + (verb ? Narrow(verb) : std::string("<null>")) +
			" object=" + (objectName ? Narrow(objectName) : std::string("<null>")) +
			" flags=0x" + [&]
			{
				char buffer[16] = {};
				sprintf_s(buffer, "%lX", flags);
				return std::string(buffer);
			}());
		HINTERNET request = s_originalWinHttpOpenRequest(connect, verb, objectName, version, referrer, acceptTypes, flags);
		AllowLocalWinHttpCertificate(request, "open-request");
		return request;
	}

	// NOTE (2026-07-06): The game's TLS is DirtySDK ProtoSSL using BearSSL's minimal
	// X.509 engine with a pinned EA "gosca" CA, not Windows schannel. It never calls
	// crypt32/wintrust, so hooks on CertVerifyCertificateChainPolicy /
	// CertGetCertificateChain / WinVerifyTrust did nothing and were removed. Confirmed
	// by the CollegeFB27_Trial memory dump (DirtySDK, ProtoSSL*, gosca, BearSSL strings).
	// The certificate is instead accepted by hooking BearSSL's X.509 end_chain callback
	// (see InstallBearSslCertificateBypass in MemoryDiscovery.cpp), which forces the
	// validation context to BR_ERR_X509_OK. The local Blaze bridge terminates TLS with
	// tools/cypress-servers/internal/cfb27blaze/tls.go.
	// The HookSend/HookWSASend TLS-alert stack tracing above is kept on purpose: it
	// captures the game-side call stack when ProtoSSL emits a bad_certificate alert
	// (desc 46), which locates the BearSSL/ProtoSSL verify path in the real executable.
}

namespace Cypress::CFB27
{
	void RegisterRedirectedSocketForDiagnostics(const std::uintptr_t socket, const std::string& info)
	{
		std::lock_guard lock(s_redirectedSocketsMutex);
		s_redirectedSockets[static_cast<SOCKET>(socket)] = info;
	}

	bool IsRedirectedSocketForDiagnostics(const std::uintptr_t socket)
	{
		std::lock_guard lock(s_redirectedSocketsMutex);
		return s_redirectedSockets.contains(static_cast<SOCKET>(socket));
	}

	void UnregisterRedirectedSocketForDiagnostics(const std::uintptr_t socket)
	{
		std::lock_guard lock(s_redirectedSocketsMutex);
		s_redirectedSockets.erase(static_cast<SOCKET>(socket));
	}

	bool IsTlsServerHelloDone(const std::uint8_t* data, const std::size_t length)
	{
		return data && length == 4 &&
			data[0] == 0x0E && data[1] == 0x00 && data[2] == 0x00 && data[3] == 0x00;
	}

	bool IsBlazeRedirectorHost(const std::string& host)
	{
		std::string lowered = host;
		std::transform(lowered.begin(), lowered.end(), lowered.begin(), [](const unsigned char value)
		{
			return static_cast<char>(std::tolower(value));
		});
		return lowered.ends_with(".blazeredirector.ea.com") ||
			lowered.ends_with(".gosredirector.ea.com");
	}

	bool IsKnownRedirectorIPv4(const std::uint32_t hostOrderAddress)
	{
		constexpr std::array<std::uint32_t, 2> addresses{
			0xA6751733, // 166.117.23.51
			0x6353B822, // 99.83.184.34
		};
		return std::find(addresses.begin(), addresses.end(), hostOrderAddress) != addresses.end();
	}

	std::vector<CandidateEndpoint> ParseCandidateEndpointsJson(const std::string& json)
	{
		std::vector<CandidateEndpoint> endpoints;
		const std::regex endpointRegex(
			R"json("address"\s*:\s*"([0-9]{1,3}(?:\.[0-9]{1,3}){3})"[\s\S]*?"port"\s*:\s*([0-9]+))json",
			std::regex::ECMAScript);

		for (std::sregex_iterator it(json.begin(), json.end(), endpointRegex), end; it != end; ++it)
		{
			const std::string addressText = (*it)[1].str();
			const std::string portText = (*it)[2].str();
			char* parsedEnd = nullptr;
			const unsigned long parsedPort = std::strtoul(portText.c_str(), &parsedEnd, 10);
			if (!parsedEnd || *parsedEnd != '\0' || parsedPort == 0 || parsedPort > 65535)
				continue;

			in_addr address{};
			if (inet_pton(AF_INET, addressText.c_str(), &address) != 1)
				continue;

			CandidateEndpoint endpoint;
			endpoint.hostOrderAddress = ntohl(address.s_addr);
			endpoint.port = static_cast<std::uint16_t>(parsedPort);
			if (std::none_of(
				endpoints.begin(),
				endpoints.end(),
				[&](const CandidateEndpoint& existing)
				{
					return existing.hostOrderAddress == endpoint.hostOrderAddress && existing.port == endpoint.port;
				}))
				endpoints.push_back(endpoint);
		}

		return endpoints;
	}

	bool InstallRedirectorHooks(const BridgeConfig& config, BridgeLog& log)
	{
		s_bridgeHost = config.blazeHost;
		s_bridgeHostWide = Widen(config.blazeHost);
		s_bridgePort = config.blazePort;
		s_log = &log;
		s_enableRedirectorNetworkRedirect = config.enableRedirectorNetworkRedirect;
		s_enableCandidateEndpointRedirects = config.enableCandidateEndpointRedirects;
		s_candidateEndpoints = LoadCandidateEndpoints(config.endpointsFile, log);
		if (!s_enableCandidateEndpointRedirects)
			log.Write("candidate endpoint redirects disabled by configuration");
		if (!s_enableRedirectorNetworkRedirect)
			log.Write("redirector network redirect disabled; observation mode leaves redirector traffic unchanged");

		if (MH_CreateHook(
			reinterpret_cast<LPVOID>(&getaddrinfo),
			reinterpret_cast<LPVOID>(&HookGetAddrInfoA),
			reinterpret_cast<LPVOID*>(&s_originalGetAddrInfoA)) != MH_OK)
			return false;
		if (MH_CreateHook(
			reinterpret_cast<LPVOID>(&GetAddrInfoW),
			reinterpret_cast<LPVOID>(&HookGetAddrInfoW),
			reinterpret_cast<LPVOID*>(&s_originalGetAddrInfoW)) != MH_OK)
			return false;
		if (MH_CreateHook(
			reinterpret_cast<LPVOID>(&connect),
			reinterpret_cast<LPVOID>(&HookConnect),
			reinterpret_cast<LPVOID*>(&s_originalConnect)) != MH_OK)
			return false;
		if (MH_CreateHook(
			reinterpret_cast<LPVOID>(&closesocket),
			reinterpret_cast<LPVOID>(&HookCloseSocket),
			reinterpret_cast<LPVOID*>(&s_originalCloseSocket)) != MH_OK)
			return false;
		if (MH_CreateHook(
			reinterpret_cast<LPVOID>(&WSAConnect),
			reinterpret_cast<LPVOID>(&HookWSAConnect),
			reinterpret_cast<LPVOID*>(&s_originalWSAConnect)) != MH_OK)
			return false;
		if (MH_CreateHook(
			reinterpret_cast<LPVOID>(&send),
			reinterpret_cast<LPVOID>(&HookSend),
			reinterpret_cast<LPVOID*>(&s_originalSend)) != MH_OK)
			return false;
		if (MH_CreateHook(
			reinterpret_cast<LPVOID>(&recv),
			reinterpret_cast<LPVOID>(&HookRecv),
			reinterpret_cast<LPVOID*>(&s_originalRecv)) != MH_OK)
			return false;
		if (MH_CreateHook(
			reinterpret_cast<LPVOID>(&WSASend),
			reinterpret_cast<LPVOID>(&HookWSASend),
			reinterpret_cast<LPVOID*>(&s_originalWSASend)) != MH_OK)
			return false;
		if (MH_CreateHook(
			reinterpret_cast<LPVOID>(&WSARecv),
			reinterpret_cast<LPVOID>(&HookWSARecv),
			reinterpret_cast<LPVOID*>(&s_originalWSARecv)) != MH_OK)
			return false;
		void* winHttpConnect = ResolveWinHttpProc("WinHttpConnect");
		void* winHttpOpenRequest = ResolveWinHttpProc("WinHttpOpenRequest");
		void* winHttpSendRequest = ResolveWinHttpProc("WinHttpSendRequest");
		void* winHttpReceiveResponse = ResolveWinHttpProc("WinHttpReceiveResponse");
		if (!winHttpConnect || !winHttpOpenRequest || !winHttpSendRequest || !winHttpReceiveResponse)
		{
			log.Write("WinHTTP hook resolve failed");
			return false;
		}
		if (MH_CreateHook(
			winHttpConnect,
			reinterpret_cast<LPVOID>(&HookWinHttpConnect),
			reinterpret_cast<LPVOID*>(&s_originalWinHttpConnect)) != MH_OK)
			return false;
		if (MH_CreateHook(
			winHttpOpenRequest,
			reinterpret_cast<LPVOID>(&HookWinHttpOpenRequest),
			reinterpret_cast<LPVOID*>(&s_originalWinHttpOpenRequest)) != MH_OK)
			return false;
		if (MH_CreateHook(
			winHttpSendRequest,
			reinterpret_cast<LPVOID>(&HookWinHttpSendRequest),
			reinterpret_cast<LPVOID*>(&s_originalWinHttpSendRequest)) != MH_OK)
			return false;
		if (MH_CreateHook(
			winHttpReceiveResponse,
			reinterpret_cast<LPVOID>(&HookWinHttpReceiveResponse),
			reinterpret_cast<LPVOID*>(&s_originalWinHttpReceiveResponse)) != MH_OK)
			return false;

		if (MH_EnableHook(reinterpret_cast<LPVOID>(&send)) != MH_OK)
			return false;
		if (MH_EnableHook(reinterpret_cast<LPVOID>(&recv)) != MH_OK)
			return false;
		if (MH_EnableHook(reinterpret_cast<LPVOID>(&WSASend)) != MH_OK)
			return false;
		if (MH_EnableHook(reinterpret_cast<LPVOID>(&WSARecv)) != MH_OK)
			return false;
		if (MH_EnableHook(reinterpret_cast<LPVOID>(&closesocket)) != MH_OK)
			return false;
		if (MH_EnableHook(reinterpret_cast<LPVOID>(&connect)) != MH_OK)
			return false;
		if (MH_EnableHook(reinterpret_cast<LPVOID>(&WSAConnect)) != MH_OK)
			return false;
		if (MH_EnableHook(winHttpConnect) != MH_OK)
			return false;
		if (MH_EnableHook(winHttpOpenRequest) != MH_OK)
			return false;
		if (MH_EnableHook(winHttpSendRequest) != MH_OK)
			return false;
		if (MH_EnableHook(winHttpReceiveResponse) != MH_OK)
			return false;
		if (MH_EnableHook(reinterpret_cast<LPVOID>(&getaddrinfo)) != MH_OK)
			return false;
		if (MH_EnableHook(reinterpret_cast<LPVOID>(&GetAddrInfoW)) != MH_OK)
			return false;

		log.Write("installed redirector DNS/TCP hooks, redirected-socket receive tracing, WinHTTP local-cert allowance, and ProtoSSL TLS-alert stack tracing");
		return true;
	}
}
