package main

import (
	"runtime"

	"github.com/ebitengine/purego"
)

// preloadQuickLookUI loads the framework that registers QLPreviewPanel on
// current macOS. mygo's Shell.PreviewItem opens QuickLook.framework only,
// whose ObjC classes have moved into QuickLookUI (under Quartz) by macOS
// 14 — without this, PreviewItem reports ErrUnsupported. Harmless when the
// class is already visible or the framework is missing.
func preloadQuickLookUI() error {
	if runtime.GOOS != "darwin" {
		return nil
	}
	_, err := purego.Dlopen("/System/Library/Frameworks/Quartz.framework/Frameworks/QuickLookUI.framework/QuickLookUI", purego.RTLD_GLOBAL|purego.RTLD_NOW)
	return err
}
