package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var lang = "en"

func T(ru, en string) string {
	if lang == "ru" {
		return ru
	}
	return en
}

func langPath() string { return filepath.Join(dir.cfg, "lang") }

func loadLang() {
	if dir.cfg != "" {
		if b, err := os.ReadFile(langPath()); err == nil {
			if l := strings.TrimSpace(string(b)); l == "ru" || l == "en" {
				lang = l
				return
			}
		}
	}
	for _, k := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := os.Getenv(k); v != "" {
			if strings.HasPrefix(v, "ru") {
				lang = "ru"
			}
			return
		}
	}
}

func cmdLang(args []string) error {
	pos, err := parse(newFlags("lang"), args)
	if err != nil {
		return err
	}
	if len(pos) == 0 {
		fmt.Println(lang)
		return nil
	}
	l := strings.ToLower(pos[0])
	if len(pos) > 1 || (l != "ru" && l != "en") {
		return errors.New(T("использование: flc lang ru | en", "usage: flc lang ru | en"))
	}

	unlock, err := lock()
	if err != nil {
		return err
	}
	defer unlock()
	if err := writeFile(langPath(), []byte(l+"\n")); err != nil {
		return err
	}
	lang = l
	fmt.Println(T("язык: русский", "language: English"))
	return nil
}
