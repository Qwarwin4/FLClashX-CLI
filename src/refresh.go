package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

var ruleKeys = []string{"rules", "sub-rules", "rule-providers"}

func fetchProfile(p *profile, s settings, allowHTTP bool) ([]byte, *sub, string, error) {
	sb, err := download(p.URL, s, allowHTTP)
	if err != nil {
		return nil, nil, "", err
	}
	cfg, err := normalize(sb.body)
	if err != nil {
		return nil, nil, "", err
	}
	note := ""
	if s.KeepRules {
		if old, err := os.ReadFile(p.path()); err == nil {
			cfg, note = keepRules(old, cfg)
		}
	}
	return cfg, sb, note, nil
}

func saveProfile(id string, cfg []byte, info *subInfo) (*store, *profile, bool, error) {
	st, err := loadStore()
	if err != nil {
		return nil, nil, false, err
	}
	p := st.byID(id)
	if p == nil {
		return nil, nil, false, errors.New(T("пока качалась подписка, профиль удалили", "the profile was deleted while the subscription was downloading"))
	}
	old, _ := os.ReadFile(p.path())
	changed := !bytes.Equal(old, cfg)
	if changed {
		if err := writeFile(p.path(), cfg); err != nil {
			return nil, nil, false, err
		}
	}
	p.Updated, p.Info = time.Now(), info
	return st, p, changed, st.save()
}

func keepRules(old, fresh []byte) ([]byte, string) {
	var om, nm map[string]any
	if yaml.Unmarshal(old, &om) != nil || yaml.Unmarshal(fresh, &nm) != nil || om == nil || nm == nil {
		return fresh, ""
	}
	rules, _ := om["rules"].([]any)
	if len(rules) == 0 {
		return fresh, ""
	}

	have := map[string]bool{}
	for n := range builtin {
		have[n] = true
	}
	for _, key := range []string{"proxies", "proxy-groups"} {
		list, _ := nm[key].([]any)
		for _, x := range list {
			if e, ok := x.(map[string]any); ok {
				if n, ok := e["name"].(string); ok {
					have[n] = true
				}
			}
		}
	}
	subs, _ := om["sub-rules"].(map[string]any)

	var lost []string
	check := func(list []any) {
		for _, r := range list {
			rs, _ := r.(string)
			t, sub := ruleTarget(rs)
			ok := t == "" || have[t]
			if sub {
				_, ok = subs[t]
			}
			if !ok && !slices.Contains(lost, t) {
				lost = append(lost, t)
			}
		}
	}
	check(rules)
	for _, v := range subs {
		l, _ := v.([]any)
		check(l)
	}
	if len(lost) > 0 {
		for i := range lost {
			lost[i] = show(lost[i])
		}
		if len(lost) > 5 {
			lost = append(lost[:5], "…")
		}
		return fresh, fmt.Sprintf(T("свои правила не сохранены: в новой подписке нет %s",
			"your rules weren't kept: the new subscription has no %s"), strings.Join(lost, ", "))
	}

	for _, k := range ruleKeys {
		if v, ok := om[k]; ok {
			nm[k] = v
		} else {
			delete(nm, k)
		}
	}
	out, err := yaml.Marshal(nm)
	if err != nil {
		return fresh, ""
	}
	return out, T("свои правила маршрутизации сохранены", "your routing rules were kept")
}

func ruleTarget(r string) (string, bool) {
	r = strings.TrimSpace(r)
	typ, rest, ok := strings.Cut(r, ",")
	if !ok {
		return "", false
	}
	typ = strings.ToUpper(strings.TrimSpace(typ))
	switch typ {
	case "MATCH", "FINAL":
		t, _, _ := strings.Cut(rest, ",")
		return strings.TrimSpace(t), false
	case "AND", "OR", "NOT", "SUB-RULE":
		rest = strings.TrimSpace(rest)
		depth, i := 0, 0
		for ; i < len(rest); i++ {
			switch rest[i] {
			case '(':
				depth++
			case ')':
				depth--
			}
			if depth == 0 {
				break
			}
		}
		if i >= len(rest) {
			return "", false
		}
		after := strings.TrimPrefix(strings.TrimSpace(rest[i+1:]), ",")
		t, _, _ := strings.Cut(after, ",")
		return strings.TrimSpace(t), typ == "SUB-RULE"
	}
	f := strings.Split(rest, ",")
	if len(f) < 2 {
		return "", false
	}
	return strings.TrimSpace(f[1]), false
}
