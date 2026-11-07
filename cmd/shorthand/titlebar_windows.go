//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

var (
	user32            = syscall.NewLazyDLL("user32.dll")
	dwmapi            = syscall.NewLazyDLL("dwmapi.dll")
	procFindWindow    = user32.NewProc("FindWindowW")
	procIsVisible     = user32.NewProc("IsWindowVisible")
	procSetWindowPos  = user32.NewProc("SetWindowPos")
	procGetWindowTPID = user32.NewProc("GetWindowThreadProcessId")
	procDwmSetAttr    = dwmapi.NewProc("DwmSetWindowAttribute")
	procDwmGetAttr    = dwmapi.NewProc("DwmGetWindowAttribute")
	procGetCurrentPID = syscall.NewLazyDLL("kernel32.dll").NewProc("GetCurrentProcessId")
)

const (
	dwmaUseImmersiveDarkMode       = 20 // Windows 10 20H1 и новее, Windows 11
	dwmaUseImmersiveDarkModeBefore = 19 // Windows 10 1809–1909
	swpNoSize                      = 0x0001
	swpNoMove                      = 0x0002
	swpNoZOrder                    = 0x0004
	swpNoActivate                  = 0x0010
	swpFrameChanged                = 0x0020
)

// darkTitleBar включает тёмный заголовок окна этой программы с указанным
// заголовком. Возвращает true, когда тёмный режим включён (или включать
// нечего), false — если окно ещё не готово или Fyne пока не закончил его
// создание: тогда вызов надо повторить.
func darkTitleBar(title string) bool {
	if procDwmSetAttr.Find() != nil || procDwmGetAttr.Find() != nil {
		return true // нет DWM — делать нечего
	}
	t, err := syscall.UTF16PtrFromString(title)
	if err != nil {
		return true
	}
	hwnd, _, _ := procFindWindow.Call(0, uintptr(unsafe.Pointer(t)))
	if hwnd == 0 {
		return false
	}
	var pid uint32
	procGetWindowTPID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if self, _, _ := procGetCurrentPID.Call(); uint32(self) != pid {
		return false // окно с таким заголовком — чужое
	}
	if vis, _, _ := procIsVisible.Call(hwnd); vis == 0 {
		return false // Fyne ещё создаёт окно и сам выставит системный режим
	}
	var cur int32
	if r, _, _ := procDwmGetAttr.Call(hwnd, dwmaUseImmersiveDarkMode, uintptr(unsafe.Pointer(&cur)), 4); r == 0 && cur != 0 {
		return true
	}
	on := int32(1)
	if r, _, _ := procDwmSetAttr.Call(hwnd, dwmaUseImmersiveDarkMode, uintptr(unsafe.Pointer(&on)), 4); r != 0 {
		procDwmSetAttr.Call(hwnd, dwmaUseImmersiveDarkModeBefore, uintptr(unsafe.Pointer(&on)), 4)
	}
	// перерисовать рамку окна
	procSetWindowPos.Call(hwnd, 0, 0, 0, 0, 0, swpNoSize|swpNoMove|swpNoZOrder|swpNoActivate|swpFrameChanged)
	return false // проверим на следующем шаге, что значение удержалось
}
