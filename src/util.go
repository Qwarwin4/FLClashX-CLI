package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
	"unicode"

	"golang.org/x/sys/unix"
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

func show(s string) string {
	s = clean(s)
	if !textFlags() {
		return s
	}
	return flagText(s)
}

func isRI(r rune) bool { return r >= 0x1f1e6 && r <= 0x1f1ff }

func flagText(s string) string {
	rs := []rune(s)
	var b strings.Builder
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		switch {
		case isRI(r) && i+1 < len(rs) && isRI(rs[i+1]):
			b.WriteRune('A' + r - 0x1f1e6)
			b.WriteRune('A' + rs[i+1] - 0x1f1e6)
			i++
			if i+1 < len(rs) && (unicode.IsLetter(rs[i+1]) || unicode.IsDigit(rs[i+1])) {
				b.WriteByte(' ')
			}
		case isRI(r), r >= 0xe0000 && r <= 0xe007f:
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

var flagsOnce = sync.OnceValue(func() bool {
	mode := "auto"
	if s, err := loadSettings(); err == nil {
		mode = s.Flags
	}
	switch mode {
	case "emoji":
		return false
	case "text":
		return true
	}
	return !flagTerm()
})

func textFlags() bool { return flagsOnce() }

func flagTerm() bool {
	if _, err := unix.IoctlGetTermios(int(os.Stdout.Fd()), unix.TCGETS); err != nil {
		return true
	}
	env := os.Getenv
	t, prog := env("TERM"), strings.ToLower(env("TERM_PROGRAM"))
	switch {
	case env("ALACRITTY_WINDOW_ID") != "" || env("ALACRITTY_SOCKET") != "" || strings.HasPrefix(t, "alacritty"):
		return false
	case env("KITTY_WINDOW_ID") != "" || t == "xterm-kitty":
		return true
	case strings.HasPrefix(t, "foot"), t == "xterm-ghostty", t == "wezterm", strings.HasPrefix(t, "contour"):
		return true
	case prog == "wezterm" || prog == "ghostty" || env("WEZTERM_PANE") != "" || env("GHOSTTY_RESOURCES_DIR") != "":
		return true
	}
	return false
}

var wide = [][2]rune{
	{0x1100, 0x115f}, {0x231a, 0x231b}, {0x23e9, 0x23ec}, {0x23f0, 0x23f0}, {0x23f3, 0x23f3},
	{0x25fd, 0x25fe}, {0x2614, 0x2615}, {0x2648, 0x2653}, {0x267f, 0x267f}, {0x2693, 0x2693},
	{0x26a1, 0x26a1}, {0x26aa, 0x26ab}, {0x26bd, 0x26be}, {0x26c4, 0x26c5}, {0x26ce, 0x26ce},
	{0x26d4, 0x26d4}, {0x26ea, 0x26ea}, {0x26f2, 0x26f3}, {0x26f5, 0x26f5}, {0x26fa, 0x26fa},
	{0x26fd, 0x26fd}, {0x2705, 0x2705}, {0x270a, 0x270b}, {0x2728, 0x2728}, {0x274c, 0x274c},
	{0x274e, 0x274e}, {0x2753, 0x2755}, {0x2757, 0x2757}, {0x2795, 0x2797}, {0x27b0, 0x27b0},
	{0x27bf, 0x27bf}, {0x2b1b, 0x2b1c}, {0x2b50, 0x2b50}, {0x2b55, 0x2b55}, {0x2e80, 0xa4cf},
	{0xac00, 0xd7a3}, {0xf900, 0xfaff}, {0xfe30, 0xfe4f}, {0xff00, 0xff60}, {0xffe0, 0xffe6},
	{0x1f004, 0x1f004}, {0x1f0cf, 0x1f0cf}, {0x1f18e, 0x1f18e}, {0x1f191, 0x1f19a},
	{0x1f200, 0x1f2ff}, {0x1f300, 0x1f64f}, {0x1f680, 0x1f6ff}, {0x1f7e0, 0x1f7eb},
	{0x1f90c, 0x1f9ff}, {0x1fa70, 0x1faff}, {0x20000, 0x3fffd},
}

func cells(s string) int {
	n := 0
	for _, r := range s {
		switch {
		case r == 0x200d, r >= 0xfe00 && r <= 0xfe0f, r >= 0xe0000 && r <= 0xe007f,
			unicode.In(r, unicode.Mn, unicode.Me, unicode.Cf):
		default:
			n++
			for _, w := range wide {
				if r >= w[0] && r <= w[1] {
					n++
					break
				}
			}
		}
	}
	return n
}

type table [][]string

func (t *table) add(cols ...string) { *t = append(*t, cols) }

func (t table) print() {
	var ws []int
	for _, row := range t {
		for i, c := range row {
			if i >= len(ws) {
				ws = append(ws, 0)
			}
			ws[i] = max(ws[i], cells(c))
		}
	}
	var b strings.Builder
	for _, row := range t {
		ln := ""
		for i, c := range row {
			ln += c
			if i < len(row)-1 {
				ln += strings.Repeat(" ", ws[i]-cells(c)+2)
			}
		}
		b.WriteString(strings.TrimRight(ln, " ") + "\n")
	}
	fmt.Print(b.String())
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
