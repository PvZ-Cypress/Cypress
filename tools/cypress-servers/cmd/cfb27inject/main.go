// cfb27inject loads the Cypress bridge into an already-running CollegeFB27.
//
// A capture cannot use the dinput8 proxy path: EA's anticheat inspects the game
// directory at launch and refuses to start when the proxy is present. The
// working sequence is to launch the game clean, let it reach the press-start
// screen, stop the anticheat, and only then load the bridge into the live
// process. That window is short, so this tool does one thing and reports
// precisely why it failed when it does.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func main() {
	var processName string
	var dllPath string
	var timeout time.Duration
	flag.StringVar(&processName, "process", "CollegeFB27.exe", "target process image name")
	flag.StringVar(&dllPath, "dll", "", "path to the bridge DLL to inject")
	flag.DurationVar(&timeout, "wait", 0, "wait up to this long for the process to appear")
	flag.Parse()

	if strings.TrimSpace(dllPath) == "" {
		fmt.Fprintln(os.Stderr, "fatal: -dll is required")
		os.Exit(1)
	}
	absolute, err := filepath.Abs(dllPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: resolve DLL path: %v\n", err)
		os.Exit(1)
	}
	if info, statErr := os.Stat(absolute); statErr != nil || !info.Mode().IsRegular() {
		fmt.Fprintf(os.Stderr, "fatal: DLL not found: %s\n", absolute)
		os.Exit(1)
	}

	pid, err := findProcess(processName, timeout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("target %s pid=%d\n", processName, pid)

	if err := inject(pid, absolute); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("injected %s\n", absolute)
}

func findProcess(name string, timeout time.Duration) (uint32, error) {
	deadline := time.Now().Add(timeout)
	for {
		pid, err := snapshotFind(name)
		if err == nil {
			return pid, nil
		}
		if time.Now().After(deadline) {
			return 0, fmt.Errorf("process %s is not running", name)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func snapshotFind(name string) (uint32, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0, err
	}
	defer windows.CloseHandle(snapshot)

	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	if err := windows.Process32First(snapshot, &entry); err != nil {
		return 0, err
	}
	for {
		current := windows.UTF16ToString(entry.ExeFile[:])
		if strings.EqualFold(current, name) {
			return entry.ProcessID, nil
		}
		if err := windows.Process32Next(snapshot, &entry); err != nil {
			return 0, fmt.Errorf("process %s not found", name)
		}
	}
}

// x/sys/windows does not export these, so bind them directly.
var (
	kernel32DLL          = windows.NewLazySystemDLL("kernel32.dll")
	procVirtualAllocEx   = kernel32DLL.NewProc("VirtualAllocEx")
	procVirtualFreeEx    = kernel32DLL.NewProc("VirtualFreeEx")
	procCreateRemoteThrd = kernel32DLL.NewProc("CreateRemoteThread")
	procGetExitCodeThrd  = kernel32DLL.NewProc("GetExitCodeThread")
)

func virtualAllocEx(process windows.Handle, size uintptr) (uintptr, error) {
	address, _, err := procVirtualAllocEx.Call(
		uintptr(process), 0, size,
		uintptr(windows.MEM_COMMIT|windows.MEM_RESERVE), uintptr(windows.PAGE_READWRITE))
	if address == 0 {
		return 0, err
	}
	return address, nil
}

func virtualFreeEx(process windows.Handle, address uintptr) {
	const memRelease = 0x8000
	_, _, _ = procVirtualFreeEx.Call(uintptr(process), address, 0, memRelease)
}

func createRemoteThread(process windows.Handle, start, parameter uintptr) (windows.Handle, error) {
	handle, _, err := procCreateRemoteThrd.Call(
		uintptr(process), 0, 0, start, parameter, 0, 0)
	if handle == 0 {
		return 0, err
	}
	return windows.Handle(handle), nil
}

func getExitCodeThread(thread windows.Handle) (uint32, error) {
	var code uint32
	result, _, err := procGetExitCodeThrd.Call(uintptr(thread), uintptr(unsafe.Pointer(&code)))
	if result == 0 {
		return 0, err
	}
	return code, nil
}

func inject(pid uint32, dllPath string) error {
	const access = windows.PROCESS_CREATE_THREAD |
		windows.PROCESS_QUERY_INFORMATION |
		windows.PROCESS_VM_OPERATION |
		windows.PROCESS_VM_WRITE |
		windows.PROCESS_VM_READ

	process, err := windows.OpenProcess(access, false, pid)
	if err != nil {
		return fmt.Errorf("open process %d (run this elevated): %w", pid, err)
	}
	defer windows.CloseHandle(process)

	encoded, err := windows.UTF16FromString(dllPath)
	if err != nil {
		return err
	}
	size := uintptr(len(encoded) * 2)

	remote, err := virtualAllocEx(process, size)
	if err != nil {
		return fmt.Errorf("allocate in target: %w", err)
	}
	defer virtualFreeEx(process, remote)

	var written uintptr
	if err := windows.WriteProcessMemory(
		process, remote, (*byte)(unsafe.Pointer(&encoded[0])), size, &written); err != nil {
		return fmt.Errorf("write path into target: %w", err)
	}

	kernel32, err := windows.LoadLibrary("kernel32.dll")
	if err != nil {
		return err
	}
	loadLibrary, err := windows.GetProcAddress(kernel32, "LoadLibraryW")
	if err != nil {
		return err
	}

	thread, err := createRemoteThread(process, loadLibrary, remote)
	if err != nil {
		return fmt.Errorf("start loader thread: %w", err)
	}
	defer windows.CloseHandle(thread)

	state, err := windows.WaitForSingleObject(thread, 15000)
	if err != nil {
		return fmt.Errorf("wait for loader thread: %w", err)
	}
	if state == uint32(windows.WAIT_TIMEOUT) {
		return fmt.Errorf("loader thread did not finish within 15s")
	}
	exitCode, err := getExitCodeThread(thread)
	if err != nil {
		return err
	}
	// LoadLibraryW returns the module handle; zero means the target refused it.
	if exitCode == 0 {
		return fmt.Errorf("LoadLibraryW returned NULL in the target; the DLL was not loaded")
	}
	return nil
}
