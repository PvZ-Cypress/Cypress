// capturehook.dll — records decrypted Blaze traffic from a running CFB27.
//
// EA statically links DirtySDK, whose ProtoSSL layer encrypts on send and
// decrypts on receive. Hooking ProtoSSLSend/ProtoSSLRecv therefore sees the
// plaintext application bytes — the raw Blaze frames — with no proxy, no cert
// handling, and no dependence on which EA host the game talked to. This is how
// EA-MITM produces its dumps; this is a version we control and can re-point at
// each game build.
//
// Two reasons the earlier attempts failed inform the design here:
//   * Hardcoded addresses from a mismatched build land mid-instruction and
//     crash when overwritten. So this defaults to PROBE mode, which only reads
//     and logs the bytes at the configured addresses. Probe cannot crash; it
//     tells us whether the addresses are real function entries before anything
//     is patched.
//   * Overwriting a hot function while another thread runs it tears the
//     instruction stream. HOOK mode uses MinHook, which suspends threads across
//     the patch, so ProtoSSLSend/Recv can be hooked safely.
//
// Config is capturehook.ini beside the DLL:
//   [Capture]
//   Mode=probe            ; probe (safe, default) or hook
//   DumpDir=              ; where to write; defaults beside the DLL
//   [ProtoSSL]
//   Connect=0x016D4120    ; RVAs from the game image base
//   Send=0x016D4940
//   Recv=0x016D4830
#define WIN32_LEAN_AND_MEAN
#define NOMINMAX
#include <windows.h>
#include <tlhelp32.h>

#include <cstdint>
#include <cstdio>
#include <string>

#include "MinHook.h"

namespace {

std::wstring gModuleDir;
std::wstring gDumpDir;
// Raw Win32 handles, not CRT FILE*: the C runtime's file layer is unreliable
// inside a freshly injected module, whereas CreateFileW/WriteFile are not.
HANDLE gLog = INVALID_HANDLE_VALUE;
HANDLE gDump = INVALID_HANDLE_VALUE;
CRITICAL_SECTION gLock;

HANDLE OpenAppend(const std::wstring& path) {
    return CreateFileW(path.c_str(), FILE_APPEND_DATA, FILE_SHARE_READ | FILE_SHARE_WRITE,
                       nullptr, OPEN_ALWAYS, FILE_ATTRIBUTE_NORMAL, nullptr);
}

uintptr_t gConnectRva = 0;
uintptr_t gSendRva = 0;
uintptr_t gRecvRva = 0;

std::wstring DirectoryOf(HMODULE module) {
    wchar_t path[MAX_PATH] = {};
    GetModuleFileNameW(module, path, MAX_PATH);
    std::wstring full(path);
    const size_t slash = full.find_last_of(L"\\/");
    return slash == std::wstring::npos ? L"." : full.substr(0, slash);
}

void LogLine(const char* format, ...) {
    char body[1024] = {};
    va_list args;
    va_start(args, format);
    _vsnprintf_s(body, sizeof(body), _TRUNCATE, format, args);
    va_end(args);
    SYSTEMTIME now = {};
    GetLocalTime(&now);
    char line[1200] = {};
    int n = _snprintf_s(line, sizeof(line), _TRUNCATE, "%02u:%02u:%02u.%03u %s\r\n",
                        now.wHour, now.wMinute, now.wSecond, now.wMilliseconds, body);
    EnterCriticalSection(&gLock);
    if (gLog != INVALID_HANDLE_VALUE && n > 0) {
        DWORD written = 0;
        WriteFile(gLog, line, static_cast<DWORD>(n), &written, nullptr);
    }
    LeaveCriticalSection(&gLock);
}

std::wstring IniPath() { return gModuleDir + L"\\capturehook.ini"; }

std::wstring ReadIni(const wchar_t* section, const wchar_t* key, const wchar_t* fallback) {
    wchar_t buffer[512] = {};
    GetPrivateProfileStringW(section, key, fallback, buffer, 512, IniPath().c_str());
    return std::wstring(buffer);
}

uintptr_t ReadIniAddress(const wchar_t* key) {
    const std::wstring value = ReadIni(L"ProtoSSL", key, L"0");
    return static_cast<uintptr_t>(_wcstoui64(value.c_str(), nullptr, 0));
}

// ---- capture dump -------------------------------------------------------
// Framed as [dir:1][len:4 LE][bytes]; a companion Go tool reassembles each
// direction and parses Blaze frames with the shared decoder.
// Record: [dir:1][conn:8 LE][len:4 LE][bytes]. conn is the ProtoSSL state
// pointer, which separates the Blaze session from auth/CDN streams that share
// these same functions.
void DumpRecord(uint8_t direction, uint64_t connection, const void* data, int32_t length) {
    if (length <= 0 || data == nullptr) {
        return;
    }
    uint8_t header[13];
    header[0] = direction;
    for (int i = 0; i < 8; ++i) header[1 + i] = static_cast<uint8_t>((connection >> (i * 8)) & 0xff);
    for (int i = 0; i < 4; ++i) header[9 + i] = static_cast<uint8_t>((static_cast<uint32_t>(length) >> (i * 8)) & 0xff);
    EnterCriticalSection(&gLock);
    if (gDump != INVALID_HANDLE_VALUE) {
        DWORD written = 0;
        WriteFile(gDump, header, sizeof(header), &written, nullptr);
        WriteFile(gDump, data, static_cast<DWORD>(length), &written, nullptr);
    }
    LeaveCriticalSection(&gLock);
}

// ---- ProtoSSL hook signatures (DirtySDK, x64) --------------------------
using ProtoSSLSendFn = int32_t(__fastcall*)(void* state, const char* buffer, int32_t length);
using ProtoSSLRecvFn = int32_t(__fastcall*)(void* state, char* buffer, int32_t length);
using ProtoSSLConnectFn =
    int32_t(__fastcall*)(void* state, int32_t secure, const char* addr, uint32_t uaddr, int32_t port);

ProtoSSLSendFn gOriginalSend = nullptr;
ProtoSSLRecvFn gOriginalRecv = nullptr;
ProtoSSLConnectFn gOriginalConnect = nullptr;

int32_t __fastcall HookSend(void* state, const char* buffer, int32_t length) {
    int32_t effective = length;
    if (effective < 0 && buffer) {
        effective = static_cast<int32_t>(strlen(buffer));
    }
    DumpRecord(/*client_to_server*/ 1, reinterpret_cast<uint64_t>(state), buffer, effective);
    return gOriginalSend(state, buffer, length);
}

int32_t __fastcall HookRecv(void* state, char* buffer, int32_t length) {
    const int32_t received = gOriginalRecv(state, buffer, length);
    if (received > 0) {
        DumpRecord(/*server_to_client*/ 2, reinterpret_cast<uint64_t>(state), buffer, received);
    }
    return received;
}

// ======================================================================
// Hardware-breakpoint capture (for Denuvo-protected .text).
//
// The game's code pages cannot be written (IN_PAGE_ERROR), so no code-patching
// hook works. Debug registers break on execution reaching an address without
// modifying any byte, so page protection is irrelevant. We set Dr0=Send entry,
// Dr1=Recv entry, and read arguments from the trapped thread context. For Recv
// the payload only exists after the call returns, so on a Recv entry we arm
// Dr2 at the return address and read the filled buffer when it trips.
// ======================================================================
namespace hwbp {

uintptr_t gSend = 0;
uintptr_t gRecv = 0;

// Pending Recv return, keyed by thread. Blaze traffic runs on one pump thread in
// practice, but a small table keeps concurrent calls from clobbering each other.
struct PendingRecv {
    DWORD threadId;
    uintptr_t returnAddress;
    char* buffer;
    uint64_t connection;
};
PendingRecv gPending[16] = {};

PendingRecv* FindPending(DWORD threadId) {
    for (auto& entry : gPending) {
        if (entry.threadId == threadId) {
            return &entry;
        }
    }
    return nullptr;
}

void SetDebugRegister(CONTEXT& context, int slot, uintptr_t address, bool enable) {
    if (slot == 0) context.Dr0 = address;
    if (slot == 1) context.Dr1 = address;
    if (slot == 2) context.Dr2 = address;
    const int localBit = slot * 2;              // L0/L1/L2 local-enable
    const int rwLenBase = 16 + slot * 4;        // R/W and LEN fields
    if (enable) {
        context.Dr7 |= (1ull << localBit);
        // R/W = 00 (execute), LEN = 00 (1 byte). Clear both fields.
        context.Dr7 &= ~(0xFull << rwLenBase);
    } else {
        context.Dr7 &= ~(1ull << localBit);
        if (slot == 0) context.Dr0 = 0;
        if (slot == 1) context.Dr1 = 0;
        if (slot == 2) context.Dr2 = 0;
    }
}

// Arm a slot across every thread in the process.
void ArmAllThreads(int slot, uintptr_t address, bool enable) {
    HANDLE snapshot = CreateToolhelp32Snapshot(TH32CS_SNAPTHREAD, 0);
    if (snapshot == INVALID_HANDLE_VALUE) {
        return;
    }
    THREADENTRY32 entry = {};
    entry.dwSize = sizeof(entry);
    const DWORD self = GetCurrentThreadId();
    const DWORD pid = GetCurrentProcessId();
    if (Thread32First(snapshot, &entry)) {
        do {
            if (entry.th32OwnerProcessID != pid || entry.th32ThreadID == self) {
                continue;
            }
            HANDLE thread = OpenThread(THREAD_GET_CONTEXT | THREAD_SET_CONTEXT | THREAD_SUSPEND_RESUME,
                                       FALSE, entry.th32ThreadID);
            if (!thread) {
                continue;
            }
            SuspendThread(thread);
            CONTEXT context = {};
            context.ContextFlags = CONTEXT_DEBUG_REGISTERS;
            if (GetThreadContext(thread, &context)) {
                SetDebugRegister(context, slot, address, enable);
                context.ContextFlags = CONTEXT_DEBUG_REGISTERS;
                SetThreadContext(thread, &context);
            }
            ResumeThread(thread);
            CloseHandle(thread);
        } while (Thread32Next(snapshot, &entry));
    }
    CloseHandle(snapshot);
}

// Arm Dr2 (Recv return) on just the trapped thread, via its live context.
void ArmReturnOnContext(CONTEXT* context, uintptr_t address, bool enable) {
    SetDebugRegister(*context, 2, address, enable);
}

LONG CALLBACK VectoredHandler(EXCEPTION_POINTERS* info) {
    if (info->ExceptionRecord->ExceptionCode != EXCEPTION_SINGLE_STEP) {
        return EXCEPTION_CONTINUE_SEARCH;
    }
    CONTEXT* context = info->ContextRecord;
    const DWORD64 dr6 = context->Dr6;
    const DWORD threadId = GetCurrentThreadId();

    if (dr6 & 0x1) {  // Dr0 = Send entry: RCX=state, RDX=buffer, R8=length (x64 ABI).
        const char* buffer = reinterpret_cast<const char*>(context->Rdx);
        int32_t length = static_cast<int32_t>(context->R8);
        if (length < 0 && buffer) length = static_cast<int32_t>(strlen(buffer));
        DumpRecord(1, context->Rcx, buffer, length);
    }
    if (dr6 & 0x2) {  // Dr1 = Recv entry: RCX=state, RDX=buffer, return addr at [RSP].
        char* buffer = reinterpret_cast<char*>(context->Rdx);
        uintptr_t returnAddress = 0;
        if (context->Rsp) {
            returnAddress = *reinterpret_cast<uintptr_t*>(context->Rsp);
        }
        PendingRecv* slot = FindPending(threadId);
        if (!slot) slot = FindPending(0);
        if (slot && returnAddress) {
            slot->threadId = threadId;
            slot->returnAddress = returnAddress;
            slot->buffer = buffer;
            slot->connection = context->Rcx;
            ArmReturnOnContext(context, returnAddress, true);
        }
    }
    if (dr6 & 0x4) {  // Dr2 = Recv return: RAX=bytes received, saved buffer filled.
        PendingRecv* slot = FindPending(threadId);
        if (slot) {
            const int32_t received = static_cast<int32_t>(context->Rax);
            if (received > 0 && slot->buffer) {
                DumpRecord(2, slot->connection, slot->buffer, received);
            }
            ArmReturnOnContext(context, 0, false);
            slot->threadId = 0;
            slot->returnAddress = 0;
            slot->buffer = nullptr;
            slot->connection = 0;
        }
    }

    context->Dr6 = 0;
    context->EFlags |= 0x10000;  // Resume flag: don't immediately re-trigger.
    return EXCEPTION_CONTINUE_EXECUTION;
}

void Install(unsigned char* base, uintptr_t sendRva, uintptr_t recvRva) {
    gSend = reinterpret_cast<uintptr_t>(base) + sendRva;
    gRecv = reinterpret_cast<uintptr_t>(base) + recvRva;
    AddVectoredExceptionHandler(1, VectoredHandler);
    if (sendRva) {
        ArmAllThreads(0, gSend, true);
        LogLine("hwbp: armed Send at %p (Dr0)", reinterpret_cast<void*>(gSend));
    }
    if (recvRva) {
        ArmAllThreads(1, gRecv, true);
        LogLine("hwbp: armed Recv at %p (Dr1)", reinterpret_cast<void*>(gRecv));
    }
    LogLine("hwbp: capture armed via debug registers (no code modified)");
}

}  // namespace hwbp

int32_t __fastcall HookConnect(void* state, int32_t secure, const char* addr, uint32_t uaddr, int32_t port) {
    if (addr) {
        LogLine("ProtoSSLConnect secure=%d addr=%s port=%d", secure, addr, port);
    } else {
        LogLine("ProtoSSLConnect secure=%d uaddr=0x%08x port=%d", secure, uaddr, port);
    }
    return gOriginalConnect(state, secure, addr, uaddr, port);
}

// ---- probe --------------------------------------------------------------
bool ReadableExecutable(const void* address, size_t size) {
    MEMORY_BASIC_INFORMATION info = {};
    if (VirtualQuery(address, &info, sizeof(info)) != sizeof(info)) {
        return false;
    }
    if (info.State != MEM_COMMIT) {
        return false;
    }
    const DWORD protect = info.Protect & 0xff;
    const bool executable = protect == PAGE_EXECUTE || protect == PAGE_EXECUTE_READ ||
                            protect == PAGE_EXECUTE_READWRITE || protect == PAGE_EXECUTE_WRITECOPY;
    const uintptr_t end = reinterpret_cast<uintptr_t>(address) + size;
    const uintptr_t regionEnd = reinterpret_cast<uintptr_t>(info.BaseAddress) + info.RegionSize;
    return executable && !(info.Protect & (PAGE_GUARD | PAGE_NOACCESS)) && end <= regionEnd;
}

void ProbeAddress(unsigned char* base, const char* name, uintptr_t rva) {
    if (rva == 0) {
        LogLine("probe %s: no address configured", name);
        return;
    }
    unsigned char* target = base + rva;
    if (!ReadableExecutable(target, 24)) {
        LogLine("probe %s at rva=0x%llx (va=%p): NOT in committed executable memory", name,
                static_cast<unsigned long long>(rva), target);
        return;
    }
    char hex[128] = {};
    int written = 0;
    for (int i = 0; i < 24; ++i) {
        written += _snprintf_s(hex + written, sizeof(hex) - written, _TRUNCATE, "%02x ", target[i]);
    }
    // A real function entry starts with a recognizable prologue; anything else
    // means the address is wrong for this build.
    const bool prologue = target[0] == 0x48 || target[0] == 0x40 || target[0] == 0x55 ||
                          target[0] == 0x53 || target[0] == 0x57 || target[0] == 0x4c ||
                          target[0] == 0xe9 || target[0] == 0xff;
    LogLine("probe %s at rva=0x%llx (va=%p): bytes: %s| looks-like-entry=%s", name,
            static_cast<unsigned long long>(rva), target, hex, prologue ? "yes" : "no");
}

// Returns 0 ok, 1 create-failed, 2 enable-failed, 3 hardware exception.
int CreateAndEnable(void* target, void* detour, void** original, const char* name) {
    __try {
        MH_STATUS status = MH_CreateHook(target, detour, original);
        if (status != MH_OK) {
            LogLine("hook %s: MH_CreateHook failed: %s", name, MH_StatusToString(status));
            return 1;
        }
        LogLine("hook %s: created, enabling (threads suspend here)", name);
        status = MH_EnableHook(target);
        if (status != MH_OK) {
            LogLine("hook %s: MH_EnableHook failed: %s", name, MH_StatusToString(status));
            return 2;
        }
        return 0;
    } __except (EXCEPTION_EXECUTE_HANDLER) {
        // A fault during hooking is caught here instead of killing the game, so
        // the log identifies the culprit function precisely.
        LogLine("hook %s: EXCEPTION 0x%08lx during hook install", name,
                GetExceptionCode());
        return 3;
    }
}

bool InstallHook(unsigned char* base, const char* name, uintptr_t rva, void* detour, void** original) {
    if (rva == 0) {
        LogLine("hook %s: no address configured; skipped", name);
        return false;
    }
    void* target = base + rva;
    if (!ReadableExecutable(target, 16)) {
        LogLine("hook %s at rva=0x%llx: target not executable; refusing to hook", name,
                static_cast<unsigned long long>(rva));
        return false;
    }
    LogLine("hook %s: creating at va=%p", name, target);
    if (CreateAndEnable(target, detour, original, name) != 0) {
        return false;
    }
    LogLine("hook %s: installed at va=%p", name, target);
    return true;
}

bool IniEnabled(const wchar_t* key) {
    return _wtoi(ReadIni(L"Hooks", key, L"1").c_str()) != 0;
}

void InstallCapture(unsigned char* base) {
    const std::wstring dumpPath = gDumpDir + L"\\capture.acp2";
    gDump = OpenAppend(dumpPath);
    if (gDump == INVALID_HANDLE_VALUE) {
        LogLine("hook: could not open dump file; aborting to avoid a silent capture");
        return;
    }
    LogLine("hook: dumping decrypted frames to %ls", dumpPath.c_str());

    LogLine("hook: calling MH_Initialize");
    MH_STATUS init = MH_Initialize();
    if (init != MH_OK) {
        LogLine("hook: MH_Initialize failed: %s", MH_StatusToString(init));
        return;
    }
    LogLine("hook: MH_Initialize ok");
    // Individually toggleable so a crashing hook can be bisected from the ini.
    // Recv/Send carry the payload; Connect only annotates the host per stream.
    if (IniEnabled(L"Connect")) {
        InstallHook(base, "Connect", gConnectRva, reinterpret_cast<void*>(&HookConnect),
                    reinterpret_cast<void**>(&gOriginalConnect));
    }
    if (IniEnabled(L"Recv")) {
        InstallHook(base, "Recv", gRecvRva, reinterpret_cast<void*>(&HookRecv),
                    reinterpret_cast<void**>(&gOriginalRecv));
    }
    if (IniEnabled(L"Send")) {
        InstallHook(base, "Send", gSendRva, reinterpret_cast<void*>(&HookSend),
                    reinterpret_cast<void**>(&gOriginalSend));
    }
}

DWORD WINAPI Worker(void*) {
    // Open the log here, not in DllMain: the CRT file APIs can misbehave under
    // the loader lock, which silently produced no log on the first attempt.
    gDumpDir = ReadIni(L"Capture", L"DumpDir", L"");
    if (gDumpDir.empty()) {
        gDumpDir = gModuleDir;
    }
    const std::wstring logPath = gDumpDir + L"\\capturehook.log";
    gLog = OpenAppend(logPath);

    HMODULE game = GetModuleHandleW(nullptr);
    auto* base = reinterpret_cast<unsigned char*>(game);

    const std::wstring mode = ReadIni(L"Capture", L"Mode", L"probe");
    gConnectRva = ReadIniAddress(L"Connect");
    gSendRva = ReadIniAddress(L"Send");
    gRecvRva = ReadIniAddress(L"Recv");

    LogLine("capturehook worker: image base=%p mode=%ls", base,
            mode.empty() ? L"(none)" : mode.c_str());
    LogLine("configured rvas: connect=0x%llx send=0x%llx recv=0x%llx",
            static_cast<unsigned long long>(gConnectRva),
            static_cast<unsigned long long>(gSendRva),
            static_cast<unsigned long long>(gRecvRva));

    ProbeAddress(base, "Connect", gConnectRva);
    ProbeAddress(base, "Send", gSendRva);
    ProbeAddress(base, "Recv", gRecvRva);

    // Windows will not re-run DllMain for an already-loaded module, so a second
    // injection cannot flip probe -> hook. Instead the worker stays resident and
    // watches the ini: re-running the launch script (which rewrites Mode=hook)
    // arms the capture in this same loaded instance, with no game restart.
    // hwbp: debug-register capture, the only method that works against the
    // game's Denuvo-protected code (writing it faults with IN_PAGE_ERROR).
    // hook: legacy MinHook path, kept for unprotected targets.
    if (_wcsicmp(mode.c_str(), L"hwbp") == 0) {
        const std::wstring dumpPath = gDumpDir + L"\\capture.acp2";
        gDump = OpenAppend(dumpPath);
        if (gDump == INVALID_HANDLE_VALUE) {
            LogLine("hwbp: could not open dump file; aborting");
            return 0;
        }
        LogLine("hwbp: dumping decrypted frames to %ls", dumpPath.c_str());
        hwbp::Install(base, gSendRva, gRecvRva);
        // Re-arm periodically so threads created after injection (e.g. a network
        // pump spun up on entering Online Dynasty) also get the breakpoints.
        for (;;) {
            Sleep(3000);
            if (gSendRva) hwbp::ArmAllThreads(0, hwbp::gSend, true);
            if (gRecvRva) hwbp::ArmAllThreads(1, hwbp::gRecv, true);
        }
    }

    bool armed = _wcsicmp(mode.c_str(), L"hook") == 0;
    if (!armed) {
        LogLine("probe complete; watching ini. Set Mode=hwbp (re-run with -Mode "
                "hwbp) to arm capture in this session.");
    }
    for (;;) {
        if (armed) {
            InstallCapture(base);
            LogLine("hook: capture armed; recording decrypted frames");
            return 0;
        }
        Sleep(1000);
        const std::wstring current = ReadIni(L"Capture", L"Mode", L"probe");
        armed = _wcsicmp(current.c_str(), L"hook") == 0;
        if (_wcsicmp(current.c_str(), L"hwbp") == 0) {
            const std::wstring dumpPath = gDumpDir + L"\\capture.acp2";
            gDump = OpenAppend(dumpPath);
            LogLine("hwbp: dumping decrypted frames to %ls", dumpPath.c_str());
            hwbp::Install(base, gSendRva, gRecvRva);
            return 0;
        }
    }
}

}  // namespace

BOOL WINAPI DllMain(HINSTANCE instance, DWORD reason, LPVOID) {
    if (reason == DLL_PROCESS_ATTACH) {
        DisableThreadLibraryCalls(instance);
        InitializeCriticalSection(&gLock);
        gModuleDir = DirectoryOf(instance);
        // Nothing else here: DllMain holds the loader lock, and CRT file APIs
        // can fail or hang under it. The worker opens files and does all work
        // once the lock is released.
        CreateThread(nullptr, 0, Worker, nullptr, 0, nullptr);
    }
    return TRUE;
}
