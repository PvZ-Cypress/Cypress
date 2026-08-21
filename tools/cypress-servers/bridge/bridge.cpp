#define WIN32_LEAN_AND_MEAN
#define NOMINMAX
#define DIRECTINPUT_VERSION 0x0800

#include <winsock2.h>
#include <ws2tcpip.h>
#include <mswsock.h>
#include <windows.h>
#include <dinput.h>
#include <psapi.h>
#include <shlobj.h>
#include <winhttp.h>

#include "runtime_scan.h"

#include <algorithm>
#include <array>
#include <cstdarg>
#include <ctime>
#include <cstring>
#include <cstdio>
#include <iterator>
#include <string>
#include <mutex>
#include <set>
#include <vector>

namespace {

constexpr wchar_t kBridgeVersion[] = L"2026.08.04.2";
constexpr unsigned short kLocalBlazePort = 27920;
constexpr uintptr_t kGameConnectSlotRva = 0x0A030498;
constexpr uintptr_t kGameWSAIoctlSlotRva = 0x0A030540;
constexpr uintptr_t kGameConnectIatRva = 0x10738D30;
constexpr uintptr_t kGameWSAIoctlIatRva = 0x10738DD8;

SRWLOCK gLogLock = SRWLOCK_INIT;
INIT_ONCE gProxyInit = INIT_ONCE_STATIC_INIT;
HMODULE gSystemDinput8 = nullptr;

using DirectInput8CreateFn = HRESULT(WINAPI*)(HINSTANCE, DWORD, REFIID, LPVOID*, LPUNKNOWN);
using DllCanUnloadNowFn = HRESULT(WINAPI*)();
using DllGetClassObjectFn = HRESULT(WINAPI*)(REFCLSID, REFIID, LPVOID*);
using DllRegisterServerFn = HRESULT(WINAPI*)();
using DllUnregisterServerFn = HRESULT(WINAPI*)();
using GetdfDIJoystickFn = const DIDATAFORMAT*(WINAPI*)();

DirectInput8CreateFn gDirectInput8Create = nullptr;
DllCanUnloadNowFn gDllCanUnloadNow = nullptr;
DllGetClassObjectFn gDllGetClassObject = nullptr;
DllRegisterServerFn gDllRegisterServer = nullptr;
DllUnregisterServerFn gDllUnregisterServer = nullptr;
GetdfDIJoystickFn gGetdfDIJoystick = nullptr;

using ConnectFn = int(WSAAPI*)(SOCKET, const sockaddr*, int);
using WSAConnectFn = int(WSAAPI*)(
    SOCKET, const sockaddr*, int, LPWSABUF, LPWSABUF, LPQOS, LPQOS);
using WSAIoctlFn = int(WSAAPI*)(
    SOCKET, DWORD, LPVOID, DWORD, LPVOID, DWORD, LPDWORD,
    LPWSAOVERLAPPED, LPWSAOVERLAPPED_COMPLETION_ROUTINE);
using WinHttpConnectFn = HINTERNET(WINAPI*)(HINTERNET, LPCWSTR, INTERNET_PORT, DWORD);
using WinHttpOpenRequestFn = HINTERNET(WINAPI*)(
    HINTERNET, LPCWSTR, LPCWSTR, LPCWSTR, LPCWSTR, LPCWSTR const*, DWORD);
using WinHttpSetOptionFn = BOOL(WINAPI*)(HINTERNET, DWORD, LPVOID, DWORD);

ConnectFn gRealConnect = nullptr;
WSAConnectFn gRealWSAConnect = nullptr;
WSAIoctlFn gRealWSAIoctl = nullptr;
LPFN_CONNECTEX gRealConnectEx = nullptr;
WinHttpConnectFn gRealWinHttpConnect = nullptr;
WinHttpOpenRequestFn gRealWinHttpOpenRequest = nullptr;
WinHttpSetOptionFn gRealWinHttpSetOption = nullptr;
volatile LONG gRedirectCount = 0;

std::wstring ReadEnvironment(const wchar_t* name) {
    const DWORD required = GetEnvironmentVariableW(name, nullptr, 0);
    if (required == 0) {
        return {};
    }
    std::wstring value(required, L'\0');
    const DWORD written = GetEnvironmentVariableW(name, value.data(), required);
    if (written == 0 || written >= required) {
        return {};
    }
    value.resize(written);
    return value;
}

std::wstring DefaultPrivateRoot() {
    std::wstring appData = ReadEnvironment(L"APPDATA");
    if (appData.empty()) {
        wchar_t buffer[MAX_PATH] = {};
        if (FAILED(SHGetFolderPathW(nullptr, CSIDL_APPDATA, nullptr, SHGFP_TYPE_CURRENT, buffer))) {
            return L".";
        }
        appData = buffer;
    }
    return appData + L"\\Cypress\\CFB27\\Private";
}

std::uint64_t ReadPrivateLaunchExpiry(const std::wstring& path) {
    FILE* file = nullptr;
    if (_wfopen_s(&file, path.c_str(), L"rt") != 0 || file == nullptr) {
        return 0;
    }
    wchar_t line[4096] = {};
    std::uint64_t expiry = 0;
    constexpr wchar_t prefix[] = L"privateLaunchExpiresUtc=";
    while (fgetws(line, static_cast<int>(std::size(line)), file)) {
        const std::wstring value(line);
        if (value.rfind(prefix, 0) == 0) {
            wchar_t* end = nullptr;
            expiry = _wcstoui64(value.c_str() + std::size(prefix) - 1, &end, 10);
            if (end == value.c_str() + std::size(prefix) - 1) {
                expiry = 0;
            }
            break;
        }
    }
    fclose(file);
    return expiry;
}

bool CaptureMarkerPresent() {
    const std::wstring marker = DefaultPrivateRoot() + L"\\capture-mode";
    return GetFileAttributesW(marker.c_str()) != INVALID_FILE_ATTRIBUTES;
}

bool ComputePrivateOnlineDynastyEnabled() {
    const std::wstring value = ReadEnvironment(L"CYPRESS_CFB27_PRIVATE_ONLINE_DYNASTY");
    if (value == L"1" || _wcsicmp(value.c_str(), L"true") == 0) {
        return true;
    }
    // A capture is taken by injecting into a game that was launched normally, so
    // no Cypress environment or launch lease exists. The marker is the operator's
    // explicit request, and it arms the hooks the capture depends on.
    if (CaptureMarkerPresent()) {
        return true;
    }
    const std::uint64_t expiry = ReadPrivateLaunchExpiry(DefaultPrivateRoot() + L"\\cfb27-bridge.ini");
    const auto now = static_cast<std::uint64_t>(std::time(nullptr));
    return Cypress::CFB27::RuntimeScan::PrivateLaunchLeaseIsActive(now, expiry);
}

// The lease authorises THIS LAUNCH, so it is evaluated exactly once and latched.
//
// It used to be re-read on every call. A lease that expired while the game was
// running then disarmed the bridge underneath a live session: redirection
// stopped, the game's next connection went nowhere, and it reported "You have
// lost your connection to the EA Servers". Whether a running game stays
// connected must not depend on wall-clock time.
bool PrivateOnlineDynastyEnabled() {
    static const bool enabled = ComputePrivateOnlineDynastyEnabled();
    return enabled;
}

std::wstring ReadRunDirectoryFromConfig(const std::wstring& path) {
    FILE* file = nullptr;
    if (_wfopen_s(&file, path.c_str(), L"rt") != 0 || file == nullptr) {
        return {};
    }
    wchar_t line[4096] = {};
    std::wstring result;
    while (fgetws(line, static_cast<int>(std::size(line)), file)) {
        std::wstring value(line);
        while (!value.empty() && (value.back() == L'\r' || value.back() == L'\n')) {
            value.pop_back();
        }
        constexpr wchar_t prefix[] = L"runDirectory=";
        if (value.rfind(prefix, 0) == 0) {
            result = value.substr(std::size(prefix) - 1);
            break;
        }
    }
    fclose(file);
    return result;
}

// Generic "key=value" lookup in the bridge ini, same shape as the run-directory
// reader above.
std::wstring ReadConfigValue(const std::wstring& path, const std::wstring& key) {
    FILE* file = nullptr;
    if (_wfopen_s(&file, path.c_str(), L"rt") != 0 || file == nullptr) {
        return {};
    }
    wchar_t line[4096] = {};
    std::wstring result;
    const std::wstring prefix = key + L"=";
    while (fgetws(line, static_cast<int>(std::size(line)), file)) {
        std::wstring value(line);
        while (!value.empty() && (value.back() == L'\r' || value.back() == L'\n')) {
            value.pop_back();
        }
        if (value.rfind(prefix, 0) == 0) {
            result = value.substr(prefix.size());
            break;
        }
    }
    fclose(file);
    return result;
}

std::wstring BridgeConfigPath() {
    std::wstring config = ReadEnvironment(L"CYPRESS_CFB27_BRIDGE_CONFIG");
    if (config.empty()) {
        config = DefaultPrivateRoot() + L"\\cfb27-bridge.ini";
    }
    return config;
}

/*
 * Where the game's traffic is sent. The host runs the server on the same machine
 * and uses loopback; a second player must instead be pointed at the host, so the
 * target is configurable via blazeHost in the bridge ini (or
 * CYPRESS_CFB27_BLAZE_HOST). Without this a joining client would redirect to
 * 127.0.0.1 on its OWN machine, where no server is listening, and could never
 * reach the host.
 *
 * Returned in host byte order; callers apply htonl.
 */
std::uint32_t RedirectTargetAddress() {
    static const std::uint32_t address = [] () -> std::uint32_t {
        std::wstring host = ReadEnvironment(L"CYPRESS_CFB27_BLAZE_HOST");
        if (host.empty()) {
            host = ReadConfigValue(BridgeConfigPath(), L"blazeHost");
        }
        if (host.empty()) {
            return INADDR_LOOPBACK;
        }
        // Parse dotted IPv4 without pulling in a resolver: the host is always
        // configured as a literal LAN/VPN address.
        unsigned int octets[4] = {};
        if (swscanf_s(host.c_str(), L"%u.%u.%u.%u", &octets[0], &octets[1], &octets[2], &octets[3]) != 4) {
            return INADDR_LOOPBACK;
        }
        for (const unsigned int octet : octets) {
            if (octet > 255) {
                return INADDR_LOOPBACK;
            }
        }
        return (octets[0] << 24) | (octets[1] << 16) | (octets[2] << 8) | octets[3];
    }();
    return address;
}

// Dotted text form of RedirectTargetAddress, for the WinHTTP hook (which takes a
// host string, not a sockaddr) and for logging. Logging the real target matters:
// the redirect line used to print "127.0.0.1" literally regardless of where
// traffic actually went, which made a misconfigured client indistinguishable
// from a correct one.
const wchar_t* RedirectTargetHostText() {
    static const std::wstring text = [] () -> std::wstring {
        const std::uint32_t address = RedirectTargetAddress();
        wchar_t buffer[64] = {};
        _snwprintf_s(
            buffer, std::size(buffer), _TRUNCATE, L"%u.%u.%u.%u",
            (address >> 24) & 0xff, (address >> 16) & 0xff,
            (address >> 8) & 0xff, address & 0xff);
        return buffer;
    }();
    return text.c_str();
}

std::wstring LogPath() {
    std::wstring runDirectory = ReadEnvironment(L"CYPRESS_CFB27_RUN_DIR");
    if (runDirectory.empty()) {
        std::wstring config = ReadEnvironment(L"CYPRESS_CFB27_BRIDGE_CONFIG");
        if (config.empty()) {
            config = DefaultPrivateRoot() + L"\\cfb27-bridge.ini";
        }
        runDirectory = ReadRunDirectoryFromConfig(config);
    }
    if (runDirectory.empty()) {
        runDirectory = DefaultPrivateRoot();
    }
    SHCreateDirectoryExW(nullptr, runDirectory.c_str(), nullptr);
    return runDirectory + L"\\cfb27-bridge.log";
}

void Log(const wchar_t* format, ...) {
    wchar_t message[2048] = {};
    va_list args;
    va_start(args, format);
    _vsnwprintf_s(message, std::size(message), _TRUNCATE, format, args);
    va_end(args);

    SYSTEMTIME now = {};
    GetLocalTime(&now);
    wchar_t line[2300] = {};
    _snwprintf_s(
        line,
        std::size(line),
        _TRUNCATE,
        L"%04u-%02u-%02u %02u:%02u:%02u.%03u [pid=%lu] %s\r\n",
        now.wYear,
        now.wMonth,
        now.wDay,
        now.wHour,
        now.wMinute,
        now.wSecond,
        now.wMilliseconds,
        GetCurrentProcessId(),
        message);

    const int bytesRequired = WideCharToMultiByte(CP_UTF8, 0, line, -1, nullptr, 0, nullptr, nullptr);
    if (bytesRequired <= 1) {
        return;
    }
    std::string bytes(static_cast<size_t>(bytesRequired), '\0');
    WideCharToMultiByte(CP_UTF8, 0, line, -1, bytes.data(), bytesRequired, nullptr, nullptr);
    bytes.resize(static_cast<size_t>(bytesRequired - 1));

    AcquireSRWLockExclusive(&gLogLock);
    const std::wstring path = LogPath();
    HANDLE file = CreateFileW(
        path.c_str(), FILE_APPEND_DATA, FILE_SHARE_READ | FILE_SHARE_WRITE, nullptr, OPEN_ALWAYS,
        FILE_ATTRIBUTE_NORMAL, nullptr);
    if (file != INVALID_HANDLE_VALUE) {
        DWORD written = 0;
        WriteFile(file, bytes.data(), static_cast<DWORD>(bytes.size()), &written, nullptr);
        CloseHandle(file);
    }
    ReleaseSRWLockExclusive(&gLogLock);
}

BOOL CALLBACK InitializeProxy(PINIT_ONCE, PVOID, PVOID*) {
    wchar_t systemDirectory[MAX_PATH] = {};
    const UINT length = GetSystemDirectoryW(systemDirectory, static_cast<UINT>(std::size(systemDirectory)));
    if (length == 0 || length >= std::size(systemDirectory) - 14) {
        return TRUE;
    }
    std::wstring path(systemDirectory, length);
    path += L"\\dinput8.dll";
    gSystemDinput8 = LoadLibraryW(path.c_str());
    if (!gSystemDinput8) {
        return TRUE;
    }
    gDirectInput8Create = reinterpret_cast<DirectInput8CreateFn>(GetProcAddress(gSystemDinput8, "DirectInput8Create"));
    gDllCanUnloadNow = reinterpret_cast<DllCanUnloadNowFn>(GetProcAddress(gSystemDinput8, "DllCanUnloadNow"));
    gDllGetClassObject = reinterpret_cast<DllGetClassObjectFn>(GetProcAddress(gSystemDinput8, "DllGetClassObject"));
    gDllRegisterServer = reinterpret_cast<DllRegisterServerFn>(GetProcAddress(gSystemDinput8, "DllRegisterServer"));
    gDllUnregisterServer = reinterpret_cast<DllUnregisterServerFn>(GetProcAddress(gSystemDinput8, "DllUnregisterServer"));
    gGetdfDIJoystick = reinterpret_cast<GetdfDIJoystickFn>(GetProcAddress(gSystemDinput8, "GetdfDIJoystick"));
    return TRUE;
}

void EnsureProxy() {
    InitOnceExecuteOnce(&gProxyInit, InitializeProxy, nullptr, nullptr);
}

bool ReadableMemory(const void* address, size_t size) {
    MEMORY_BASIC_INFORMATION info = {};
    if (VirtualQuery(address, &info, sizeof(info)) != sizeof(info)) {
        return false;
    }
    const uintptr_t start = reinterpret_cast<uintptr_t>(address);
    const uintptr_t end = start + size;
    const uintptr_t regionEnd = reinterpret_cast<uintptr_t>(info.BaseAddress) + info.RegionSize;
    return info.State == MEM_COMMIT && !(info.Protect & (PAGE_GUARD | PAGE_NOACCESS)) && end <= regionEnd;
}

bool BytesEqual(const unsigned char* address, const unsigned char* expected, size_t size) {
    if (!ReadableMemory(address, size)) {
        return false;
    }
    __try {
        return memcmp(address, expected, size) == 0;
    } __except (EXCEPTION_EXECUTE_HANDLER) {
        return false;
    }
}

bool IsLoopback(const sockaddr* address, int addressLength) {
    if (!address) {
        return false;
    }
    if (address->sa_family == AF_INET && addressLength >= static_cast<int>(sizeof(sockaddr_in))) {
        const auto* ipv4 = reinterpret_cast<const sockaddr_in*>(address);
        return (ntohl(ipv4->sin_addr.s_addr) & 0xFF000000UL) == 0x7F000000UL;
    }
    if (address->sa_family == AF_INET6 && addressLength >= static_cast<int>(sizeof(sockaddr_in6))) {
        const auto* ipv6 = reinterpret_cast<const sockaddr_in6*>(address);
        if (IN6_IS_ADDR_LOOPBACK(&ipv6->sin6_addr)) {
            return true;
        }
        if (IN6_IS_ADDR_V4MAPPED(&ipv6->sin6_addr)) {
            const unsigned char* bytes = ipv6->sin6_addr.u.Byte;
            return bytes[12] == 127;
        }
    }
    return false;
}

/*
 * Capture mode exists to obtain wire shapes we do not have: several Dynasty
 * commands have never appeared in a capture, so they cannot be implemented from
 * the fixtures we hold. It sends the game to a local capture proxy that forwards
 * upstream using the SNI the client already provides.
 *
 * The redirect port is offset rather than preserved so the capture proxy never
 * contends with the private Blaze server, which listens on 27920 — the same port
 * EA's Blaze servers use. Keeping them apart means a capture can be taken
 * through the normal launch path without first tearing the private stack down.
 *
 * This deliberately relaxes the loopback-only guarantee: in capture mode traffic
 * does reach EA, by way of the proxy. It is off unless explicitly enabled, and
 * the private-server path is unaffected.
 */
constexpr unsigned short kCapturePortOffset = 10000;

unsigned short CapturePortFor(const unsigned short originalPort) {
    const unsigned int shifted = static_cast<unsigned int>(originalPort) + kCapturePortOffset;
    return shifted > 65535 ? static_cast<unsigned short>(shifted - 65536)
                           : static_cast<unsigned short>(shifted);
}
bool CaptureModeEnabled() {
    static const bool enabled = [] {
        const std::wstring value = ReadEnvironment(L"CYPRESS_CFB27_CAPTURE_MODE");
        if (value == L"1" || _wcsicmp(value.c_str(), L"true") == 0) {
            return true;
        }
        // A marker file rather than an environment variable, because the game is
        // launched normally and this library is injected into it afterwards.
        return CaptureMarkerPresent();
    }();
    return enabled;
}

/*
 * IPv4 addresses of the pass-through hosts, resolved once.
 *
 * Exempting authentication by hostname in the WinHTTP hook is not enough: the
 * socket underneath still reaches connect(), where every non-loopback address is
 * redirected. Without this the login traffic would be captured anyway and the
 * hostname exemption would appear to do nothing.
 */
// Addresses the game itself resolved for a pass-through host. Resolving those
// names here instead does not work: EA fronts them with a CDN that answers with
// different addresses per lookup, so the set we resolved never contained the
// address the game actually dialled. Recording the game's own DNS answers is
// exact by construction.
std::mutex gPassThroughMutex;
std::set<std::uint32_t> gPassThroughAddresses;

void RememberPassThroughAddresses(const ADDRINFOA* info) {
    std::lock_guard<std::mutex> guard(gPassThroughMutex);
    for (const ADDRINFOA* it = info; it != nullptr; it = it->ai_next) {
        if (it->ai_family == AF_INET && it->ai_addr) {
            const auto* v4 = reinterpret_cast<const sockaddr_in*>(it->ai_addr);
            gPassThroughAddresses.insert(ntohl(v4->sin_addr.s_addr));
        }
    }
}

void RememberPassThroughAddressesW(const ADDRINFOW* info) {
    std::lock_guard<std::mutex> guard(gPassThroughMutex);
    for (const ADDRINFOW* it = info; it != nullptr; it = it->ai_next) {
        if (it->ai_family == AF_INET && it->ai_addr) {
            const auto* v4 = reinterpret_cast<const sockaddr_in*>(it->ai_addr);
            gPassThroughAddresses.insert(ntohl(v4->sin_addr.s_addr));
        }
    }
}

bool IsPassThroughAddress(std::uint32_t hostOrderAddress) {
    std::lock_guard<std::mutex> guard(gPassThroughMutex);
    return gPassThroughAddresses.count(hostOrderAddress) != 0;
}

bool RedirectAddress(
    const sockaddr* original,
    int originalLength,
    sockaddr_storage& redirected,
    int& redirectedLength) {
    if (!original || IsLoopback(original, originalLength)) {
        return false;
    }
    if (original->sa_family == AF_INET && originalLength >= static_cast<int>(sizeof(sockaddr_in))) {
        const auto* source = reinterpret_cast<const sockaddr_in*>(original);
        if (IsPassThroughAddress(ntohl(source->sin_addr.s_addr))) {
            return false;
        }
    }
    if (original->sa_family == AF_INET6 && originalLength >= static_cast<int>(sizeof(sockaddr_in6))) {
        const auto* source = reinterpret_cast<const sockaddr_in6*>(original);
        const auto& bytes = source->sin6_addr.u.Byte;
        const bool mapped = IN6_IS_ADDR_V4MAPPED(&source->sin6_addr);
        if (mapped) {
            const std::uint32_t v4 =
                (static_cast<std::uint32_t>(bytes[12]) << 24) |
                (static_cast<std::uint32_t>(bytes[13]) << 16) |
                (static_cast<std::uint32_t>(bytes[14]) << 8) |
                static_cast<std::uint32_t>(bytes[15]);
            if (IsPassThroughAddress(v4)) {
                return false;
            }
        }
    }
    const bool capture = CaptureModeEnabled();
    memset(&redirected, 0, sizeof(redirected));
    if (original->sa_family == AF_INET && originalLength >= static_cast<int>(sizeof(sockaddr_in))) {
        const auto* source = reinterpret_cast<const sockaddr_in*>(original);
        auto* target = reinterpret_cast<sockaddr_in*>(&redirected);
        target->sin_family = AF_INET;
        target->sin_port = capture ? htons(CapturePortFor(ntohs(source->sin_port)))
                                   : htons(kLocalBlazePort);
        target->sin_addr.s_addr = htonl(RedirectTargetAddress());
        redirectedLength = sizeof(*target);
        return true;
    }
    if (original->sa_family == AF_INET6 && originalLength >= static_cast<int>(sizeof(sockaddr_in6))) {
        const auto* source = reinterpret_cast<const sockaddr_in6*>(original);
        auto* target = reinterpret_cast<sockaddr_in6*>(&redirected);
        target->sin6_family = AF_INET6;
        target->sin6_port = capture ? htons(CapturePortFor(ntohs(source->sin6_port)))
                                    : htons(kLocalBlazePort);
        const std::uint32_t host = RedirectTargetAddress();
        if (host == INADDR_LOOPBACK) {
            target->sin6_addr = in6addr_loopback;
        } else {
            // Remote host: send IPv6 connections to the IPv4-mapped form
            // (::ffff:a.b.c.d) rather than this machine's loopback, which would
            // silently point a joining client back at itself.
            std::memset(&target->sin6_addr, 0, sizeof(target->sin6_addr));
            auto* bytes = reinterpret_cast<std::uint8_t*>(&target->sin6_addr);
            bytes[10] = 0xff;
            bytes[11] = 0xff;
            bytes[12] = static_cast<std::uint8_t>((host >> 24) & 0xff);
            bytes[13] = static_cast<std::uint8_t>((host >> 16) & 0xff);
            bytes[14] = static_cast<std::uint8_t>((host >> 8) & 0xff);
            bytes[15] = static_cast<std::uint8_t>(host & 0xff);
        }
        redirectedLength = sizeof(*target);
        return true;
    }
    return false;
}

std::wstring EndpointText(const sockaddr* address, int addressLength) {
    if (!address) {
        return L"<null>";
    }
    wchar_t host[INET6_ADDRSTRLEN] = {};
    unsigned short port = 0;
    if (address->sa_family == AF_INET && addressLength >= static_cast<int>(sizeof(sockaddr_in))) {
        const auto* ipv4 = reinterpret_cast<const sockaddr_in*>(address);
        InetNtopW(AF_INET, const_cast<IN_ADDR*>(&ipv4->sin_addr), host, static_cast<DWORD>(std::size(host)));
        port = ntohs(ipv4->sin_port);
    } else if (address->sa_family == AF_INET6 && addressLength >= static_cast<int>(sizeof(sockaddr_in6))) {
        const auto* ipv6 = reinterpret_cast<const sockaddr_in6*>(address);
        InetNtopW(AF_INET6, const_cast<IN6_ADDR*>(&ipv6->sin6_addr), host, static_cast<DWORD>(std::size(host)));
        port = ntohs(ipv6->sin6_port);
    } else {
        return L"<non-IP socket>";
    }
    wchar_t result[128] = {};
    if (address->sa_family == AF_INET6) {
        _snwprintf_s(result, std::size(result), _TRUNCATE, L"[%s]:%u", host, port);
    } else {
        _snwprintf_s(result, std::size(result), _TRUNCATE, L"%s:%u", host, port);
    }
    return result;
}

void LogRedirect(const wchar_t* api, const sockaddr* original, int originalLength) {
    const LONG number = InterlockedIncrement(&gRedirectCount);
    const std::wstring endpoint = EndpointText(original, originalLength);
    Log(
        L"redirect #%ld via %s: %s -> %s:%u",
        number, api, endpoint.c_str(), RedirectTargetHostText(), kLocalBlazePort);
}

int WSAAPI HookConnect(SOCKET socket, const sockaddr* name, int nameLength) {
    sockaddr_storage redirected = {};
    int redirectedLength = 0;
    if (RedirectAddress(name, nameLength, redirected, redirectedLength)) {
        LogRedirect(L"connect", name, nameLength);
        return gRealConnect(socket, reinterpret_cast<const sockaddr*>(&redirected), redirectedLength);
    }
    return gRealConnect(socket, name, nameLength);
}

int WSAAPI HookWSAConnect(
    SOCKET socket,
    const sockaddr* name,
    int nameLength,
    LPWSABUF callerData,
    LPWSABUF calleeData,
    LPQOS socketQos,
    LPQOS groupQos) {
    sockaddr_storage redirected = {};
    int redirectedLength = 0;
    if (RedirectAddress(name, nameLength, redirected, redirectedLength)) {
        LogRedirect(L"WSAConnect", name, nameLength);
        return gRealWSAConnect(
            socket, reinterpret_cast<const sockaddr*>(&redirected), redirectedLength,
            callerData, calleeData, socketQos, groupQos);
    }
    return gRealWSAConnect(socket, name, nameLength, callerData, calleeData, socketQos, groupQos);
}

BOOL PASCAL HookConnectEx(
    SOCKET socket,
    const sockaddr* name,
    int nameLength,
    PVOID sendBuffer,
    DWORD sendLength,
    LPDWORD bytesSent,
    LPOVERLAPPED overlapped) {
    if (!gRealConnectEx) {
        WSASetLastError(WSAEOPNOTSUPP);
        return FALSE;
    }
    sockaddr_storage redirected = {};
    int redirectedLength = 0;
    if (RedirectAddress(name, nameLength, redirected, redirectedLength)) {
        LogRedirect(L"ConnectEx", name, nameLength);
        return gRealConnectEx(
            socket, reinterpret_cast<const sockaddr*>(&redirected), redirectedLength,
            sendBuffer, sendLength, bytesSent, overlapped);
    }
    return gRealConnectEx(socket, name, nameLength, sendBuffer, sendLength, bytesSent, overlapped);
}

int WSAAPI HookWSAIoctl(
    SOCKET socket,
    DWORD controlCode,
    LPVOID input,
    DWORD inputSize,
    LPVOID output,
    DWORD outputSize,
    LPDWORD bytesReturned,
    LPWSAOVERLAPPED overlapped,
    LPWSAOVERLAPPED_COMPLETION_ROUTINE completionRoutine) {
    const int result = gRealWSAIoctl(
        socket, controlCode, input, inputSize, output, outputSize, bytesReturned,
        overlapped, completionRoutine);
    if (result == 0 && controlCode == SIO_GET_EXTENSION_FUNCTION_POINTER &&
        input && inputSize >= sizeof(GUID) && output && outputSize >= sizeof(LPFN_CONNECTEX) &&
        IsEqualGUID(*reinterpret_cast<const GUID*>(input), WSAID_CONNECTEX)) {
        auto* function = reinterpret_cast<LPFN_CONNECTEX*>(output);
        if (*function && *function != HookConnectEx) {
            InterlockedExchangePointer(
                reinterpret_cast<PVOID volatile*>(&gRealConnectEx), reinterpret_cast<PVOID>(*function));
            *function = HookConnectEx;
            Log(L"intercepted a ConnectEx extension-function request");
        }
    }
    return result;
}

bool IsLoopbackHost(const wchar_t* host) {
    return host && (
        _wcsicmp(host, L"localhost") == 0 ||
        _wcsicmp(host, L"127.0.0.1") == 0 ||
        _wcsicmp(host, L"::1") == 0 ||
        _wcsicmp(host, L"[::1]") == 0);
}

/*
 * Hosts that must keep talking to real EA rather than to the private server.
 *
 * EA account sign-in is the one thing the private server cannot stand in for:
 * it is an OAuth exchange against EA's identity service, and we have no capture
 * of it to replay. Redirecting it meant the game received a stub response, the
 * login never completed, and it showed "ACCOUNT ERROR" / "lost connection".
 * That was masked for a long time on the host because its EA session was still
 * cached; when the cache expired the host failed in exactly the same way.
 *
 * So authentication goes to EA, and only the game services we actually
 * implement are intercepted. Extra hosts can be added via passThroughHosts in
 * the bridge ini (comma separated).
 */
bool PassThroughToRealEA(LPCWSTR serverName) {
    if (!serverName) {
        return false;
    }
    static const std::vector<std::wstring> hosts = [] () -> std::vector<std::wstring> {
        std::vector<std::wstring> result = {
            L"accounts.ea.com",
            L"signin.ea.com",
        };
        std::wstring extra = ReadConfigValue(BridgeConfigPath(), L"passThroughHosts");
        size_t start = 0;
        while (start <= extra.size() && !extra.empty()) {
            const size_t comma = extra.find(L',', start);
            std::wstring token = extra.substr(
                start, comma == std::wstring::npos ? std::wstring::npos : comma - start);
            const size_t first = token.find_first_not_of(L" \t\r\n");
            const size_t last = token.find_last_not_of(L" \t\r\n");
            if (first != std::wstring::npos) {
                result.push_back(token.substr(first, last - first + 1));
            }
            if (comma == std::wstring::npos) {
                break;
            }
            start = comma + 1;
        }
        return result;
    }();
    for (const std::wstring& host : hosts) {
        if (_wcsicmp(serverName, host.c_str()) == 0) {
            return true;
        }
    }
    return false;
}

// getaddrinfo / GetAddrInfoW are hooked purely to learn which addresses belong
// to the authentication hosts, so connect() can leave those alone. The lookups
// themselves are untouched.
decltype(&getaddrinfo) gRealGetAddrInfo = nullptr;
decltype(&GetAddrInfoW) gRealGetAddrInfoW = nullptr;

int WSAAPI HookGetAddrInfo(
    PCSTR nodeName, PCSTR serviceName, const ADDRINFOA* hints, PADDRINFOA* result) {
    const auto real = gRealGetAddrInfo ? gRealGetAddrInfo : getaddrinfo;
    const int status = real(nodeName, serviceName, hints, result);
    if (status == 0 && result && *result && nodeName) {
        const std::wstring wide(nodeName, nodeName + strlen(nodeName));
        if (PassThroughToRealEA(wide.c_str())) {
            RememberPassThroughAddresses(*result);
            Log(L"pass-through: %S resolved; its addresses stay on real EA", nodeName);
        }
    }
    return status;
}

INT WSAAPI HookGetAddrInfoW(
    PCWSTR nodeName, PCWSTR serviceName, const ADDRINFOW* hints, PADDRINFOW* result) {
    const auto real = gRealGetAddrInfoW ? gRealGetAddrInfoW : GetAddrInfoW;
    const INT status = real(nodeName, serviceName, hints, result);
    if (status == 0 && result && *result && PassThroughToRealEA(nodeName)) {
        RememberPassThroughAddressesW(*result);
        Log(L"pass-through: %s resolved; its addresses stay on real EA", nodeName);
    }
    return status;
}

HINTERNET WINAPI HookWinHttpConnect(
    HINTERNET session, LPCWSTR serverName, INTERNET_PORT serverPort, DWORD reserved) {
    if (PassThroughToRealEA(serverName)) {
        Log(L"pass-through to real EA: %s:%u (authentication is not served locally)",
            serverName, serverPort);
        return gRealWinHttpConnect(session, serverName, serverPort, reserved);
    }
    if (serverName && !IsLoopbackHost(serverName)) {
        const LONG number = InterlockedIncrement(&gRedirectCount);
        // Must use the configured host, not a hardcoded loopback: on a joining
        // player's machine loopback is their own PC, where no server listens.
        Log(
            L"redirect #%ld via WinHttpConnect: %s:%u -> %s:%u",
            number, serverName, serverPort, RedirectTargetHostText(), kLocalBlazePort);
        return gRealWinHttpConnect(session, RedirectTargetHostText(), kLocalBlazePort, reserved);
    }
    return gRealWinHttpConnect(session, serverName, serverPort, reserved);
}

HINTERNET WINAPI HookWinHttpOpenRequest(
    HINTERNET connection,
    LPCWSTR verb,
    LPCWSTR objectName,
    LPCWSTR version,
    LPCWSTR referrer,
    LPCWSTR const* acceptTypes,
    DWORD flags) {
    HINTERNET request = gRealWinHttpOpenRequest(
        connection, verb, objectName, version, referrer, acceptTypes, flags);
    if (request && (flags & WINHTTP_FLAG_SECURE) && gRealWinHttpSetOption) {
        DWORD securityFlags =
            SECURITY_FLAG_IGNORE_UNKNOWN_CA |
            SECURITY_FLAG_IGNORE_CERT_DATE_INVALID |
            SECURITY_FLAG_IGNORE_CERT_CN_INVALID |
            SECURITY_FLAG_IGNORE_CERT_WRONG_USAGE;
        if (gRealWinHttpSetOption(
                request,
                WINHTTP_OPTION_SECURITY_FLAGS,
                &securityFlags,
                sizeof(securityFlags))) {
            Log(L"WinHTTP certificate checks relaxed for a redirected local TLS request");
        }
    }
    return request;
}

void* ReplacementForImport(const char* dll, const char* name, unsigned short ordinal, void* current) {
    const bool ws2 = dll && _stricmp(dll, "WS2_32.dll") == 0;
    const bool winhttp = dll && _stricmp(dll, "WINHTTP.dll") == 0;
    if (name) {
        if (ws2 && _stricmp(name, "connect") == 0) {
            return reinterpret_cast<void*>(HookConnect);
        }
        if (ws2 && _stricmp(name, "WSAConnect") == 0) {
            return reinterpret_cast<void*>(HookWSAConnect);
        }
        if (ws2 && _stricmp(name, "WSAIoctl") == 0) {
            return reinterpret_cast<void*>(HookWSAIoctl);
        }
        if (ws2 && _stricmp(name, "getaddrinfo") == 0) {
            return reinterpret_cast<void*>(HookGetAddrInfo);
        }
        if (ws2 && _stricmp(name, "GetAddrInfoW") == 0) {
            return reinterpret_cast<void*>(HookGetAddrInfoW);
        }
        if (winhttp && _stricmp(name, "WinHttpConnect") == 0) {
            return reinterpret_cast<void*>(HookWinHttpConnect);
        }
        if (winhttp && _stricmp(name, "WinHttpOpenRequest") == 0) {
            return reinterpret_cast<void*>(HookWinHttpOpenRequest);
        }
    }
    if (ws2 && ordinal == 4) {
        return reinterpret_cast<void*>(HookConnect);
    }
    if (ws2 && ordinal == 46) {
        return reinterpret_cast<void*>(HookWSAConnect);
    }
    if (ws2 && ordinal == 78) {
        return reinterpret_cast<void*>(HookWSAIoctl);
    }
    if (current == reinterpret_cast<void*>(gRealConnect)) {
        return reinterpret_cast<void*>(HookConnect);
    }
    if (current == reinterpret_cast<void*>(gRealWSAConnect)) {
        return reinterpret_cast<void*>(HookWSAConnect);
    }
    if (current == reinterpret_cast<void*>(gRealWSAIoctl)) {
        return reinterpret_cast<void*>(HookWSAIoctl);
    }
    if (current == reinterpret_cast<void*>(gRealWinHttpConnect)) {
        return reinterpret_cast<void*>(HookWinHttpConnect);
    }
    if (current == reinterpret_cast<void*>(gRealWinHttpOpenRequest)) {
        return reinterpret_cast<void*>(HookWinHttpOpenRequest);
    }
    return nullptr;
}

bool PatchPointerSlot(void** slot, void* replacement) {
    if (!ReadableMemory(slot, sizeof(*slot)) || *slot == replacement) {
        return false;
    }
    DWORD oldProtection = 0;
    if (!VirtualProtect(slot, sizeof(*slot), PAGE_READWRITE, &oldProtection)) {
        return false;
    }
    InterlockedExchangePointer(reinterpret_cast<PVOID volatile*>(slot), replacement);
    DWORD ignored = 0;
    VirtualProtect(slot, sizeof(*slot), oldProtection, &ignored);
    return true;
}

/*
 * The game resolves Winsock entry points once and calls them through its own
 * pointer slots, so patching import tables is not enough to observe or redirect
 * its traffic. Those slots used to be named by fixed RVAs below, which silently
 * stopped matching when a title update moved them: the guard declined to patch,
 * the game reached its main menu, and every connection went straight to the real
 * Winsock without ever appearing in this log.
 *
 * Discover them the same way the ProtoSSL patch point is discovered — by
 * scanning at runtime — so a title update relocating them costs nothing. Any
 * writable slot in the game image holding a resolved entry point is a call path
 * we need to own.
 */
bool IsWritableDataProtection(const DWORD protection) {
    if (protection & (PAGE_GUARD | PAGE_NOACCESS)) {
        return false;
    }
    switch (protection & 0xFF) {
    case PAGE_READWRITE:
    case PAGE_WRITECOPY:
    case PAGE_EXECUTE_READWRITE:
    case PAGE_EXECUTE_WRITECOPY:
        return true;
    default:
        return false;
    }
}

void ScanImageForPointerSlots(
    unsigned char* base,
    const size_t imageSize,
    void* target,
    std::vector<void**>& found) {
    if (!target || !base || imageSize == 0) {
        return;
    }
    unsigned char* const imageEnd = base + imageSize;
    for (unsigned char* cursor = base; cursor < imageEnd;) {
        MEMORY_BASIC_INFORMATION info = {};
        if (VirtualQuery(cursor, &info, sizeof(info)) != sizeof(info) || info.RegionSize == 0) {
            break;
        }
        auto* regionStart = (std::max)(cursor, static_cast<unsigned char*>(info.BaseAddress));
        auto* queriedEnd = static_cast<unsigned char*>(info.BaseAddress) + info.RegionSize;
        auto* regionEnd = (std::min)(queriedEnd, imageEnd);
        if (info.State == MEM_COMMIT && IsWritableDataProtection(info.Protect) && regionEnd > regionStart) {
            const uintptr_t alignment = sizeof(void*);
            auto* scan = reinterpret_cast<unsigned char*>(
                (reinterpret_cast<uintptr_t>(regionStart) + alignment - 1) & ~(alignment - 1));
            for (; scan + sizeof(void*) <= regionEnd; scan += sizeof(void*)) {
                auto** slot = reinterpret_cast<void**>(scan);
                if (*slot == target) {
                    found.push_back(slot);
                }
            }
        }
        cursor = regionEnd > cursor ? regionEnd : cursor + 0x1000;
    }
}

size_t PatchDiscoveredGameSlots(unsigned char* base, const size_t imageSize) {
    static bool reported = false;
    size_t patched = 0;
    size_t connectCount = 0;
    size_t ioctlCount = 0;
    std::vector<void**> slots;

    ScanImageForPointerSlots(base, imageSize, reinterpret_cast<void*>(gRealConnect), slots);
    connectCount = slots.size();
    for (auto** slot : slots) {
        if (PatchPointerSlot(slot, reinterpret_cast<void*>(HookConnect))) {
            ++patched;
        }
    }

    slots.clear();
    ScanImageForPointerSlots(base, imageSize, reinterpret_cast<void*>(gRealWSAIoctl), slots);
    ioctlCount = slots.size();
    for (auto** slot : slots) {
        if (PatchPointerSlot(slot, reinterpret_cast<void*>(HookWSAIoctl))) {
            ++patched;
        }
    }

    // Report the first outcome unconditionally: "found nothing" is the signal
    // that the game moved to a call path this scan does not cover, and silence
    // is exactly what made the previous breakage so hard to place.
    if (!reported) {
        reported = true;
        Log(L"resolved-pointer slot scan: %zu connect, %zu WSAIoctl slot(s) discovered in the game image",
            connectCount, ioctlCount);
    }
    return patched;
}

size_t PatchKnownGameSlots(HMODULE executable) {
    auto* base = reinterpret_cast<unsigned char*>(executable);
    size_t patched = 0;
    // Retained for the build these offsets were derived from. Each entry is
    // guarded on its expected value, so a build that moved them simply skips.
    const struct {
        uintptr_t rva;
        void* expected;
        void* replacement;
    } slots[] = {
        {kGameConnectSlotRva, reinterpret_cast<void*>(gRealConnect), reinterpret_cast<void*>(HookConnect)},
        {kGameWSAIoctlSlotRva, reinterpret_cast<void*>(gRealWSAIoctl), reinterpret_cast<void*>(HookWSAIoctl)},
        {kGameConnectIatRva, reinterpret_cast<void*>(gRealConnect), reinterpret_cast<void*>(HookConnect)},
        {kGameWSAIoctlIatRva, reinterpret_cast<void*>(gRealWSAIoctl), reinterpret_cast<void*>(HookWSAIoctl)},
    };
    for (const auto& slot : slots) {
        auto** address = reinterpret_cast<void**>(base + slot.rva);
        if (ReadableMemory(address, sizeof(*address)) && *address == slot.expected &&
            PatchPointerSlot(address, slot.replacement)) {
            ++patched;
        }
    }
    return patched;
}

size_t PatchModuleImports(HMODULE module) {
    auto* base = reinterpret_cast<unsigned char*>(module);
    if (!ReadableMemory(base, sizeof(IMAGE_DOS_HEADER))) {
        return 0;
    }
    auto* dos = reinterpret_cast<IMAGE_DOS_HEADER*>(base);
    if (dos->e_magic != IMAGE_DOS_SIGNATURE) {
        return 0;
    }
    auto* nt = reinterpret_cast<IMAGE_NT_HEADERS64*>(base + dos->e_lfanew);
    if (!ReadableMemory(nt, sizeof(*nt)) || nt->Signature != IMAGE_NT_SIGNATURE) {
        return 0;
    }
    const IMAGE_DATA_DIRECTORY imports = nt->OptionalHeader.DataDirectory[IMAGE_DIRECTORY_ENTRY_IMPORT];
    size_t patched = 0;
    if (imports.VirtualAddress && imports.Size) {
        auto* descriptor = reinterpret_cast<IMAGE_IMPORT_DESCRIPTOR*>(base + imports.VirtualAddress);
        const size_t descriptorLimit = imports.Size / sizeof(IMAGE_IMPORT_DESCRIPTOR);
        for (size_t descriptorIndex = 0; descriptorIndex < descriptorLimit; ++descriptorIndex, ++descriptor) {
        if (!ReadableMemory(descriptor, sizeof(*descriptor)) || descriptor->Name == 0) {
            break;
        }
        const char* dll = reinterpret_cast<const char*>(base + descriptor->Name);
        if (!ReadableMemory(dll, 16)) {
            continue;
        }
        auto* iat = reinterpret_cast<IMAGE_THUNK_DATA64*>(base + descriptor->FirstThunk);
        auto* lookup = descriptor->OriginalFirstThunk
            ? reinterpret_cast<IMAGE_THUNK_DATA64*>(base + descriptor->OriginalFirstThunk)
            : nullptr;
        for (size_t thunkIndex = 0;; ++thunkIndex) {
            auto* iatEntry = iat + thunkIndex;
            if (!ReadableMemory(iatEntry, sizeof(*iatEntry)) || iatEntry->u1.Function == 0) {
                break;
            }
            const char* importName = nullptr;
            unsigned short ordinal = 0;
            if (lookup && ReadableMemory(lookup + thunkIndex, sizeof(*lookup))) {
                const ULONGLONG ordinalOrName = lookup[thunkIndex].u1.Ordinal;
                if (ordinalOrName & IMAGE_ORDINAL_FLAG64) {
                    ordinal = static_cast<unsigned short>(ordinalOrName & 0xFFFF);
                } else if (ordinalOrName) {
                    auto* import = reinterpret_cast<IMAGE_IMPORT_BY_NAME*>(base + ordinalOrName);
                    if (ReadableMemory(import, sizeof(*import) + 64)) {
                        importName = reinterpret_cast<const char*>(import->Name);
                    }
                }
            }
            void** slot = reinterpret_cast<void**>(&iatEntry->u1.Function);
            void* current = *slot;
            void* replacement = ReplacementForImport(dll, importName, ordinal, current);
            if (!replacement || current == replacement) {
                continue;
            }
            patched += PatchPointerSlot(slot, replacement) ? 1 : 0;
        }
    }
    }

    struct DelayDescriptor {
        DWORD attributes;
        DWORD name;
        DWORD moduleHandle;
        DWORD iat;
        DWORD nameTable;
        DWORD boundIat;
        DWORD unloadIat;
        DWORD timestamp;
    };
    const IMAGE_DATA_DIRECTORY delays = nt->OptionalHeader.DataDirectory[IMAGE_DIRECTORY_ENTRY_DELAY_IMPORT];
    if (!delays.VirtualAddress || !delays.Size) {
        return patched;
    }
    auto* delay = reinterpret_cast<DelayDescriptor*>(base + delays.VirtualAddress);
    const size_t delayLimit = delays.Size / sizeof(DelayDescriptor);
    for (size_t descriptorIndex = 0; descriptorIndex < delayLimit; ++descriptorIndex, ++delay) {
        if (!ReadableMemory(delay, sizeof(*delay)) || delay->name == 0) {
            break;
        }
        const bool rvaBased = (delay->attributes & 1) != 0;
        auto address = [base, rvaBased](ULONGLONG value) -> unsigned char* {
            return rvaBased ? base + value : reinterpret_cast<unsigned char*>(value);
        };
        const char* dll = reinterpret_cast<const char*>(address(delay->name));
        auto* iat = reinterpret_cast<IMAGE_THUNK_DATA64*>(address(delay->iat));
        auto* lookup = reinterpret_cast<IMAGE_THUNK_DATA64*>(address(delay->nameTable));
        if (!ReadableMemory(dll, 16)) {
            continue;
        }
        for (size_t thunkIndex = 0;; ++thunkIndex) {
            auto* iatEntry = iat + thunkIndex;
            auto* lookupEntry = lookup + thunkIndex;
            if (!ReadableMemory(iatEntry, sizeof(*iatEntry)) ||
                !ReadableMemory(lookupEntry, sizeof(*lookupEntry)) || lookupEntry->u1.Ordinal == 0) {
                break;
            }
            const ULONGLONG ordinalOrName = lookupEntry->u1.Ordinal;
            const char* importName = nullptr;
            unsigned short ordinal = 0;
            if (ordinalOrName & IMAGE_ORDINAL_FLAG64) {
                ordinal = static_cast<unsigned short>(ordinalOrName & 0xFFFF);
            } else {
                auto* import = reinterpret_cast<IMAGE_IMPORT_BY_NAME*>(address(ordinalOrName));
                if (ReadableMemory(import, sizeof(*import) + 64)) {
                    importName = reinterpret_cast<const char*>(import->Name);
                }
            }
            void** slot = reinterpret_cast<void**>(&iatEntry->u1.Function);
            void* current = *slot;
            void* replacement = ReplacementForImport(dll, importName, ordinal, current);
            if (replacement && current != replacement && PatchPointerSlot(slot, replacement)) {
                ++patched;
            }
        }
    }
    return patched;
}

size_t PatchLoadedModuleImports(HMODULE ws2) {
    std::array<HMODULE, 2048> modules = {};
    DWORD required = 0;
    if (!EnumProcessModules(
            GetCurrentProcess(), modules.data(), static_cast<DWORD>(sizeof(modules)), &required)) {
        return 0;
    }
    const size_t count = std::min(modules.size(), static_cast<size_t>(required / sizeof(HMODULE)));
    size_t patched = 0;
    for (size_t index = 0; index < count; ++index) {
        if (modules[index] && modules[index] != ws2) {
            patched += PatchModuleImports(modules[index]);
        }
    }
    return patched;
}

DWORD WINAPI RedirectWorker(void*) {
    Log(L"RedirectWorker: thread entered");
    if (!PrivateOnlineDynastyEnabled()) {
        return 0;
    }
    // When injected mid-session the game's networking and TLS are already live.
    // Give the loader lock time to release and the just-created threads time to
    // settle before touching the game's code and pointer tables.
    if (CaptureModeEnabled()) {
        Sleep(1500);
        Log(L"RedirectWorker: capture-mode settle delay elapsed");
    }
    HMODULE executable = GetModuleHandleW(nullptr);
    if (!executable) {
        Log(L"private Online Dynasty enforcement failed: executable module unavailable");
        return 0;
    }
    HMODULE ws2 = GetModuleHandleW(L"ws2_32.dll");
    if (!ws2) {
        ws2 = LoadLibraryW(L"ws2_32.dll");
    }
    if (!ws2) {
        Log(L"unable to load ws2_32.dll for traffic redirection");
        return 0;
    }
    gRealConnect = reinterpret_cast<ConnectFn>(GetProcAddress(ws2, "connect"));
    gRealWSAConnect = reinterpret_cast<WSAConnectFn>(GetProcAddress(ws2, "WSAConnect"));
    gRealWSAIoctl = reinterpret_cast<WSAIoctlFn>(GetProcAddress(ws2, "WSAIoctl"));
    if (!gRealConnect || !gRealWSAConnect || !gRealWSAIoctl) {
        Log(L"one or more required Winsock entry points could not be resolved");
        return 0;
    }

    HMODULE winhttp = GetModuleHandleW(L"winhttp.dll");
    if (!winhttp) {
        winhttp = LoadLibraryW(L"winhttp.dll");
    }
    if (winhttp) {
        gRealWinHttpConnect = reinterpret_cast<WinHttpConnectFn>(
            GetProcAddress(winhttp, "WinHttpConnect"));
        gRealWinHttpOpenRequest = reinterpret_cast<WinHttpOpenRequestFn>(
            GetProcAddress(winhttp, "WinHttpOpenRequest"));
        gRealWinHttpSetOption = reinterpret_cast<WinHttpSetOptionFn>(
            GetProcAddress(winhttp, "WinHttpSetOption"));
    }

    if (CaptureModeEnabled()) {
        Log(
            L"CAPTURE MODE: non-loopback IPv4/IPv6 -> 127.0.0.1 at original port +%u; "
            L"traffic reaches EA through the local capture proxy and is recorded",
            kCapturePortOffset);
    } else {
        Log(
            L"private Online Dynasty network enforcement armed: all non-loopback IPv4/IPv6 -> %u.%u.%u.%u:%u; external pass-through disabled",
            (RedirectTargetAddress() >> 24) & 0xff, (RedirectTargetAddress() >> 16) & 0xff,
            (RedirectTargetAddress() >> 8) & 0xff, RedirectTargetAddress() & 0xff,
            kLocalBlazePort);
    }
    // The pointer-slot scan walks the game image's writable data, so run it on a
    // slower cadence than the import sweep rather than every tick.
    size_t imageSize = 0;
    MODULEINFO moduleInfo = {};
    if (GetModuleInformation(GetCurrentProcess(), executable, &moduleInfo, sizeof(moduleInfo))) {
        imageSize = moduleInfo.SizeOfImage;
    } else {
        Log(L"unable to size the game image; resolved-pointer slot scanning is disabled");
    }
    unsigned tick = 0;
    for (;;) {
        size_t patched = PatchKnownGameSlots(executable) + PatchLoadedModuleImports(ws2);
        if (imageSize != 0 && (tick % 8) == 0) {
            patched += PatchDiscoveredGameSlots(reinterpret_cast<unsigned char*>(executable), imageSize);
        }
        if (patched) {
            Log(L"installed %zu Winsock import hook(s)", patched);
        }
        ++tick;
        Sleep(250);
    }
}

bool IsExecutableProtection(const DWORD protection) {
    if (protection & (PAGE_GUARD | PAGE_NOACCESS)) {
        return false;
    }
    switch (protection & 0xFF) {
    case PAGE_EXECUTE:
    case PAGE_EXECUTE_READ:
    case PAGE_EXECUTE_READWRITE:
    case PAGE_EXECUTE_WRITECOPY:
        return true;
    default:
        return false;
    }
}

struct RuntimeSignatureMatches {
    unsigned char* address = nullptr;
    size_t count = 0;
};

RuntimeSignatureMatches ScanExecutableImage(
    unsigned char* base,
    const size_t imageSize,
    const std::span<const std::uint8_t> signature) {
    RuntimeSignatureMatches matches;
    unsigned char* const imageEnd = base + imageSize;
    for (unsigned char* cursor = base; cursor < imageEnd;) {
        MEMORY_BASIC_INFORMATION info = {};
        if (VirtualQuery(cursor, &info, sizeof(info)) != sizeof(info) || info.RegionSize == 0) {
            break;
        }
        auto* regionStart = (std::max)(cursor, static_cast<unsigned char*>(info.BaseAddress));
        auto* queriedEnd = static_cast<unsigned char*>(info.BaseAddress) + info.RegionSize;
        auto* regionEnd = (std::min)(queriedEnd, imageEnd);
        if (info.State == MEM_COMMIT && IsExecutableProtection(info.Protect) &&
            regionEnd > regionStart && static_cast<size_t>(regionEnd - regionStart) >= signature.size()) {
            const auto result = Cypress::CFB27::RuntimeScan::FindUniquePattern(
                std::span<const std::uint8_t>(regionStart, static_cast<size_t>(regionEnd - regionStart)),
                signature);
            if (result.count != 0) {
                if (matches.count == 0 && result.count == 1) {
                    matches.address = regionStart + result.offset;
                } else {
                    matches.address = nullptr;
                }
                matches.count += result.count;
                if (matches.count > 1) {
                    matches.address = nullptr;
                    return matches;
                }
            }
        }
        cursor = regionEnd > cursor ? regionEnd : cursor + 0x1000;
    }
    return matches;
}

DWORD WINAPI PatchWorker(void*) {
    using namespace Cypress::CFB27::RuntimeScan;
    Log(L"PatchWorker: thread entered");
    if (!PrivateOnlineDynastyEnabled()) {
        Log(L"bridge %s loaded but private Online Dynasty lease is inactive; bridge remains inert", kBridgeVersion);
        return 0;
    }
    Log(L"bridge %s loaded; runtime ProtoSSL signature scan started", kBridgeVersion);
    // State the inputs, not just the conclusion: when a client redirects to the
    // wrong place the question is always "which config did it actually read?".
    Log(L"config file: %s", BridgeConfigPath().c_str());
    Log(L"config blazeHost=[%s] env CYPRESS_CFB27_BLAZE_HOST=[%s] -> redirect target %s:%u",
        ReadConfigValue(BridgeConfigPath(), L"blazeHost").c_str(),
        ReadEnvironment(L"CYPRESS_CFB27_BLAZE_HOST").c_str(),
        RedirectTargetHostText(), kLocalBlazePort);
    HMODULE executable = GetModuleHandleW(nullptr);
    if (!executable) {
        Log(L"unable to obtain the executable module");
        return 0;
    }

    auto* base = reinterpret_cast<unsigned char*>(executable);
    auto* dos = reinterpret_cast<IMAGE_DOS_HEADER*>(base);
    if (!ReadableMemory(base, sizeof(*dos)) || dos->e_magic != IMAGE_DOS_SIGNATURE) {
        Log(L"main module has no readable DOS header");
        return 0;
    }
    auto* nt = reinterpret_cast<IMAGE_NT_HEADERS64*>(base + dos->e_lfanew);
    if (!ReadableMemory(nt, sizeof(*nt)) || nt->Signature != IMAGE_NT_SIGNATURE) {
        Log(L"main module has no readable PE header");
        return 0;
    }
    const DWORD timestamp = nt->FileHeader.TimeDateStamp;
    const DWORD imageSize = nt->OptionalHeader.SizeOfImage;
    if (imageSize < kVerifiedSignature.size() || imageSize > 0x40000000) {
        Log(L"invalid executable image size=0x%08lX; no patch applied", imageSize);
        return 0;
    }
    Log(L"runtime executable metadata timestamp=0x%08lX imageSize=0x%08lX", timestamp, imageSize);

    std::array<std::uint8_t, kVerifiedSignature.size()> alreadyPatched = kVerifiedSignature;
    for (size_t index = 0; index < kReplacementBytes.size(); ++index) {
        alreadyPatched[kPatchOffsetInSignature + index] = kReplacementBytes[index];
    }

    constexpr DWORD kWaitMilliseconds = 10 * 60 * 1000;
    constexpr DWORD kPollMilliseconds = 50;
    size_t lastMatchCount = static_cast<size_t>(-1);
    for (DWORD elapsed = 0; elapsed < kWaitMilliseconds; elapsed += kPollMilliseconds) {
        const auto patchedMatches = ScanExecutableImage(base, imageSize, alreadyPatched);
        if (patchedMatches.count > 1) {
            Log(L"runtime patched signature matches=%zu; ambiguous image, no patch applied", patchedMatches.count);
            return 0;
        }
        if (patchedMatches.count == 1 && patchedMatches.address) {
            const uintptr_t rva = static_cast<uintptr_t>(patchedMatches.address - base);
            Log(L"certificate validation was already disabled at discovered RVA 0x%08llX", rva + kPatchOffsetInSignature);
            return 0;
        }

        // The certificate-pinning bypass can be turned off for diagnosis. It is a
        // byte patch located by signature scan, so on a build it was not
        // calibrated against it may land somewhere harmless-looking and still
        // change behaviour. With the redirector now tunnelled to real EA, the
        // patch is not needed for that connection, which makes "off" a usable
        // comparison rather than a guaranteed failure.
        // Read from the ini as well as the environment: the game is not always
        // launched as a child of the shell that set the variable, so an
        // environment-only switch can silently fail to arrive.
        if (ReadEnvironment(L"CYPRESS_CFB27_NO_PROTOSSL_PATCH") == L"1" ||
            ReadConfigValue(BridgeConfigPath(), L"noProtoSslPatch") == L"1") {
            Log(L"ProtoSSL verifier patch disabled by CYPRESS_CFB27_NO_PROTOSSL_PATCH");
            return 0;
        }

        const auto matches = ScanExecutableImage(base, imageSize, kVerifiedSignature);
        if (matches.count != lastMatchCount) {
            Log(L"runtime signature matches=%zu", matches.count);
            lastMatchCount = matches.count;
        }
        if (matches.count > 1) {
            Log(L"runtime signature is ambiguous; no patch applied");
            return 0;
        }
        if (matches.count == 1 && matches.address) {
            const uintptr_t signatureRva = static_cast<uintptr_t>(matches.address - base);
            unsigned char* const patchAddress = matches.address + kPatchOffsetInSignature;
            const uintptr_t patchRva = signatureRva + kPatchOffsetInSignature;
            Log(L"runtime signature discovered rva=0x%08llX patchRva=0x%08llX", signatureRva, patchRva);
            if (!BytesEqual(matches.address, kVerifiedSignature.data(), kVerifiedSignature.size())) {
                Log(L"runtime signature changed before patch; no patch applied");
                return 0;
            }
            DWORD oldProtection = 0;
            if (!VirtualProtect(patchAddress, kReplacementBytes.size(), PAGE_EXECUTE_READWRITE, &oldProtection)) {
                Log(L"VirtualProtect failed at RVA 0x%08llX error=%lu", patchRva, GetLastError());
                return 0;
            }
            memcpy(patchAddress, kReplacementBytes.data(), kReplacementBytes.size());
            FlushInstructionCache(GetCurrentProcess(), patchAddress, kReplacementBytes.size());
            DWORD ignored = 0;
            VirtualProtect(patchAddress, kReplacementBytes.size(), oldProtection, &ignored);
            if (!BytesEqual(patchAddress, kReplacementBytes.data(), kReplacementBytes.size())) {
                Log(L"write verification failed at RVA 0x%08llX", patchRva);
                return 0;
            }
            Log(
                L"forced ProtoSSL verifier result success at discovered RVA 0x%08llX; parsing, key extraction, and TLS state machine remain active",
                patchRva);
            return 0;
        }
        Sleep(kPollMilliseconds);
    }
    Log(L"timed out waiting for the dump-derived signature; no patch applied");
    return 0;
}

}  // namespace

extern "C" HRESULT WINAPI DirectInput8Create(
    HINSTANCE instance, DWORD version, REFIID interfaceId, LPVOID* output, LPUNKNOWN outer) {
    EnsureProxy();
    return gDirectInput8Create
        ? gDirectInput8Create(instance, version, interfaceId, output, outer)
        : HRESULT_FROM_WIN32(ERROR_PROC_NOT_FOUND);
}

extern "C" HRESULT WINAPI DllCanUnloadNow() {
    EnsureProxy();
    return gDllCanUnloadNow ? gDllCanUnloadNow() : S_FALSE;
}

extern "C" HRESULT WINAPI DllGetClassObject(REFCLSID classId, REFIID interfaceId, LPVOID* output) {
    EnsureProxy();
    return gDllGetClassObject
        ? gDllGetClassObject(classId, interfaceId, output)
        : HRESULT_FROM_WIN32(ERROR_PROC_NOT_FOUND);
}

extern "C" HRESULT WINAPI DllRegisterServer() {
    EnsureProxy();
    return gDllRegisterServer ? gDllRegisterServer() : HRESULT_FROM_WIN32(ERROR_PROC_NOT_FOUND);
}

extern "C" HRESULT WINAPI DllUnregisterServer() {
    EnsureProxy();
    return gDllUnregisterServer ? gDllUnregisterServer() : HRESULT_FROM_WIN32(ERROR_PROC_NOT_FOUND);
}

extern "C" const DIDATAFORMAT* WINAPI GetdfDIJoystick() {
    EnsureProxy();
    return gGetdfDIJoystick ? gGetdfDIJoystick() : nullptr;
}

BOOL WINAPI DllMain(HINSTANCE instance, DWORD reason, LPVOID) {
    if (reason == DLL_PROCESS_ATTACH) {
        // No logging here: DllMain runs under the loader lock, and Log() creates
        // directories, which can load a module and deadlock. The workers log
        // their first step once the lock is released.
        DisableThreadLibraryCalls(instance);
        HANDLE patchWorker = CreateThread(nullptr, 0, PatchWorker, nullptr, 0, nullptr);
        if (patchWorker) {
            CloseHandle(patchWorker);
        }
        HANDLE redirectWorker = CreateThread(nullptr, 0, RedirectWorker, nullptr, 0, nullptr);
        if (redirectWorker) {
            CloseHandle(redirectWorker);
        }
    }
    return TRUE;
}
