package main

import (
	"errors"
	"fmt"
	"strings"
)

func groupName(st *store, p *profile) (string, error) {
	v, err := load(st, p)
	if err != nil {
		return "", err
	}
	n := v.g.root(v.match, "rule")
	if n == "GLOBAL" || !v.g.group(n) {
		return "", errors.New(T("в профиле нет групп серверов", "the profile has no server groups"))
	}
	return n, nil
}

func cmdGroupName(args []string) error {
	fs := newFlags("servergr-name")
	which := fs.String("profile", "", "")
	all := fs.Bool("all-profiles", false, "")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if *which == "" && len(pos) > 0 {
		return errors.New(T("профиль задаётся флагом: flc servergr-name --profile <имя>",
			"pick the profile with a flag: flc servergr-name --profile <name>"))
	}
	if *all && *which != "" {
		return errors.New(T("либо --profile, либо --all-profiles", "use either --profile or --all-profiles"))
	}
	st, err := loadStore()
	if err != nil {
		return err
	}

	if *all {
		if len(st.Profiles) == 0 {
			fmt.Println(T("профилей нет, добавь: flc add-profile --link <ссылка>", "no profiles yet, add one: flc add-profile --link <url>"))
			return nil
		}
		var t table
		for _, p := range st.Profiles {
			mark := " "
			if p.ID == st.Current {
				mark = "*"
			}
			g, err := groupName(st, p)
			if err != nil {
				g = "-"
			} else {
				g = show(g)
			}
			t.add(mark+" "+show(p.Name), g)
		}
		t.print()
		return nil
	}

	var p *profile
	if *which != "" {
		p, err = st.find(strings.Join(append([]string{*which}, pos...), " "))
	} else {
		p, err = st.active()
	}
	if err != nil {
		return err
	}
	g, err := groupName(st, p)
	if err != nil {
		return err
	}
	fmt.Println(show(g))
	return nil
}
