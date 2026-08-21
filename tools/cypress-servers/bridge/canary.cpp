// canary.dll — the smallest possible injected module.
//
// It writes one marker file from DllMain and does nothing else: no threads, no
// memory scanning, no patching. It exists to answer a single question about the
// capture crash — does the game die because a new module attached at all
// (anticheat integrity guard), or because of something the real bridge does
// after attaching?
//
//   canary survives  -> injection is safe; the fault is in the bridge's workers.
//   canary crashes   -> the process rejects any injected module; a different
//                        capture approach is required.
#define WIN32_LEAN_AND_MEAN
#include <windows.h>

#include <string>

static void WriteMarker() {
    wchar_t appData[MAX_PATH] = {};
    DWORD length = GetEnvironmentVariableW(L"APPDATA", appData, MAX_PATH);
    if (length == 0 || length >= MAX_PATH) {
        return;
    }
    std::wstring path(appData);
    path += L"\\Cypress\\CFB27\\Private\\canary.txt";
    HANDLE file = CreateFileW(
        path.c_str(), FILE_APPEND_DATA, FILE_SHARE_READ | FILE_SHARE_WRITE,
        nullptr, OPEN_ALWAYS, FILE_ATTRIBUTE_NORMAL, nullptr);
    if (file == INVALID_HANDLE_VALUE) {
        return;
    }
    SYSTEMTIME now = {};
    GetLocalTime(&now);
    char line[128] = {};
    int n = wsprintfA(
        line, "canary attached pid=%lu at %02u:%02u:%02u\r\n",
        GetCurrentProcessId(), now.wHour, now.wMinute, now.wSecond);
    DWORD written = 0;
    WriteFile(file, line, static_cast<DWORD>(n), &written, nullptr);
    CloseHandle(file);
}

BOOL WINAPI DllMain(HINSTANCE instance, DWORD reason, LPVOID) {
    if (reason == DLL_PROCESS_ATTACH) {
        DisableThreadLibraryCalls(instance);
        WriteMarker();
    }
    return TRUE;
}
