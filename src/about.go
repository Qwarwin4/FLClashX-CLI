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

func hwidState(s settings) string {
	switch s.HWID {
	case "none":
		return T("не передаётся", "not sent")
	case "fake":
		return T("подменный, свой для каждой панели", "a stand-in, different for each panel")
	}
	if id := hwid(); id != "" {
		return id[:12] + "…"
	}
	return T("нет machine-id", "no machine-id")
}

func about() string {
	s, _ := readSettings()

	hw := hwidState(s)
	d := deviceInfo()
	hidden := T(" (панели не передаётся)", " (not sent to panels)")
	sent := s.HWID == "fake" || s.HWID == "real" && hwid() != ""
	sys, dev := d.ver+", "+runtime.GOARCH, d.model
	if s.HideOS || !sent {
		sys += hidden
	}
	if s.HideDevice || !sent {
		dev += hidden
	}
	ua := agentFor(s)
	if s.HideUA {
		ua += T(" (настоящий скрыт)", " (the real one is hidden)")
	}
	upd := T("выкл", "off")
	if s.Interval > 0 {
		upd = fmt.Sprintf(T("каждые %d мин", "every %d min"), s.Interval)
	}

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
		{"User-Agent", ua},
		{"HWID", hw},
		{T("система", "system"), sys},
		{T("устройство", "device"), dev},
		{T("подписка", "subscription"), upd},
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
