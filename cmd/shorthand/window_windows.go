//go:build windows

package main

import (
	"image"
	"syscall"
	"unsafe"
)

// Оформление окна средствами Windows, которых нет в Fyne 2.4:
//   - тёмный заголовок (DwmSetWindowAttribute). Fyne красит заголовок по
//     настройке Windows один раз, при создании окна, поэтому после появления
//     окна он перекрашивается ещё раз;
//   - значки окна точного размера (WM_SETICON). Fyne отдаёт Windows одну
//     картинку, и та уменьшает её до 16–24 px сама — получается каша.

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	dwmapi   = syscall.NewLazyDLL("dwmapi.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	procFindWindow          = user32.NewProc("FindWindowW")
	procIsVisible           = user32.NewProc("IsWindowVisible")
	procSetWindowPos        = user32.NewProc("SetWindowPos")
	procGetWindowTPID       = user32.NewProc("GetWindowThreadProcessId")
	procSendMessage         = user32.NewProc("SendMessageW")
	procGetSystemMetrics    = user32.NewProc("GetSystemMetrics")
	procGetSystemMetricsDpi = user32.NewProc("GetSystemMetricsForDpi")
	procGetDpiForWindow     = user32.NewProc("GetDpiForWindow")
	procCreateIconIndirect  = user32.NewProc("CreateIconIndirect")
	procGetDC               = user32.NewProc("GetDC")
	procReleaseDC           = user32.NewProc("ReleaseDC")
	procCreateDIBSection    = gdi32.NewProc("CreateDIBSection")
	procCreateBitmap        = gdi32.NewProc("CreateBitmap")
	procDeleteObject        = gdi32.NewProc("DeleteObject")
	procDwmSetAttr          = dwmapi.NewProc("DwmSetWindowAttribute")
	procDwmGetAttr          = dwmapi.NewProc("DwmGetWindowAttribute")
	procGetCurrentPID       = kernel32.NewProc("GetCurrentProcessId")
)

const (
	dwmaUseImmersiveDarkMode       = 20 // Windows 10 20H1 и новее, Windows 11
	dwmaUseImmersiveDarkModeBefore = 19 // Windows 10 1809–1909
	swpNoSize                      = 0x0001
	swpNoMove                      = 0x0002
	swpNoZOrder                    = 0x0004
	swpNoActivate                  = 0x0010
	swpFrameChanged                = 0x0020
	wmSetIcon                      = 0x0080
	iconSmall                      = 0
	iconBig                        = 1
	smCxIcon                       = 11
	smCxSmIcon                     = 49
)

var iconsDone bool

// decorateWindow оформляет окно этой программы с заголовком title.
// Возвращает true, когда всё сделано; false — окно ещё не готово или Fyne
// пока не закончил его создание: вызов надо повторить.
func decorateWindow(title string) bool {
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
		return false // Fyne ещё создаёт окно
	}
	if !iconsDone {
		setIcons(hwnd)
		iconsDone = true
	}
	return darkTitleBar(hwnd)
}

func darkTitleBar(hwnd uintptr) bool {
	if procDwmSetAttr.Find() != nil || procDwmGetAttr.Find() != nil {
		return true // нет DWM — делать нечего
	}
	var cur int32
	if r, _, _ := procDwmGetAttr.Call(hwnd, dwmaUseImmersiveDarkMode, uintptr(unsafe.Pointer(&cur)), 4); r == 0 && cur != 0 {
		return true
	}
	on := int32(1)
	if r, _, _ := procDwmSetAttr.Call(hwnd, dwmaUseImmersiveDarkMode, uintptr(unsafe.Pointer(&on)), 4); r != 0 {
		procDwmSetAttr.Call(hwnd, dwmaUseImmersiveDarkModeBefore, uintptr(unsafe.Pointer(&on)), 4)
	}
	procSetWindowPos.Call(hwnd, 0, 0, 0, 0, 0, swpNoSize|swpNoMove|swpNoZOrder|swpNoActivate|swpFrameChanged)
	return false // проверим на следующем шаге, что значение удержалось
}

// iconSize — размер значка (SM_CXSMICON или SM_CXICON) с учётом масштаба окна.
func iconSize(hwnd, metric uintptr) int {
	if procGetSystemMetricsDpi.Find() == nil && procGetDpiForWindow.Find() == nil {
		if dpi, _, _ := procGetDpiForWindow.Call(hwnd); dpi != 0 {
			if v, _, _ := procGetSystemMetricsDpi.Call(metric, dpi); v != 0 {
				return int(v)
			}
		}
	}
	if v, _, _ := procGetSystemMetrics.Call(metric); v != 0 {
		return int(v)
	}
	return 16
}

func setIcons(hwnd uintptr) {
	for _, k := range []struct{ which, metric uintptr }{{iconSmall, smCxSmIcon}, {iconBig, smCxIcon}} {
		if h := makeIcon(renderIcon(iconSize(hwnd, k.metric))); h != 0 {
			procSendMessage.Call(hwnd, wmSetIcon, k.which, h)
		}
	}
}

type bitmapInfoHeader struct {
	Size          uint32
	Width, Height int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

type iconInfo struct {
	FIcon              int32
	XHotspot, YHotspot uint32
	HbmMask, HbmColor  uintptr
}

// makeIcon создаёт HICON из картинки с прозрачностью (32 бита, BGRA).
func makeIcon(img *image.NRGBA) uintptr {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	bi := bitmapInfoHeader{Size: 40, Width: int32(w), Height: -int32(h), Planes: 1, BitCount: 32}
	hdc, _, _ := procGetDC.Call(0)
	var bits unsafe.Pointer
	colorBmp, _, _ := procCreateDIBSection.Call(hdc, uintptr(unsafe.Pointer(&bi)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	procReleaseDC.Call(0, hdc)
	if colorBmp == 0 || bits == nil {
		return 0
	}
	defer procDeleteObject.Call(colorBmp)
	dst := unsafe.Slice((*byte)(bits), w*h*4)
	for i := 0; i < w*h; i++ {
		p := img.Pix[4*i : 4*i+4]
		dst[4*i], dst[4*i+1], dst[4*i+2], dst[4*i+3] = p[2], p[1], p[0], p[3]
	}
	maskBits := make([]byte, (w+15)/16*2*h) // нулевая маска: прозрачность берётся из альфа-канала
	mask, _, _ := procCreateBitmap.Call(uintptr(w), uintptr(h), 1, 1, uintptr(unsafe.Pointer(&maskBits[0])))
	if mask == 0 {
		return 0
	}
	defer procDeleteObject.Call(mask)
	ii := iconInfo{FIcon: 1, HbmMask: mask, HbmColor: colorBmp}
	icon, _, _ := procCreateIconIndirect.Call(uintptr(unsafe.Pointer(&ii)))
	return icon
}
