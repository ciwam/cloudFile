package main

import (
	"runtime"
	"sync"

	"github.com/ebitengine/purego"
)

// The ObjC bits the app needs over what mygo's Shell.PreviewItem offers:
// asking whether the Quick Look panel is up, and hiding it, so the arrow
// keys can move the preview and Space can close it, as Finder's space bar
// does.
var (
	qlOnce         sync.Once
	qlOK           bool
	qlObjCGetClass func(name string) uintptr
	qlMsgSend      func(self, op uintptr, args ...uintptr) uintptr
	qlSelShared    uintptr
	qlSelVisible   uintptr
	qlSelOrderOut  uintptr
)

func qlInit() {
	qlOnce.Do(func() {
		if runtime.GOOS != "darwin" {
			return
		}
		libobjc, err := purego.Dlopen("/usr/lib/libobjc.A.dylib", purego.RTLD_GLOBAL|purego.RTLD_NOW)
		if err != nil {
			return
		}
		purego.RegisterLibFunc(&qlObjCGetClass, libobjc, "objc_getClass")
		addr, err := purego.Dlsym(libobjc, "objc_msgSend")
		if err != nil {
			return
		}
		purego.RegisterFunc(&qlMsgSend, addr)
		selAddr, err := purego.Dlsym(libobjc, "sel_registerName")
		if err != nil {
			return
		}
		var selRegister func(name string) uintptr
		purego.RegisterFunc(&selRegister, selAddr)
		qlSelShared = selRegister("sharedPreviewPanel")
		qlSelVisible = selRegister("isVisible")
		qlSelOrderOut = selRegister("orderOut:")
		qlOK = qlSelShared != 0 && qlSelVisible != 0 && qlSelOrderOut != 0
	})
}

// qlPanel returns the shared QLPreviewPanel, 0 when unavailable.
func qlPanel() uintptr {
	qlInit()
	if !qlOK {
		return 0
	}
	cls := qlObjCGetClass("QLPreviewPanel")
	if cls == 0 {
		return 0
	}
	return qlMsgSend(cls, qlSelShared)
}

// qlPanelVisible reports whether the Quick Look panel is on screen.
func qlPanelVisible() bool {
	p := qlPanel()
	if p == 0 {
		return false
	}
	return qlMsgSend(p, qlSelVisible) != 0
}

// qlHidePanel closes the Quick Look panel, as pressing Space again in
// Finder does.
func qlHidePanel() {
	if p := qlPanel(); p != 0 {
		qlMsgSend(p, qlSelOrderOut)
	}
}
