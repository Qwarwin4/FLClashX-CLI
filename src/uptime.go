package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func sincePath() string { return filepath.Join(dir.run, "since") }

func markStart() {
	_ = writeFile(sincePath(), []byte(strconv.FormatInt(time.Now().Unix(), 10)+"\n"))
}

func clearStart() { _ = os.Remove(sincePath()) }

func procStart(pid int) (time.Time, bool) {
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return time.Time{}, false
	}
	i := strings.LastIndexByte(string(b), ')')
	if i < 0 {
		return time.Time{}, false
	}
	f := strings.Fields(string(b[i+1:]))
	if len(f) < 20 {
		return time.Time{}, false
	}
	ticks, err := strconv.ParseInt(f[19], 10, 64)
	if err != nil {
		return time.Time{}, false
	}
	st, err := os.ReadFile("/proc/stat")
	if err != nil {
		return time.Time{}, false
	}
	for _, ln := range strings.Split(string(st), "\n") {
		if v, ok := strings.CutPrefix(ln, "btime "); ok {
			bt, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
			if err != nil {
				return time.Time{}, false
			}
			return time.Unix(bt, 0).Add(time.Duration(ticks) * time.Second / 100), true
		}
	}
	return time.Time{}, false
}

func upSince(pid int) (time.Time, bool) {
	now := time.Now()
	if b, err := os.ReadFile(sincePath()); err == nil {
		if n, err := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64); err == nil {
			if t := time.Unix(n, 0); !t.After(now) {
				return t, true
			}
		}
	}
	return procStart(pid)
}

func uptime(t time.Time) string {
	d := time.Since(t)
	if d < 0 {
		d = 0
	}
	s := int64(d / time.Second)
	days, h, m, sec := s/86400, s%86400/3600, s%3600/60, s%60
	out := fmt.Sprintf("%02d:%02d:%02d", h, m, sec)
	if days > 0 {
		out = fmt.Sprintf(T("%d д ", "%dd ")+"%s", days, out)
	}
	at := t.Local().Format("15:04")
	if y, mo, dd := t.Local().Date(); y != time.Now().Year() || mo != time.Now().Month() || dd != time.Now().Day() {
		at = t.Local().Format("2006-01-02 15:04")
	}
	return out + fmt.Sprintf(T(" (с %s)", " (since %s)"), at)
}
