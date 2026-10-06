package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

type node struct {
	Type    string   `json:"type"`
	Now     string   `json:"now"`
	All     []string `json:"all"`
	History []struct {
		Delay int `json:"delay"`
	} `json:"history"`
}

type graph map[string]*node

var groupTypes = map[string]string{
	"select":       "Selector",
	"url-test":     "URLTest",
	"fallback":     "Fallback",
	"load-balance": "LoadBalance",
	"relay":        "Relay",
}

var builtin = map[string]bool{
	"DIRECT": true, "REJECT": true, "REJECT-DROP": true,
	"PASS": true, "COMPATIBLE": true, "GLOBAL": true,
}

func (g graph) group(n string) bool {
	x := g[n]
	if x == nil {
		return false
	}
	for _, t := range groupTypes {
		if x.Type == t {
			return true
		}
	}
	return false
}

func (g graph) selector(n string) bool {
	x := g[n]
	return x != nil && x.Type == "Selector"
}

func (g graph) delay(n string) int {
	x := g[n]
	if x == nil || len(x.History) == 0 {
		return 0
	}
	return x.History[len(x.History)-1].Delay
}

func (g graph) root(match, mode string) string {
	if mode == "global" && g["GLOBAL"] != nil {
		return "GLOBAL"
	}
	if g.group(match) {
		return match
	}
	if gl := g["GLOBAL"]; gl != nil {
		for _, n := range gl.All {
			if g.selector(n) {
				return n
			}
		}
	}
	return "GLOBAL"
}

func (g graph) servers(root string) []string {
	var out []string
	seen := map[string]bool{}
	var walk func(string)
	walk = func(n string) {
		if seen[n] {
			return
		}
		seen[n] = true
		x := g[n]
		switch {
		case x == nil:
		case g.group(n):
			for _, c := range x.All {
				walk(c)
			}
		case !builtin[n]:
			out = append(out, n)
		}
	}
	walk(root)
	walk("GLOBAL")
	return out
}

func (g graph) current(root string) string {
	n := root
	for i := 0; i < 32 && g.group(n) && g[n].Now != ""; i++ {
		n = g[n].Now
	}
	if g.group(n) {
		return ""
	}
	return n
}

func (g graph) path(from, to string, seen map[string]bool) []string {
	if !g.selector(from) || seen[from] {
		return nil
	}
	seen[from] = true
	for _, c := range g[from].All {
		if c == to {
			return []string{from}
		}
	}
	for _, c := range g[from].All {
		if p := g.path(c, to, seen); p != nil {
			return append([]string{from}, p...)
		}
	}
	return nil
}

func (g graph) route(root, target string) [][2]string {
	p := g.path(root, target, map[string]bool{})
	if p == nil && g["GLOBAL"] != nil {
		for _, n := range g["GLOBAL"].All {
			if p = g.path(n, target, map[string]bool{}); p != nil {
				break
			}
		}
	}
	steps := make([][2]string, len(p))
	for i := range p {
		next := target
		if i+1 < len(p) {
			next = p[i+1]
		}
		steps[i] = [2]string{p[i], next}
	}
	return steps
}

func fileGraph(p *profile) (graph, string, bool, error) {
	b, err := os.ReadFile(p.path())
	if err != nil {
		return nil, "", false, err
	}
	var c struct {
		Proxies []struct {
			Name string `yaml:"name"`
			Type string `yaml:"type"`
		} `yaml:"proxies"`
		Groups []struct {
			Name    string   `yaml:"name"`
			Type    string   `yaml:"type"`
			Proxies []string `yaml:"proxies"`
		} `yaml:"proxy-groups"`
		Providers map[string]any `yaml:"proxy-providers"`
		Rules     []any          `yaml:"rules"`
	}
	if err := yaml.Unmarshal(b, &c); err != nil {
		return nil, "", false, fmt.Errorf(T("профиль «%s» не читается: %w", "can't read profile %q: %w"), p.Name, err)
	}

	g := graph{}
	gl := &node{Type: "Selector"}
	for _, x := range c.Groups {
		t, ok := groupTypes[x.Type]
		if !ok {
			t = x.Type
		}
		g[x.Name] = &node{Type: t, All: x.Proxies, Now: p.Selected[x.Name]}
		gl.All = append(gl.All, x.Name)
	}
	for _, x := range c.Proxies {
		g[x.Name] = &node{Type: x.Type}
		gl.All = append(gl.All, x.Name)
	}
	gl.Now = p.Selected["GLOBAL"]
	g["GLOBAL"] = gl
	for _, x := range g {
		if x.Type == "Selector" && x.Now == "" && len(x.All) > 0 {
			x.Now = x.All[0]
		}
	}

	match := ""
	for i := len(c.Rules) - 1; i >= 0; i-- {
		s, _ := c.Rules[i].(string)
		f := strings.Split(s, ",")
		if len(f) >= 2 && strings.EqualFold(strings.TrimSpace(f[0]), "MATCH") {
			match = strings.TrimSpace(f[1])
			break
		}
	}
	return g, match, len(c.Providers) > 0, nil
}

func pick(list []string, q string) (string, error) {
	for _, n := range list {
		if n == q {
			return n, nil
		}
	}
	if i, err := strconv.Atoi(q); err == nil {
		if i >= 1 && i <= len(list) {
			return list[i-1], nil
		}
		return "", fmt.Errorf(T("номер %d вне списка (1..%d)", "number %d is out of range (1..%d)"), i, len(list))
	}
	lq := strings.ToLower(q)
	var hits []string
	for _, n := range list {
		ln := strings.ToLower(n)
		if ln == lq {
			return n, nil
		}
		if strings.Contains(ln, lq) {
			hits = append(hits, n)
		}
	}
	switch len(hits) {
	case 0:
		return "", fmt.Errorf(T("сервер %q не найден, смотри flc server-list", "server %q not found, see flc server-list"), q)
	case 1:
		return hits[0], nil
	}
	for i := range hits {
		hits[i] = clean(hits[i])
	}
	if len(hits) > 6 {
		hits = append(hits[:6], "...")
	}
	return "", errors.New(T("подходит несколько серверов: ", "several servers match: ") + strings.Join(hits, ", "))
}
