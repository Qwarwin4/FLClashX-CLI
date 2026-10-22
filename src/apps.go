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

func addAppRules(m map[string]any, s settings) {
	direct := apps(s.AppsDirect)
	if len(direct) == 0 {
		return
	}
	var add []any
	for _, a := range direct {
		add = append(add, appRule(a, "DIRECT"))
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
