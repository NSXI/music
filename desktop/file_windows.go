package main

import "golang.org/x/sys/windows"

func moveNew(source, dest string) error {
	src, e := windows.UTF16PtrFromString(source)
	if e != nil {
		return e
	}
	dst, e := windows.UTF16PtrFromString(dest)
	if e != nil {
		return e
	}
	return windows.MoveFileEx(src, dst, windows.MOVEFILE_WRITE_THROUGH)
}
