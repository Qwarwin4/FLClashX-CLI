package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"unicode/utf8"

	"github.com/metacubex/mihomo/constant/features"
	"golang.org/x/sys/unix"
)

const (
	kToggle = iota
	kChoice
	kNumber
)

type form struct {
	s    settings
	lang string
}

type field struct {
	label  func() string
	help   func(*form) string
	kind   int
	opts   []string
	show   func(string) string
	get    func(*form) string
	set    func(*form, string)
	lo, hi int
	bad    func() string
	ok     func(int) bool
}

type tab struct {
	name  func() string
	items []field
}

func toggle(get func(*settings) *bool) (func(*form) string, func(*form, string)) {
	return func(f *form) string { return strconv.FormatBool(*get(&f.s)) },
		func(f *form, v string) { *get(&f.s) = v == "true" }
}

func tabs() []tab {
	same := func(v string) string { return v }
	tg := func(label, help func() string, get func(*settings) *bool) field {
		g, s := toggle(get)
		return field{label: label, help: func(*form) string { return help() }, kind: kToggle, get: g, set: s}
	}
	general := []field{
		{
			label: func() string { return T("Язык", "Language") },
			help: func(*form) string {
				return T("Язык, на котором FLClashX CLI с тобой говорит.", "The language FLClashX CLI talks to you in.")
			},
			kind: kChoice,
			opts: []string{"en", "ru"},
			show: func(v string) string {
				if v == "ru" {
					return "Русский"
				}
				return "English"
			},
			get: func(f *form) string { return f.lang },
			set: func(f *form, v string) { f.lang, lang = v, v },
		},
		tg(func() string { return T("Весь трафик (TUN)", "System-wide (TUN)") },
			func() string {
				return T("Включено: через VPN идёт весь трафик системы. Выключено: работает только прокси на 127.0.0.1.",
					"On: all system traffic goes through the VPN. Off: only the proxy on 127.0.0.1 works.")
			},
			func(s *settings) *bool { return &s.Tun }),
		{
			label: func() string { return T("Стек TUN", "TUN stack") },
			help: func(*form) string {
				h := T("Как TUN обрабатывает пакеты. mixed подходит почти всегда, system помогает, если TUN капризничает в виртуалке.",
					"How TUN handles packets. mixed works almost everywhere; try system if TUN misbehaves in a VM.")
				if !features.WithGVisor {
					h += T(" Эта сборка умеет только system.", " This build only has system.")
				}
				return h
			},
			kind: kChoice,
			opts: stacks,
			show: same,
			get:  func(f *form) string { return f.s.Stack },
			set:  func(f *form, v string) { f.s.Stack = v },
		},
		{
			label: func() string { return T("Порт прокси", "Proxy port") },
			help: func(*form) string {
				return T("Порт локального http/socks прокси. Слушает только 127.0.0.1.",
					"Port of the local http/socks proxy. It only listens on 127.0.0.1.")
			},
			kind: kNumber,
			lo:   1,
			hi:   65535,
			bad: func() string {
				return T("порт должен быть от 1 до 65535", "the port must be between 1 and 65535")
			},
			get: func(f *form) string { return strconv.Itoa(f.s.MixedPort) },
			set: func(f *form, v string) {
				if n, err := strconv.Atoi(v); err == nil {
					f.s.MixedPort = n
				}
			},
		},
		{
			label: func() string { return T("Режим", "Routing mode") },
			help: func(*form) string {
				return T("rules: по правилам подписки. global: всё через VPN. direct: ничего через VPN.",
					"rules: follow the subscription. global: everything through the VPN. direct: nothing through it.")
			},
			kind: kChoice,
			opts: modes,
			show: modeName,
			get:  func(f *form) string { return f.s.Mode },
			set:  func(f *form, v string) { f.s.Mode = v },
		},
		{
			label: func() string { return T("Подробность лога", "Log level") },
			help: func(*form) string {
				return T("Сколько ядро пишет в flc debug log. На info в лог попадает каждый сайт, который ты открываешь.",
					"How much the core writes to flc debug log. At info every site you open ends up in the log.")
			},
			kind: kChoice,
			opts: levels,
			show: same,
			get:  func(f *form) string { return f.s.LogLevel },
			set:  func(f *form, v string) { f.s.LogLevel = v },
		},
		{
			label: func() string { return T("Флаги стран", "Country flags") },
			help: func(f *form) string {
				ex := "🇩🇪 Germany · 🇳🇱 Amsterdam"
				if flagLetters(f.s.Flags) {
					ex = flagText(ex)
				}
				return T("Как рисовать флаги стран. «Буквы» — код страны вместо флага, для терминалов вроде Alacritty. Сейчас так:",
					"How country flags are drawn. Letters put the country code in place of the flag, for terminals like Alacritty. Right now:") + "\n" + ex
			},
			kind: kChoice,
			opts: flagModes,
			show: func(v string) string {
				switch v {
				case "emoji":
					return T("эмодзи", "emoji")
				case "text":
					return T("буквы", "letters")
				}
				if flagLetters(v) {
					return T("авто (буквы)", "auto (letters)")
				}
				return T("авто (эмодзи)", "auto (emoji)")
			},
			get: func(f *form) string { return f.s.Flags },
			set: func(f *form, v string) { f.s.Flags = v },
		},
	}

	updates := []field{
		{
			label: func() string { return T("Обновлять подписку", "Refresh subscription") },
			help: func(*form) string {
				return fmt.Sprintf(T("Раз в сколько минут обновлять активную подписку, пока VPN работает. 0 — не обновлять. От %d до %d минут.",
					"How often, in minutes, to refresh the active subscription while the VPN runs. 0 = never. %d to %d minutes."), minInterval, maxInterval)
			},
			kind: kNumber,
			lo:   0,
			hi:   maxInterval,
			bad: func() string {
				return fmt.Sprintf(T("0 или от %d до %d минут", "0 or %d to %d minutes"), minInterval, maxInterval)
			},
			ok: func(n int) bool { return n == 0 || n >= minInterval },
			show: func(v string) string {
				if v == "0" {
					return T("никогда", "never")
				}
				return fmt.Sprintf(T("каждые %s мин", "every %s min"), v)
			},
			get: func(f *form) string { return strconv.Itoa(f.s.Interval) },
			set: func(f *form, v string) {
				if n, err := strconv.Atoi(v); err == nil {
					f.s.Interval = n
				}
			},
		},
		tg(func() string { return T("Сохранять свои правила", "Keep my rules") },
			func() string {
				return T("При обновлении подписки оставлять правила маршрутизации, которые ты правил в flc config. Если в новой подписке пропала группа, на которую они ссылаются, возьмутся правила подписки.",
					"When a subscription is refreshed, keep the routing rules you edited in flc config. If a group they point to is gone from the new subscription, its own rules are used.")
			},
			func(s *settings) *bool { return &s.KeepRules }),
		tg(func() string { return T("Проверять новые версии", "Check for new versions") },
			func() string {
				return T("Раз в сутки, пока работает VPN, спрашивать GitHub о новой версии FLClashX CLI. Сам ничего не ставит: покажет уведомление и подсказку в flc status.",
					"Once a day, while the VPN runs, ask GitHub about a new FLClashX CLI version. It never installs anything by itself: you get a notification and a hint in flc status.")
			},
			func(s *settings) *bool { return &s.AutoCheck }),
	}

	privacy := []field{
		{
			label: func() string { return T("Скрывать HWID", "Hide HWID") },
			help: func(f *form) string {
				switch f.s.HWID {
				case "fake":
					return T("Панель получает подменный HWID: у каждой панели свой, с этим компьютером он не связан. Лимит устройств работает, в боте видно только его.",
						"The panel gets a stand-in HWID, different for each panel and not tied to this computer. Device limits still work, the bot only shows the stand-in.")
				case "none":
					return T("HWID не передаётся совсем, а с ним и модель с системой. Панели с лимитом устройств могут отказать.",
						"No HWID is sent at all, and neither are the model and OS. Panels that limit devices may refuse.")
				}
				id := hwid()
				if id == "" {
					return T("На этой машине нет machine-id, передавать нечего.", "This machine has no machine-id, there's nothing to send.")
				}
				return fmt.Sprintf(T("Не скрывается: панель видит %s… Это хеш, сам machine-id никуда не уходит.",
					"Not hidden: the panel sees %s… It's a hash, the machine-id itself never leaves."), id[:12])
			},
			kind: kChoice,
			opts: hwidModes,
			show: func(v string) string {
				switch v {
				case "fake":
					return T("подменять", "fake it")
				case "none":
					return T("не передавать", "don't send")
				}
				return T("нет", "no")
			},
			get: func(f *form) string { return f.s.HWID },
			set: func(f *form, v string) { f.s.HWID = v },
		},
		tg(func() string { return T("Скрывать User-Agent", "Hide User-Agent") },
			func() string {
				return fmt.Sprintf(T("Вместо «%s» панель и GitHub видят обычный «%s», как у большинства clash-клиентов.",
					"Instead of %q the panel and GitHub see a plain %q, like most clash clients send."), userAgent(), plainUA)
			},
			func(s *settings) *bool { return &s.HideUA }),
		tg(func() string { return T("Скрывать устройство", "Hide device name") },
			func() string {
				return fmt.Sprintf(T("Не передавать панели модель компьютера (%s).", "Don't tell the panel the computer model (%s)."), deviceInfo().model)
			},
			func(s *settings) *bool { return &s.HideDevice }),
		tg(func() string { return T("Скрывать систему", "Hide OS") },
			func() string {
				return fmt.Sprintf(T("Не передавать панели систему и её версию (%s).", "Don't tell the panel the OS and its version (%s)."), deviceInfo().ver)
			},
			func(s *settings) *bool { return &s.HideOS }),
		tg(func() string { return T("Скрывать приложения", "Hide apps") },
			func() string {
				return T("Ядро не выясняет, какое приложение открыло соединение, и не пишет его в лог. Правила по процессам (PROCESS-NAME) перестанут работать.",
					"The core doesn't look up which app opened a connection and keeps it out of the log. PROCESS-NAME rules stop working.")
			},
			func(s *settings) *bool { return &s.HideProc }),
		tg(func() string { return T("Стирать лог", "Wipe the log") },
			func() string {
				return T("После flc stop удалять лог ядра вместе с историей соединений, а при старте не хранить лог прошлого запуска.",
					"After flc stop delete the core log with its connection history, and don't keep the previous run's log on start.")
			},
			func(s *settings) *bool { return &s.WipeLog }),
	}

	return []tab{
		{name: func() string { return T("Основные", "General") }, items: general},
		{name: func() string { return T("Обновления", "Updates") }, items: updates},
		{name: func() string { return T("Приватность", "Privacy") }, items: privacy},
	}
}

type ev struct {
	key    string
	r      rune
	mx, my int
	mb     int
}

func readEvents(b []byte) []ev {
	var out []ev
	for len(b) > 0 {
		c := b[0]
		switch {
		case c == 0x1b && len(b) > 2 && b[1] == '[' && b[2] == '<':
			end := bytes.IndexAny(b, "Mm")
			if end < 0 {
				return out
			}
			p := strings.Split(string(b[3:end]), ";")
			if len(p) == 3 && b[end] == 'M' {
				bt, _ := strconv.Atoi(p[0])
				x, _ := strconv.Atoi(p[1])
				y, _ := strconv.Atoi(p[2])
				out = append(out, ev{key: "mouse", mb: bt, mx: x, my: y})
			}
			b = b[end+1:]
		case c == 0x1b && len(b) > 2 && (b[1] == '[' || b[1] == 'O'):
			i := 2
			if b[1] == '[' {
				for i < len(b) && !(b[i] >= 0x40 && b[i] <= 0x7e) {
					i++
				}
				if i >= len(b) {
					return out
				}
			}
			switch b[i] {
			case 'A':
				out = append(out, ev{key: "up"})
			case 'B':
				out = append(out, ev{key: "down"})
			case 'C':
				out = append(out, ev{key: "right"})
			case 'D':
				out = append(out, ev{key: "left"})
			case 'Z':
				out = append(out, ev{key: "btab"})
			}
			b = b[i+1:]
		case c == 0x1b:
			out = append(out, ev{key: "esc"})
			b = b[1:]
		case c == '\r' || c == '\n':
			out = append(out, ev{key: "enter"})
			b = b[1:]
		case c == 0x7f || c == 0x08:
			out = append(out, ev{key: "bs"})
			b = b[1:]
		case c == '\t':
			out = append(out, ev{key: "tab"})
			b = b[1:]
		case c == 3:
			out = append(out, ev{key: "ctrlc"})
			b = b[1:]
		case c < 0x20:
			b = b[1:]
		default:
			r, n := utf8.DecodeRune(b)
			if r != utf8.RuneError {
				out = append(out, ev{key: "rune", r: r})
			}
			b = b[n:]
		}
	}
	return out
}

type tui struct {
	f     form
	orig  form
	tabs  []tab
	tab   int
	items []field
	cur   int
	edit  bool
	buf   []rune
	ask   bool
	msg   string
	w, h  int

	tabY  int
	tabX  [][2]int
	rowY  int
	valX  int
	btnY  int
	saveX [2]int
	quitX [2]int
	done  bool
	saved bool
}

func (u *tui) dirty() bool { return u.f != u.orig }

func (u *tui) nbtn() int { return len(u.items) + 2 }

func (u *tui) move(d int) {
	u.cur = (u.cur + d + u.nbtn()) % u.nbtn()
}

func (u *tui) setTab(i int) {
	if u.edit && !u.commit() {
		return
	}
	u.tab = (i + len(u.tabs)) % len(u.tabs)
	u.items = u.tabs[u.tab].items
	u.cur = 0
}

func (u *tui) cycle(it field, d int) {
	i := slices.Index(it.opts, it.get(&u.f))
	if i < 0 {
		i = 0
	} else {
		i = (i + d + len(it.opts)) % len(it.opts)
	}
	it.set(&u.f, it.opts[i])
}

func (u *tui) activate(d int) {
	switch {
	case u.cur == len(u.items):
		u.save()
		return
	case u.cur == len(u.items)+1:
		u.quit()
		return
	}
	it := u.items[u.cur]
	switch it.kind {
	case kToggle:
		if it.get(&u.f) == "true" {
			it.set(&u.f, "false")
		} else {
			it.set(&u.f, "true")
		}
	case kChoice:
		u.cycle(it, d)
	case kNumber:
		u.edit = true
		u.buf = []rune(it.get(&u.f))
	}
}

func (u *tui) commit() bool {
	it := u.items[u.cur]
	v := strings.TrimSpace(string(u.buf))
	n, err := strconv.Atoi(v)
	if err != nil || n < it.lo || n > it.hi || (it.ok != nil && !it.ok(n)) {
		u.msg = it.bad()
		return false
	}
	it.set(&u.f, strconv.Itoa(n))
	u.edit = false
	u.msg = ""
	return true
}

func (u *tui) save() {
	if u.edit && !u.commit() {
		return
	}
	if k := u.f.s.bad(); k != "" {
		u.msg = fmt.Sprintf(T("неверное значение %s", "bad value for %s"), k)
		return
	}
	u.saved, u.done = true, true
}

func (u *tui) quit() {
	if u.dirty() {
		u.ask = true
		return
	}
	u.done = true
}

func (u *tui) handle(e ev) {
	if u.ask {
		switch {
		case e.key == "rune" && (e.r == 'y' || e.r == 'Y' || e.r == 'н'):
			u.done = true
		case e.key == "rune" && (e.r == 's' || e.r == 'ы'):
			u.ask = false
			u.save()
		case e.key == "ctrlc":
			u.done = true
		default:
			u.ask = false
		}
		return
	}

	if e.key == "mouse" {
		u.click(e)
		return
	}

	if u.edit {
		switch e.key {
		case "rune":
			if e.r < '0' || e.r > '9' || len(u.buf) >= 5 {
				return
			}
			u.buf = append(u.buf, e.r)
		case "bs":
			if len(u.buf) > 0 {
				u.buf = u.buf[:len(u.buf)-1]
			}
		case "enter":
			u.commit()
		case "up", "down", "tab", "btab":
			if u.commit() {
				u.handle(ev{key: e.key})
			}
		case "esc":
			u.edit = false
			u.msg = ""
		case "ctrlc":
			u.done = true
		}
		return
	}

	u.msg = ""
	onField := u.cur < len(u.items)
	switch e.key {
	case "up":
		u.move(-1)
	case "down":
		u.move(1)
	case "tab":
		u.setTab(u.tab + 1)
	case "btab":
		u.setTab(u.tab - 1)
	case "left":
		if onField {
			u.activate(-1)
		} else {
			u.cur = len(u.items)
		}
	case "right":
		if onField {
			u.activate(1)
		} else {
			u.cur = len(u.items) + 1
		}
	case "enter":
		u.activate(1)
	case "esc":
		u.quit()
	case "ctrlc":
		u.done = true
	case "rune":
		switch e.r {
		case 'k', 'л':
			u.move(-1)
		case 'j', 'о':
			u.move(1)
		case 'h', 'р':
			u.handle(ev{key: "left"})
		case 'l', 'д':
			u.handle(ev{key: "right"})
		case ' ':
			u.activate(1)
		case 's', 'ы':
			u.save()
		case 'q', 'й':
			u.quit()
		case '1', '2', '3', '4', '5':
			if i := int(e.r - '1'); i < len(u.tabs) {
				u.setTab(i)
			}
		}
	}
}

func (u *tui) click(e ev) {
	switch e.mb {
	case 64:
		if !u.edit {
			u.move(-1)
		}
		return
	case 65:
		if !u.edit {
			u.move(1)
		}
		return
	case 0:
	default:
		return
	}
	if u.edit && !u.commit() {
		return
	}

	if e.my == u.tabY {
		for i, x := range u.tabX {
			if e.mx >= x[0] && e.mx <= x[1] {
				u.setTab(i)
			}
		}
		return
	}

	if e.my == u.btnY {
		switch {
		case e.mx >= u.saveX[0] && e.mx <= u.saveX[1]:
			u.cur = len(u.items)
			u.save()
		case e.mx >= u.quitX[0] && e.mx <= u.quitX[1]:
			u.cur = len(u.items) + 1
			u.quit()
		}
		return
	}

	i := e.my - u.rowY
	if i < 0 || i >= len(u.items) {
		return
	}
	u.cur = i
	d := 1
	if u.items[i].kind == kChoice && e.mx >= u.valX && e.mx <= u.valX+1 {
		d = -1
	}
	u.activate(d)
}

func fit(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) > n {
		r = append(r[:n-1], '…')
	}
	return string(r) + strings.Repeat(" ", n-len(r))
}

func wrap(s string, n int) []string {
	var out []string
	for _, par := range strings.Split(s, "\n") {
		line := ""
		for _, w := range strings.Fields(par) {
			switch {
			case line == "":
				line = w
			case utf8.RuneCountInString(line)+1+utf8.RuneCountInString(w) <= n:
				line += " " + w
			default:
				out = append(out, line)
				line = w
			}
		}
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

const (
	cRev   = "\x1b[7m"
	cBold  = "\x1b[1m"
	cDim   = "\x1b[2m"
	cRed   = "\x1b[31m"
	cYel   = "\x1b[33m"
	cCyan  = "\x1b[36m"
	cReset = "\x1b[0m"
)

func (u *tui) value(it field, editing bool) string {
	if editing {
		return string(u.buf) + "▏"
	}
	v := it.get(&u.f)
	switch it.kind {
	case kToggle:
		if v == "true" {
			return "[x] " + T("вкл", "on")
		}
		return "[ ] " + T("выкл", "off")
	case kChoice:
		return "‹ " + it.show(v) + " ›"
	}
	if it.show != nil {
		return it.show(v)
	}
	return v
}

const helpLines = 4

func (u *tui) draw() string {
	var b strings.Builder
	b.WriteString("\x1b[H\x1b[J")

	n := len(u.items)
	most := 0
	for _, t := range u.tabs {
		most = max(most, len(t.items))
	}
	need := most + 11 + helpLines
	W := min(72, u.w)
	if u.w < 48 || u.h < need {
		b.WriteString(T("Окно слишком маленькое. Растяни его или нажми q.",
			"The window is too small. Make it bigger or press q."))
		return b.String()
	}
	iw := W - 2
	x0 := (u.w-W)/2 + 1
	y0 := (u.h-need)/2 + 1
	at := func(y int) { fmt.Fprintf(&b, "\x1b[%d;%dH", y, x0) }
	row := func(y int, inner string) {
		at(y)
		b.WriteString("│" + inner + cReset + "│")
	}
	sep := func(y int) {
		at(y)
		b.WriteString("├" + strings.Repeat("─", iw) + "┤")
	}

	title := " " + T("Настройки FLClashX CLI", "FLClashX CLI settings") + " "
	if u.dirty() {
		title += "• "
	}
	at(y0)
	b.WriteString("┌─" + cBold + cCyan + title + cReset + strings.Repeat("─", max(0, iw-1-utf8.RuneCountInString(title))) + "┐")

	u.tabY = y0 + 1
	u.tabX = u.tabX[:0]
	bar, x := " ", x0+2
	for i, t := range u.tabs {
		name := " " + t.name() + " "
		w := utf8.RuneCountInString(name)
		u.tabX = append(u.tabX, [2]int{x, x + w - 1})
		if i == u.tab {
			bar += cRev + cBold + name + cReset
		} else {
			bar += name
		}
		bar += " "
		x += w + 1
	}
	row(u.tabY, bar+strings.Repeat(" ", max(0, iw-(x-x0-1))))
	sep(y0 + 2)

	lw := 24
	vw := iw - 2 - lw - 1
	u.rowY = y0 + 3
	u.valX = x0 + 1 + 2 + lw
	for i := 0; i < most; i++ {
		if i >= n {
			row(u.rowY+i, strings.Repeat(" ", iw))
			continue
		}
		it := u.items[i]
		sel := i == u.cur
		editing := sel && u.edit
		mark := "  "
		if sel {
			mark = "› "
		}
		line := mark + fit(it.label(), lw)
		val := fit(u.value(it, editing), vw) + " "
		switch {
		case editing:
			row(u.rowY+i, cRev+line+cReset+"\x1b[4m"+val)
		case sel:
			row(u.rowY+i, cRev+line+val)
		default:
			row(u.rowY+i, line+val)
		}
	}

	y := u.rowY + most
	row(y, strings.Repeat(" ", iw))
	sep(y + 1)

	help := ""
	switch {
	case u.cur < n:
		help = u.items[u.cur].help(&u.f)
	case u.cur == n:
		help = T("Сохранить настройки и выйти.", "Save the settings and leave.")
	default:
		help = T("Выйти без сохранения.", "Leave without saving.")
	}
	hl := wrap(help, iw-2)
	for i := 0; i < helpLines; i++ {
		s := ""
		if i < len(hl) {
			s = hl[i]
		}
		row(y+2+i, " "+cDim+fit(s, iw-2)+cReset+" ")
	}
	sep(y + 2 + helpLines)

	bs := "[ " + T("Сохранить", "Save") + " ]"
	bq := "[ " + T("Отмена", "Cancel") + " ]"
	gap := 4
	total := utf8.RuneCountInString(bs) + gap + utf8.RuneCountInString(bq)
	pad := (iw - total) / 2
	u.btnY = y + 3 + helpLines
	u.saveX = [2]int{x0 + 1 + pad, x0 + pad + utf8.RuneCountInString(bs)}
	u.quitX = [2]int{u.saveX[1] + 1 + gap, u.saveX[1] + gap + utf8.RuneCountInString(bq)}
	btn := func(s string, on bool) string {
		if on {
			return cRev + s + cReset
		}
		return s
	}
	row(u.btnY, strings.Repeat(" ", pad)+btn(bs, u.cur == n)+strings.Repeat(" ", gap)+
		btn(bq, u.cur == n+1)+strings.Repeat(" ", iw-pad-total))
	at(u.btnY + 1)
	b.WriteString("└" + strings.Repeat("─", iw) + "┘")

	at(u.btnY + 2)
	hint := T("Tab раздел  ↑↓ выбор  ←→ Enter изменить  s сохранить  q выход",
		"Tab section  ↑↓ move  ←→ Enter change  s save  q quit")
	if u.edit {
		hint = T("Enter готово  Esc отменить", "Enter done  Esc cancel")
	}
	b.WriteString(cDim + fit(" "+hint, W) + cReset)

	at(u.btnY + 3)
	switch {
	case u.ask:
		b.WriteString(cYel + fit(" "+T("Не сохранено: s сохранить · y выйти без сохранения · n назад",
			"Unsaved changes: s save · y leave without saving · n go back"), W) + cReset)
	case u.msg != "":
		b.WriteString(cRed + fit(" "+u.msg, W) + cReset)
	}
	return b.String()
}

func termSize() (int, int) {
	ws, err := unix.IoctlGetWinsize(int(os.Stdout.Fd()), unix.TIOCGWINSZ)
	if err != nil || ws.Col == 0 {
		return 80, 24
	}
	return int(ws.Col), int(ws.Row)
}

func rawMode(fd int) (func(), error) {
	t, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	if err != nil {
		return nil, err
	}
	old := *t
	t.Iflag &^= unix.IGNBRK | unix.BRKINT | unix.PARMRK | unix.ISTRIP | unix.INLCR | unix.IGNCR | unix.ICRNL | unix.IXON
	t.Lflag &^= unix.ECHO | unix.ECHONL | unix.ICANON | unix.ISIG | unix.IEXTEN
	t.Cflag &^= unix.CSIZE | unix.PARENB
	t.Cflag |= unix.CS8
	t.Cc[unix.VMIN] = 1
	t.Cc[unix.VTIME] = 0
	if err := unix.IoctlSetTermios(fd, unix.TCSETS, t); err != nil {
		return nil, err
	}
	return func() { _ = unix.IoctlSetTermios(fd, unix.TCSETS, &old) }, nil
}

func runTUI(f form) (form, bool, error) {
	in, out := int(os.Stdin.Fd()), int(os.Stdout.Fd())
	if _, err := unix.IoctlGetTermios(out, unix.TCGETS); err != nil {
		return f, false, errNoTTY
	}
	restore, err := rawMode(in)
	if err != nil {
		return f, false, errNoTTY
	}

	const enter = "\x1b[?1049h\x1b[?25l\x1b[?1000h\x1b[?1006h"
	const leave = "\x1b[?1006l\x1b[?1000l\x1b[?25h\x1b[?1049l"
	os.Stdout.WriteString(enter)
	defer func() {
		os.Stdout.WriteString(leave)
		restore()
	}()

	winch := make(chan os.Signal, 1)
	stop := make(chan os.Signal, 1)
	signal.Notify(winch, syscall.SIGWINCH)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGINT)
	defer signal.Stop(winch)
	defer signal.Stop(stop)

	u := &tui{f: f, orig: f, tabs: tabs()}
	u.items = u.tabs[0].items
	u.w, u.h = termSize()
	os.Stdout.WriteString(u.draw())

	buf := make([]byte, 256)
	for !u.done {
		select {
		case <-winch:
			u.w, u.h = termSize()
			os.Stdout.WriteString(u.draw())
		case <-stop:
			lang = u.orig.lang
			return u.orig, false, nil
		default:
		}

		fds := []unix.PollFd{{Fd: int32(in), Events: unix.POLLIN}}
		n, err := unix.Poll(fds, 100)
		if errors.Is(err, unix.EINTR) || n == 0 {
			continue
		}
		if err != nil {
			return u.orig, false, err
		}
		k, err := unix.Read(in, buf)
		if err != nil || k == 0 {
			continue
		}
		for _, e := range readEvents(buf[:k]) {
			u.handle(e)
			if u.done {
				break
			}
		}
		os.Stdout.WriteString(u.draw())
	}

	if !u.saved {
		lang = u.orig.lang
		return u.orig, false, nil
	}
	return u.f, true, nil
}

var errNoTTY = errors.New("no tty")

func cmdSettings(args []string) error {
	if err := noArgs("settings", args); err != nil {
		return err
	}
	s, err := loadSettings()
	if err != nil {
		return err
	}
	was := form{s: s, lang: lang}

	now, saved, err := runTUI(was)
	if errors.Is(err, errNoTTY) {
		return fmt.Errorf(T("flc settings нужен настоящий терминал. Или поправь %s вручную",
			"flc settings needs a real terminal. Or edit %s by hand"), settingsPath())
	}
	if err != nil {
		return err
	}
	if !saved {
		fmt.Println(T("ничего не изменено", "nothing changed"))
		return nil
	}

	unlock, err := lock()
	if err != nil {
		return err
	}
	defer unlock()

	if now.s != was.s {
		if err := saveSettings(now.s); err != nil {
			return err
		}
	}
	if now.lang != was.lang {
		if err := writeFile(langPath(), []byte(now.lang+"\n")); err != nil {
			return err
		}
	}
	fmt.Println(T("настройки сохранены", "settings saved"))

	if corePid() > 0 && (now.s.Interval > 0 || now.s.AutoCheck) {
		spawnWatcher()
	}
	core := now.s.Tun != was.s.Tun || now.s.Stack != was.s.Stack ||
		now.s.MixedPort != was.s.MixedPort || now.s.LogLevel != was.s.LogLevel ||
		now.s.HideProc != was.s.HideProc
	if corePid() == 0 || (!core && now.s.Mode == was.s.Mode) {
		return nil
	}

	st, err := loadStore()
	if err != nil {
		return err
	}
	p, err := st.active()
	if err != nil {
		return err
	}
	if !core {
		a := newAPI()
		if err := a.setMode(now.s.Mode); err != nil {
			return err
		}
		if now.s.Mode == "global" {
			fixGlobal(a, p)
		}
		fmt.Printf(T("режим: %s\n", "mode: %s\n"), modeName(now.s.Mode))
		return nil
	}
	if err := reloadLive(p, now.s); err == nil {
		fmt.Println(T("VPN подхватил настройки без перезапуска", "the VPN picked up the settings without a restart"))
		return nil
	}
	if !confirm(T("VPN запущен, перезапустить, чтобы применить?", "The VPN is running, restart it to apply?"), true) {
		fmt.Println(T("применится при следующем flc restart", "this applies on the next flc restart"))
		return nil
	}
	if err := restart(p, now.s); err != nil {
		return err
	}
	fmt.Println(T("VPN перезапущен", "VPN restarted"))
	return nil
}
