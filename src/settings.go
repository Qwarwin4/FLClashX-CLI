package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	"go.yaml.in/yaml/v3"
)

const defRu = `# Настройки flc. Применяются при следующем flc start.

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
# User-Agent при скачивании подписки
user-agent: clash.meta
# слать x-hwid (sha256 от machine-id) — нужно панелям с лимитом устройств
send-hwid: true
`

const defEn = `# flc settings. Applied on the next flc start.

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
# User-Agent used to fetch subscriptions
user-agent: clash.meta
# send x-hwid (sha256 of machine-id), needed by panels that limit devices
send-hwid: true
`

func settingsPath() string { return filepath.Join(dir.cfg, "settings.yaml") }

type settings struct {
	Tun       bool   `yaml:"tun"`
	Stack     string `yaml:"tun-stack"`
	MixedPort int    `yaml:"mixed-port"`
	Mode      string `yaml:"mode"`
	LogLevel  string `yaml:"log-level"`
	UA        string `yaml:"user-agent"`
	HWID      bool   `yaml:"send-hwid"`
}

func loadSettings() (settings, error) {
	var s settings
	def := T(defRu, defEn)
	if err := yaml.Unmarshal([]byte(def), &s); err != nil {
		return s, err
	}

	p := settingsPath()
	b, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return s, writeFile(p, []byte(def))
	}
	if err != nil {
		return s, err
	}
	if err := yaml.Unmarshal(b, &s); err != nil {
		return s, fmt.Errorf("%s: %w", p, err)
	}

	if s.Mode == "rules" {
		s.Mode = "rule"
	}

	bad := func(what string) error {
		return fmt.Errorf(T("%s: неверное значение %s", "%s: bad value for %s"), p, what)
	}
	switch {
	case !slices.Contains([]string{"system", "gvisor", "mixed"}, s.Stack):
		return s, bad("tun-stack")
	case s.MixedPort < 1 || s.MixedPort > 65535:
		return s, bad("mixed-port")
	case !slices.Contains([]string{"rule", "global", "direct"}, s.Mode):
		return s, bad("mode")
	case !slices.Contains([]string{"silent", "error", "warning", "info", "debug"}, s.LogLevel):
		return s, bad("log-level")
	case s.UA == "" || strings.IndexFunc(s.UA, func(r rune) bool { return !unicode.IsPrint(r) }) >= 0:
		return s, bad("user-agent")
	}
	return s, nil
}
