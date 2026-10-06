package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode"
)

func clean(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\t':
			return ' '
		case unicode.IsControl(r), isBidi(r), r == unicode.ReplacementChar:
			return -1
		}
		return r
	}, s)
}

func isBidi(r rune) bool {
	return r >= 0x202a && r <= 0x202e || r >= 0x2066 && r <= 0x2069 ||
		r == 0x200e || r == 0x200f || r == 0x061c
}

var stdin = bufio.NewReader(os.Stdin)

func confirm(q string, def bool) bool {
	hint := " [y/N] "
	if def {
		hint = " [Y/n] "
	}
	fmt.Print(q + hint)
	ln, err := stdin.ReadString('\n')
	if err != nil && ln == "" {
		fmt.Println()
		return false
	}
	switch strings.ToLower(strings.TrimSpace(ln)) {
	case "":
		return def
	case "y", "yes", "д", "да":
		return true
	}
	return false
}

func human(n int64) string {
	const k = 1024
	if n < k {
		return fmt.Sprintf("%d B", n)
	}
	f := float64(n)
	for _, u := range []string{"KB", "MB", "GB", "TB"} {
		f /= k
		if f < k || u == "TB" {
			return fmt.Sprintf("%.1f %s", f, u)
		}
	}
	return ""
}

func (in *subInfo) String() string {
	if in == nil {
		return "-"
	}
	s := human(in.Up + in.Down)
	if in.Total > 0 {
		s += " / " + human(in.Total)
	}
	return s
}

func expires(in *subInfo) string {
	if in == nil || in.Expire <= 0 {
		return "-"
	}
	t := time.Unix(in.Expire, 0)
	if time.Now().After(t) {
		return T("истекла", "expired")
	}
	return t.Format("2006-01-02")
}
