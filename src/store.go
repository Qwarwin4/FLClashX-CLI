package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type subInfo struct {
	Up     int64 `json:"up"`
	Down   int64 `json:"down"`
	Total  int64 `json:"total"`
	Expire int64 `json:"expire"`
}

type profile struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	URL      string            `json:"url,omitempty"`
	Updated  time.Time         `json:"updated"`
	Info     *subInfo          `json:"info,omitempty"`
	Selected map[string]string `json:"selected,omitempty"`
}

type store struct {
	Current  string     `json:"current"`
	Profiles []*profile `json:"profiles"`
}

var idRe = regexp.MustCompile(`^[0-9a-f]{16}$`)

func statePath() string { return filepath.Join(dir.cfg, "state.json") }

func (p *profile) path() string { return filepath.Join(profDir(), p.ID+".yaml") }

func loadStore() (*store, error) {
	st := &store{}
	b, err := os.ReadFile(statePath())
	if errors.Is(err, fs.ErrNotExist) {
		return st, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, st); err != nil {
		return nil, fmt.Errorf(T("state.json повреждён: %w", "state.json is corrupted: %w"), err)
	}
	for _, p := range st.Profiles {
		if !idRe.MatchString(p.ID) {
			return nil, fmt.Errorf(T("state.json: подозрительный id профиля %q", "state.json: suspicious profile id %q"), p.ID)
		}
	}
	return st, nil
}

func (st *store) save() error {
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return writeFile(statePath(), b)
}

func (st *store) byID(id string) *profile {
	for _, p := range st.Profiles {
		if p.ID == id {
			return p
		}
	}
	return nil
}

func (st *store) cur() *profile { return st.byID(st.Current) }

func (st *store) active() (*profile, error) {
	if p := st.cur(); p != nil {
		return p, nil
	}
	return nil, errors.New(T("нет активного профиля, добавь: flc add-profile --link <ссылка>",
		"no active profile, add one: flc add-profile --link <url>"))
}

func (st *store) byName(n string, skip *profile) *profile {
	for _, p := range st.Profiles {
		if p != skip && strings.EqualFold(p.Name, n) {
			return p
		}
	}
	return nil
}

func (st *store) find(q string) (*profile, error) {
	if q == "" {
		return nil, errors.New(T("не указан профиль", "no profile given"))
	}
	for _, p := range st.Profiles {
		if p.Name == q {
			return p, nil
		}
	}
	if p := st.byName(q, nil); p != nil {
		return p, nil
	}
	if i, err := strconv.Atoi(q); err == nil && i >= 1 && i <= len(st.Profiles) {
		return st.Profiles[i-1], nil
	}
	return nil, fmt.Errorf(T("профиль %q не найден, смотри flc profile-list", "profile %q not found, see flc profile-list"), q)
}

func (st *store) remove(p *profile) {
	for i, x := range st.Profiles {
		if x == p {
			st.Profiles = append(st.Profiles[:i], st.Profiles[i+1:]...)
			break
		}
	}
	if st.Current == p.ID {
		st.Current = ""
		if len(st.Profiles) > 0 {
			st.Current = st.Profiles[0].ID
		}
	}
}

func (st *store) uniq(n string) string {
	if st.byName(n, nil) == nil {
		return n
	}
	for i := 2; ; i++ {
		c := fmt.Sprintf("%s (%d)", n, i)
		if st.byName(c, nil) == nil {
			return c
		}
	}
}

func newID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func checkName(n string) (string, error) {
	n = strings.TrimSpace(n)
	switch {
	case n == "":
		return "", errors.New(T("пустое имя", "empty name"))
	case utf8.RuneCountInString(n) > 64:
		return "", errors.New(T("имя длиннее 64 символов", "name is longer than 64 characters"))
	case strings.IndexFunc(n, func(r rune) bool { return unicode.IsControl(r) || isBidi(r) }) >= 0:
		return "", errors.New(T("в имени есть управляющие символы", "name contains control characters"))
	}
	return n, nil
}

func tidyName(n string) string {
	n = strings.TrimSpace(clean(n))
	if r := []rune(n); len(r) > 64 {
		n = strings.TrimSpace(string(r[:64]))
	}
	return n
}
