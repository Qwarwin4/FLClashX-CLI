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
# слать x-hwid (sha256 от machine-id) — нужно панелям с лимитом устройств
send-hwid: true
# флаги стран в именах серверов: auto | emoji | text (text — код страны буквами)
flags: auto
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
# send x-hwid (sha256 of machine-id), needed by panels that limit devices
send-hwid: true
# country flags in server names: auto | emoji | text (text = country code in letters)
flags: auto
`

func settingsPath() string { return filepath.Join(dir.cfg, "settings.yaml") }

type settings struct {
	Tun       bool   `yaml:"tun"`
	Stack     string `yaml:"tun-stack"`
	MixedPort int    `yaml:"mixed-port"`
	Mode      string `yaml:"mode"`
	LogLevel  string `yaml:"log-level"`
	HWID      bool   `yaml:"send-hwid"`
	Flags     string `yaml:"flags"`
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
	}
	return ""
}

var (
	stacks    = []string{"mixed", "system", "gvisor"}
	modes     = []string{"rule", "global", "direct"}
	levels    = []string{"silent", "error", "warning", "info", "debug"}
	flagModes = []string{"auto", "emoji", "text"}
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

var oldUA = []string{"# User-Agent при скачивании подписки", "# User-Agent used to fetch subscriptions"}

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
		{"log-level", s.LogLevel}, {"send-hwid", s.HWID}, {"flags", s.Flags},
	}
	var lines []string
	for _, l := range strings.Split(strings.TrimRight(string(b), "\n"), "\n") {
		if !strings.HasPrefix(l, "user-agent:") && !slices.Contains(oldUA, l) {
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
