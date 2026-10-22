package main

import (
	"fmt"
	"regexp"
	"strings"
)

var appRe = regexp.MustCompile(`^[A-Za-z0-9._+*-]{1,64}$`)

func apps(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' })
}

func appsBad(s string) string {
	list := apps(s)
	if len(list) > 30 {
		return T("не больше 30 приложений", "30 apps at most")
	}
	for _, a := range list {
		if !appRe.MatchString(a) {
			return fmt.Sprintf(T("странное имя приложения %q", "odd app name %q"), clean(a))
		}
	}
	return ""
}

func tidyApps(s string) string { return strings.Join(apps(s), ", ") }

func appRule(name, target string) string {
	kind := "PROCESS-NAME"
	if strings.Contains(name, "*") {
		kind = "PROCESS-NAME-WILDCARD"
	}
	return kind + "," + name + "," + target
}

func vpnTarget(m map[string]any) string {
	rules, _ := m["rules"].([]any)
	for i := len(rules) - 1; i >= 0; i-- {
		r, _ := rules[i].(string)
		if t, _ := ruleTarget(r); strings.HasPrefix(strings.ToUpper(strings.TrimSpace(r)), "MATCH,") && t != "" && !builtin[t] {
			return t
		}
	}
	for _, key := range []string{"proxy-groups", "proxies"} {
		list, _ := m[key].([]any)
		for _, x := range list {
			if e, ok := x.(map[string]any); ok {
				if n, _ := e["name"].(string); n != "" {
					return n
				}
			}
		}
	}
	return ""
}

func addAppRules(m map[string]any, s settings) {
	vpn, direct := apps(s.AppsVPN), apps(s.AppsDirect)
	if len(vpn)+len(direct) == 0 {
		return
	}
	var add []any
	for _, a := range direct {
		add = append(add, appRule(a, "DIRECT"))
	}
	if t := vpnTarget(m); t != "" {
		for _, a := range vpn {
			add = append(add, appRule(a, t))
		}
	}
	rules, _ := m["rules"].([]any)
	m["rules"] = append(add, rules...)
}

func appsShow(v string) string {
	if v == "" {
		return T("нет", "none")
	}
	return v
}
