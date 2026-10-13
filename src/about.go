package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"

	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/constant/features"
)

func coreVersion() string {
	v := C.Version
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, d := range bi.Deps {
			if d.Path != "github.com/metacubex/mihomo" {
				continue
			}
			if d.Replace != nil && d.Replace.Version != "" {
				d = d.Replace
			}
			if d.Version != "" && d.Version != "(devel)" {
				v = d.Version
			}
		}
	}
	return strings.TrimPrefix(v, "v")
}

func userAgent() string {
	return fmt.Sprintf("FLClashX CLI %s/mihomo %s", strings.TrimPrefix(version, "v"), coreVersion())
}

func tilde(p string) string {
	if h, err := os.UserHomeDir(); err == nil && h != "/" {
		if r, err := filepath.Rel(h, p); err == nil && !strings.HasPrefix(r, "..") {
			return filepath.Join("~", r)
		}
	}
	return p
}

func about() string {
	s, _ := readSettings()

	id := hwid()
	hw := T("не передаётся", "not sent")
	switch {
	case id == "":
		hw = T("нет machine-id", "no machine-id")
	case s.HWID:
		hw = id[:12] + "…"
	}
	d := deviceInfo()

	fl := T("эмодзи", "emoji")
	if flagLetters(s.Flags) {
		fl = T("буквы", "letters")
	}
	if s.Flags == "auto" {
		fl += T(" (авто)", " (auto)")
	}

	st := "system"
	if features.WithGVisor {
		st = "mixed, system, gvisor"
	}

	rows := [][2]string{
		{T("версия", "version"), strings.TrimPrefix(version, "v")},
		{"mihomo", coreVersion()},
		{"User-Agent", userAgent()},
		{"HWID", hw},
		{T("система", "system"), d.ver + ", " + runtime.GOARCH},
		{T("устройство", "device"), d.model},
		{T("флаги", "flags"), fl},
		{T("стеки TUN", "TUN stacks"), st},
		{T("настройки", "settings"), tilde(settingsPath())},
		{T("профили", "profiles"), tilde(profDir())},
		{T("лог", "log"), tilde(logPath())},
		{T("исходники", "source"), "https://github.com/" + repo},
	}
	w := 0
	for _, r := range rows {
		w = max(w, cells(r[0]))
	}
	var b strings.Builder
	b.WriteString(T("\nО клиенте:\n", "\nAbout this client:\n"))
	for _, r := range rows {
		b.WriteString("  " + r[0] + strings.Repeat(" ", w-cells(r[0])+3) + show(r[1]) + "\n")
	}
	return b.String()
}
