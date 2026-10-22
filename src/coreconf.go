package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v3"
)

const coreRu = `# Свои настройки ядра mihomo. Они ложатся поверх любого профиля.
# Пиши ключи как в обычном конфиге mihomo, вложенные разделы сливаются.
# proxies, proxy-groups, rules и провайдеры здесь игнорируются, их правь в flc config.
# Всё, что открывает порты наружу или трогает систему, выкидывается, как и в профилях.
#
# ipv6: false
# tcp-concurrent: true
# unified-delay: true
# global-client-fingerprint: chrome
#
# dns:
#   enhanced-mode: fake-ip
#   nameserver:
#     - https://1.1.1.1/dns-query
#
# sniffer:
#   enable: true
#
# tun:
#   mtu: 1500
#   route-exclude-address:
#     - 192.168.0.0/16
`

const coreEn = `# Your own mihomo core settings. They go on top of every profile.
# Write keys as in a normal mihomo config, nested sections are merged.
# proxies, proxy-groups, rules and providers are ignored here, edit those in flc config.
# Anything that would open ports or touch the system is dropped, same as in profiles.
#
# ipv6: false
# tcp-concurrent: true
# unified-delay: true
# global-client-fingerprint: chrome
#
# dns:
#   enhanced-mode: fake-ip
#   nameserver:
#     - https://1.1.1.1/dns-query
#
# sniffer:
#   enable: true
#
# tun:
#   mtu: 1500
#   route-exclude-address:
#     - 192.168.0.0/16
`

var profileKeys = map[string]bool{
	"proxies": true, "proxy-groups": true, "proxy-providers": true,
	"rules": true, "sub-rules": true, "rule-providers": true,
}

func corePath() string { return filepath.Join(dir.cfg, "core.yaml") }

func coreOverrides() (map[string]any, error) {
	b, err := os.ReadFile(corePath())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := yaml.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("%s: %w", corePath(), err)
	}
	for k := range profileKeys {
		delete(m, k)
	}
	return m, nil
}

func merge(dst, src map[string]any) {
	for k, v := range src {
		sv, ok := v.(map[string]any)
		dv, ok2 := dst[k].(map[string]any)
		if ok && ok2 {
			merge(dv, sv)
			continue
		}
		dst[k] = v
	}
}

func coreCount() int {
	m, err := coreOverrides()
	if err != nil {
		return -1
	}
	return len(m)
}

func editCore() (string, error) {
	orig, err := os.ReadFile(corePath())
	if errors.Is(err, fs.ErrNotExist) {
		orig, err = []byte(T(coreRu, coreEn)), nil
	}
	if err != nil {
		return "", err
	}
	b, err := editYAML(orig, dir.cfg, false)
	if err != nil {
		return "", err
	}
	if b == nil {
		return T("конфигурация ядра не изменилась", "the core config didn't change"), nil
	}

	unlock, err := lock()
	if err != nil {
		return "", err
	}
	defer unlock()
	if err := writeFile(corePath(), b); err != nil {
		return "", err
	}
	if corePid() == 0 {
		return T("конфигурация ядра сохранена, применится при flc start", "core config saved, it applies on flc start"), nil
	}
	st, err := loadStore()
	if err != nil {
		return "", err
	}
	p, err := st.active()
	if err != nil {
		return "", err
	}
	s, err := loadSettings()
	if err != nil {
		return "", err
	}
	if err := reloadLive(p, s); err != nil {
		return "", fmt.Errorf(T("сохранено, но ядро не приняло: %w", "saved, but the core rejected it: %w"), err)
	}
	return T("конфигурация ядра сохранена, VPN подхватил её на лету", "core config saved, the VPN picked it up on the fly"), nil
}

func applyLive(p *profile) error {
	s, err := loadSettings()
	if err != nil {
		return err
	}
	err = reloadLive(p, s)
	if err == nil {
		fmt.Println(T("VPN подхватил изменения без перезапуска", "the VPN picked up the changes without a restart"))
		return nil
	}
	fmt.Fprintf(os.Stderr, T("на лету не применилось: %v\n", "couldn't apply it live: %v\n"), err)
	if !confirm(T("перезапустить VPN?", "restart the VPN?"), true) {
		return nil
	}
	return restart(p, s)
}
