package main

import "strings"

var viaVPN = []string{
	"youtube.com", "youtu.be", "googlevideo.com", "ytimg.com", "ggpht.com",
	"youtube-nocookie.com", "youtubei.googleapis.com", "yt.be",
}

func vpnTarget(m map[string]any) string {
	rules, _ := m["rules"].([]any)
	for i := len(rules) - 1; i >= 0; i-- {
		r, _ := rules[i].(string)
		t, _ := ruleTarget(r)
		if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(r)), "MATCH,") && t != "" && !builtin[t] {
			return t
		}
	}
	list, _ := m["proxy-groups"].([]any)
	for _, x := range list {
		if e, ok := x.(map[string]any); ok {
			if n, _ := e["name"].(string); n != "" {
				return n
			}
		}
	}
	return ""
}

func antiBlock(m map[string]any, s settings) {
	if !s.AntiBlock {
		return
	}
	var add []any
	add = append(add, "AND,((NETWORK,UDP),(DST-PORT,443)),REJECT")
	if t := vpnTarget(m); t != "" {
		for _, d := range viaVPN {
			add = append(add, "DOMAIN-SUFFIX,"+d+","+t)
		}
	}
	if v6, ok := m["ipv6"].(bool); ok && !v6 && s.Tun {
		m["ipv6"] = true
		if dns, ok := m["dns"].(map[string]any); ok {
			dns["ipv6"] = false
		}
		add = append(add, "IP-CIDR6,::/0,REJECT,no-resolve")
	}
	if _, ok := m["sniffer"]; !ok {
		m["sniffer"] = map[string]any{
			"enable": true,
			"sniff": map[string]any{
				"HTTP": map[string]any{"ports": []string{"80", "8080-8880"}, "override-destination": true},
				"TLS":  map[string]any{"ports": []string{"443", "8443"}},
				"QUIC": map[string]any{"ports": []string{"443", "8443"}},
			},
		}
	}
	rules, _ := m["rules"].([]any)
	m["rules"] = append(add, rules...)
}
