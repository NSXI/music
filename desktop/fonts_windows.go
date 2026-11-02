package main

import (
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"sort"
	"strings"
	"syscall"
	"unsafe"
)

func (s *Studio) GetFonts() []string {
	seen := map[string]bool{"Segoe UI": true, "Tahoma": true, "Arial": true}
	// GDI returns family names the browser can use, including fonts installed
	// only for this Windows user; registry values may instead be face names.
	type logFont struct {
		Height, Width, Escapement, Orientation, Weight                                              int32
		Italic, Underline, StrikeOut, CharSet, OutPrecision, ClipPrecision, Quality, PitchAndFamily byte
		FaceName                                                                                    [32]uint16
	}
	gdi := windows.NewLazySystemDLL("gdi32.dll")
	dc, _, _ := gdi.NewProc("CreateCompatibleDC").Call(0)
	if dc != 0 {
		lf := logFont{CharSet: 1}
		callback := syscall.NewCallback(func(font, metric, kind, data uintptr) uintptr {
			family := windows.UTF16ToString((*logFont)(unsafe.Pointer(font)).FaceName[:])
			if family != "" && !strings.HasPrefix(family, "@") {
				seen[family] = true
			}
			return 1
		})
		gdi.NewProc("EnumFontFamiliesExW").Call(dc, uintptr(unsafe.Pointer(&lf)), callback, 0, 0)
		gdi.NewProc("DeleteDC").Call(dc)
	}
	if len(seen) > 3 {
		out := make([]string, 0, len(seen))
		for family := range seen {
			out = append(out, family)
		}
		sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i]) < strings.ToLower(out[j]) })
		return out
	}
	for _, root := range []registry.Key{registry.LOCAL_MACHINE, registry.CURRENT_USER} {
		k, e := registry.OpenKey(root, `SOFTWARE\Microsoft\Windows NT\CurrentVersion\Fonts`, registry.QUERY_VALUE)
		if e != nil {
			continue
		}
		names, _ := k.ReadValueNames(-1)
		k.Close()
		for _, name := range names {
			if i := strings.LastIndex(name, " ("); i >= 0 {
				name = name[:i]
			}
			for _, family := range strings.Split(name, " & ") {
				family = strings.TrimSpace(family)
				if family != "" && !strings.HasPrefix(family, "@") {
					seen[family] = true
				}
			}
		}
	}
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i]) < strings.ToLower(out[j]) })
	return out
}
