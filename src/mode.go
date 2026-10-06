package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

func cmdChangeMode(args []string) error {
	pos, err := parse(newFlags("change-mode"), args)
	if err != nil {
		return err
	}
	bad := errors.New(T("использование: flc change-mode rules | global | direct",
		"usage: flc change-mode rules | global | direct"))
	if len(pos) != 1 {
		return bad
	}
	m := strings.ToLower(pos[0])
	if m == "rules" {
		m = "rule"
	}
	if m != "rule" && m != "global" && m != "direct" {
		return bad
	}

	unlock, err := lock()
	if err != nil {
		return err
	}
	defer unlock()

	if _, err := loadSettings(); err != nil {
		return err
	}
	if err := setMode(m); err != nil {
		return err
	}

	name := modeName(m)
	if corePid() == 0 {
		fmt.Printf(T("режим: %s (применится при flc start)\n", "mode: %s (applies on flc start)\n"), name)
		return nil
	}
	a := newAPI()
	if err := a.setMode(m); err != nil {
		return err
	}
	fmt.Printf(T("режим: %s\n", "mode: %s\n"), name)

	if m == "direct" {
		return nil
	}
	st, err := loadStore()
	if err != nil {
		return err
	}
	if p := st.cur(); p != nil {
		if m == "global" {
			fixGlobal(a, p)
		}
		if v, err := load(st, p); err == nil {
			if cur := v.g.current(v.g.root(v.match, m)); cur != "" {
				fmt.Printf(T("сервер: %s\n", "server: %s\n"), clean(cur))
			}
		}
	}
	return nil
}

func modeName(m string) string {
	if m == "rule" {
		return "rules"
	}
	return m
}

func fixGlobal(a *api, p *profile) {
	if p.Selected["GLOBAL"] != "" {
		return
	}
	g, err := a.graph()
	if err != nil || g["GLOBAL"] == nil || !builtin[g["GLOBAL"].Now] {
		return
	}
	if root := g.root(a.matchTarget(), "rule"); root != "GLOBAL" {
		_ = a.choose("GLOBAL", root)
	}
}

func setMode(m string) error {
	b, err := os.ReadFile(settingsPath())
	if err != nil {
		return err
	}
	lines := strings.Split(string(b), "\n")
	found := false
	for i, ln := range lines {
		if strings.HasPrefix(ln, "mode:") {
			lines[i] = "mode: " + m
			found = true
		}
	}
	out := strings.Join(lines, "\n")
	if !found {
		out = strings.TrimRight(out, "\n") + "\nmode: " + m + "\n"
	}
	return writeFile(settingsPath(), []byte(out))
}
