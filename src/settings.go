package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

const defRu = `# Настройки FLClashX CLI. То же самое меняется в flc settings.

# весь трафик системы через VPN (TUN). false — только локальный прокси
tun: true
# system | gvisor | mixed
tun-stack: mixed
# http/socks прокси, слушает только 127.0.0.1
mixed-port: 7890
# rule | global | direct
mode: rule
# silent | error | warning | info | debug
log-level: info
# флаги стран в именах серверов: auto | emoji | text (text — код страны буквами)
flags: auto
# запускать VPN при входе в систему
autostart: false
# приложения, которые идут мимо VPN-туннеля, через запятую: discord, steam
apps-direct: ""
# чинить то, что ломают блокировки: QUIC, утечки IPv6, YouTube мимо VPN
anti-block: true

# обновлять активную подписку каждые N минут, пока VPN запущен. 0 — не обновлять
update-interval: 0
# при обновлении подписки оставлять свои правила маршрутизации
keep-rules: false
# раз в сутки проверять, не вышла ли новая версия FLClashX CLI
check-updates: true

# HWID для панели: real — хеш machine-id, fake — подменный, свой для каждой панели, none — не передавать
hide-hwid: real
# слать панели нейтральный User-Agent clash.meta вместо FLClashX CLI
hide-user-agent: false
# не передавать панели модель компьютера
hide-device: false
# не передавать панели систему и её версию
hide-os: false
# не определять, какое приложение открыло соединение (и не писать это в лог)
hide-processes: false
# стирать лог ядра после flc stop
wipe-log: false
`

const defEn = `# FLClashX CLI settings. flc settings changes the same things.

# route all system traffic through the VPN (TUN). false = local proxy only
tun: true
# system | gvisor | mixed
tun-stack: mixed
# http/socks proxy, listens on 127.0.0.1 only
mixed-port: 7890
# rule | global | direct
mode: rule
# silent | error | warning | info | debug
log-level: info
# country flags in server names: auto | emoji | text (text = country code in letters)
flags: auto
# start the VPN when you log in
autostart: false
# apps that go around the VPN tunnel, comma separated: discord, steam
apps-direct: ""
# work around what blocking breaks: QUIC, IPv6 leaks, YouTube going around the VPN
anti-block: true

# refresh the active subscription every N minutes while the VPN runs. 0 = never
update-interval: 0
# keep your own routing rules when a subscription is refreshed
keep-rules: false
# check once a day whether a new FLClashX CLI version is out
check-updates: true

# HWID for the panel: real = hash of machine-id, fake = a stand-in, different per panel, none = don't send
hide-hwid: real
# send the panel a neutral User-Agent, clash.meta, instead of FLClashX CLI
hide-user-agent: false
# don't tell the panel the computer model
hide-device: false
# don't tell the panel the OS and its version
hide-os: false
# don't track which app opened a connection (and keep it out of the log)
hide-processes: false
# wipe the core log after flc stop
wipe-log: false
`

func settingsPath() string { return filepath.Join(dir.cfg, "settings.yaml") }

type settings struct {
	Tun        bool   `yaml:"tun"`
	Stack      string `yaml:"tun-stack"`
	MixedPort  int    `yaml:"mixed-port"`
	Mode       string `yaml:"mode"`
	LogLevel   string `yaml:"log-level"`
	Flags      string `yaml:"flags"`
	Interval   int    `yaml:"update-interval"`
	KeepRules  bool   `yaml:"keep-rules"`
	AutoCheck  bool   `yaml:"check-updates"`
	HWID       string `yaml:"hide-hwid"`
	HideUA     bool   `yaml:"hide-user-agent"`
	HideDevice bool   `yaml:"hide-device"`
	HideOS     bool   `yaml:"hide-os"`
	HideProc   bool   `yaml:"hide-processes"`
	WipeLog    bool   `yaml:"wipe-log"`
	Autostart  bool   `yaml:"autostart"`
	AppsDirect string `yaml:"apps-direct"`
	AntiBlock  bool   `yaml:"anti-block"`
}

func loadSettings() (settings, error) {
	s, err := readSettings()
	if errors.Is(err, fs.ErrNotExist) {
		return s, writeFile(settingsPath(), []byte(T(defRu, defEn)))
	}
	return s, err
}

func readSettings() (settings, error) {
	var s settings
	if err := yaml.Unmarshal([]byte(T(defRu, defEn)), &s); err != nil {
		return s, err
	}
	p := settingsPath()
	b, err := os.ReadFile(p)
	if err != nil {
		return s, err
	}
	if err := yaml.Unmarshal(b, &s); err != nil {
		return s, fmt.Errorf("%s: %w", p, err)
	}
	var old struct {
		Send *bool `yaml:"send-hwid"`
	}
	if yaml.Unmarshal(b, &old) == nil && old.Send != nil && !*old.Send && !strings.Contains(string(b), "\nhide-hwid:") {
		s.HWID = "none"
	}

	if s.Mode == "rules" {
		s.Mode = "rule"
	}
	if k := s.bad(); k != "" {
		return s, fmt.Errorf(T("%s: неверное значение %s", "%s: bad value for %s"), p, k)
	}
	return s, nil
}

func (s settings) bad() string {
	switch {
	case !slices.Contains(stacks, s.Stack):
		return "tun-stack"
	case s.MixedPort < 1 || s.MixedPort > 65535:
		return "mixed-port"
	case !slices.Contains(modes, s.Mode):
		return "mode"
	case !slices.Contains(levels, s.LogLevel):
		return "log-level"
	case !slices.Contains(flagModes, s.Flags):
		return "flags"
	case s.Interval != 0 && (s.Interval < minInterval || s.Interval > maxInterval):
		return "update-interval"
	case !slices.Contains(hwidModes, s.HWID):
		return "hide-hwid"
	case appsBad(s.AppsDirect) != "":
		return "apps-direct"
	}
	return ""
}

var (
	stacks    = []string{"mixed", "system", "gvisor"}
	modes     = []string{"rule", "global", "direct"}
	levels    = []string{"silent", "error", "warning", "info", "debug"}
	flagModes = []string{"auto", "emoji", "text"}
	hwidModes = []string{"real", "fake", "none"}
)

const (
	minInterval = 5
	maxInterval = 10080
)

func note(k string) []string {
	def := strings.Split(T(defRu, defEn), "\n")
	for i, l := range def {
		if i > 0 && strings.HasPrefix(l, k+":") && strings.HasPrefix(def[i-1], "#") {
			return []string{def[i-1]}
		}
	}
	return nil
}

var oldLines = []string{
	"# User-Agent при скачивании подписки", "# User-Agent used to fetch subscriptions",
	"# слать x-hwid (sha256 от machine-id) — нужно панелям с лимитом устройств",
	"# send x-hwid (sha256 of machine-id), needed by panels that limit devices",
	"# Настройки flc. Применяются при следующем flc start.", "# flc settings. Applied on the next flc start.",
	"# приложения, которые всегда идут через VPN, через запятую: discord, telegram-desktop",
	"# apps that always go through the VPN, comma separated: discord, telegram-desktop",
	"# приложения, которые всегда идут мимо VPN", "# apps that always bypass the VPN",
}

func saveSettings(s settings) error {
	if k := s.bad(); k != "" {
		return fmt.Errorf(T("неверное значение %s", "bad value for %s"), k)
	}
	b, err := os.ReadFile(settingsPath())
	if errors.Is(err, fs.ErrNotExist) {
		b, err = []byte(T(defRu, defEn)), nil
	}
	if err != nil {
		return err
	}

	kv := []struct {
		k string
		v any
	}{
		{"tun", s.Tun}, {"tun-stack", s.Stack}, {"mixed-port", s.MixedPort}, {"mode", s.Mode},
		{"log-level", s.LogLevel}, {"flags", s.Flags}, {"update-interval", s.Interval},
		{"keep-rules", s.KeepRules}, {"check-updates", s.AutoCheck}, {"hide-hwid", s.HWID},
		{"hide-user-agent", s.HideUA}, {"hide-device", s.HideDevice}, {"hide-os", s.HideOS},
		{"hide-processes", s.HideProc}, {"wipe-log", s.WipeLog}, {"autostart", s.Autostart},
		{"apps-direct", s.AppsDirect}, {"anti-block", s.AntiBlock},
	}
	var lines []string
	for _, l := range strings.Split(strings.TrimRight(string(b), "\n"), "\n") {
		if !strings.HasPrefix(l, "user-agent:") && !strings.HasPrefix(l, "send-hwid:") &&
			!strings.HasPrefix(l, "apps-vpn:") && !slices.Contains(oldLines, l) {
			lines = append(lines, l)
		}
	}
	for _, x := range kv {
		out, err := yaml.Marshal(map[string]any{x.k: x.v})
		if err != nil {
			return err
		}
		ln := strings.TrimRight(string(out), "\n")
		if strings.Contains(ln, "\n") {
			return fmt.Errorf(T("неверное значение %s", "bad value for %s"), x.k)
		}
		found := false
		for i, l := range lines {
			if strings.HasPrefix(l, x.k+":") {
				lines[i] = ln
				found = true
			}
		}
		if !found {
			lines = append(lines, note(x.k)...)
			lines = append(lines, ln)
		}
	}
	return writeFile(settingsPath(), []byte(strings.Join(lines, "\n")+"\n"))
}
