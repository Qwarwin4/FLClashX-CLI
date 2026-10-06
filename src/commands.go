package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"go.yaml.in/yaml/v3"
)

func noArgs(name string, args []string) error {
	pos, err := parse(newFlags(name), args)
	if err != nil {
		return err
	}
	if len(pos) > 0 {
		return fmt.Errorf(T("%s не принимает аргументов", "%s takes no arguments"), name)
	}
	return nil
}

func cmdStart(args []string) error {
	if err := noArgs("start", args); err != nil {
		return err
	}
	unlock, err := lock()
	if err != nil {
		return err
	}
	defer unlock()

	if corePid() > 0 {
		return errors.New(T("VPN уже запущен", "VPN is already running"))
	}
	st, err := loadStore()
	if err != nil {
		return err
	}
	p, err := st.active()
	if err != nil {
		return err
	}
	s, err := loadSettings()
	if err != nil {
		return err
	}
	if err := startCore(p, s); err != nil {
		return err
	}
	printUp(st, p, s)
	return nil
}

func printUp(st *store, p *profile, s settings) {
	fmt.Printf(T("VPN запущен, профиль «%s»\n", "VPN is up, profile %q\n"), clean(p.Name))
	if v, err := load(st, p); err == nil {
		if cur := v.g.current(v.g.root(v.match, s.Mode)); cur != "" {
			fmt.Printf(T("сервер: %s\n", "server: %s\n"), clean(cur))
		}
	}
}

func cmdRestart(args []string) error {
	if err := noArgs("restart", args); err != nil {
		return err
	}
	unlock, err := lock()
	if err != nil {
		return err
	}
	defer unlock()

	st, err := loadStore()
	if err != nil {
		return err
	}
	p, err := st.active()
	if err != nil {
		return err
	}
	s, err := loadSettings()
	if err != nil {
		return err
	}
	if corePid() > 0 {
		fmt.Println(T("перезапускаю VPN...", "restarting the VPN..."))
	}
	if err := restart(p, s); err != nil {
		return err
	}
	printUp(st, p, s)
	return nil
}

func cmdStop(args []string) error {
	if err := noArgs("stop", args); err != nil {
		return err
	}
	unlock, err := lock()
	if err != nil {
		return err
	}
	defer unlock()

	ok, err := stopCore()
	if err != nil {
		return err
	}
	if ok {
		fmt.Println(T("VPN остановлен", "VPN stopped"))
	} else {
		fmt.Println(T("VPN и так не запущен", "VPN isn't running"))
	}
	return nil
}

func cmdStatus(args []string) error {
	if err := noArgs("status", args); err != nil {
		return err
	}
	st, err := loadStore()
	if err != nil {
		return err
	}
	p := st.cur()
	pid := corePid()
	if pid == 0 {
		fmt.Println(T("VPN не запущен", "VPN is not running"))
		if p != nil {
			fmt.Printf(T("профиль: %s\n", "profile: %s\n"), clean(p.Name))
		}
		return nil
	}

	fmt.Printf(T("VPN запущен (pid %d)\n", "VPN is running (pid %d)\n"), pid)
	if p == nil {
		return nil
	}
	fmt.Printf(T("профиль: %s\n", "profile: %s\n"), clean(p.Name))
	a := newAPI()
	c, err := a.configs()
	if err != nil {
		return fmt.Errorf(T("ядро не отвечает: %w", "core isn't responding: %w"), err)
	}
	v, err := load(st, p)
	if err == nil {
		if cur := v.g.current(v.g.root(v.match, c.Mode)); cur != "" {
			fmt.Printf(T("сервер:  %s\n", "server:  %s\n"), clean(cur))
		}
	}
	tun := T("выкл", "off")
	if c.Tun.Enable {
		tun = T("вкл", "on")
	}
	fmt.Printf(T("режим:   %s, TUN %s, прокси 127.0.0.1:%d\n", "mode:    %s, TUN %s, proxy 127.0.0.1:%d\n"), modeName(c.Mode), tun, c.MixedPort)
	return nil
}

type view struct {
	g       graph
	match   string
	live    bool
	partial bool
}

func load(st *store, p *profile) (*view, error) {
	if st.Current == p.ID && corePid() > 0 {
		a := newAPI()
		if g, err := a.graph(); err == nil {
			return &view{g: g, match: a.matchTarget(), live: true}, nil
		}
	}
	g, m, prov, err := fileGraph(p)
	if err != nil {
		return nil, err
	}
	return &view{g: g, match: m, partial: prov}, nil
}

func cmdServerList(args []string) error {
	if err := noArgs("server-list", args); err != nil {
		return err
	}
	st, err := loadStore()
	if err != nil {
		return err
	}
	p, err := st.active()
	if err != nil {
		return err
	}
	s, err := loadSettings()
	if err != nil {
		return err
	}
	v, err := load(st, p)
	if err != nil {
		return err
	}

	root := v.g.root(v.match, s.Mode)
	list := v.g.servers(root)
	cur := v.g.current(root)
	if len(list) == 0 {
		fmt.Println(T("серверов нет", "no servers"))
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for i, n := range list {
		mark := " "
		if n == cur {
			mark = "*"
		}
		d := ""
		if ms := v.g.delay(n); ms > 0 {
			d = fmt.Sprintf("%d ms", ms)
		}
		fmt.Fprintf(tw, "%s %d\t%s\t%s\t%s\n", mark, i+1, clean(n), strings.ToLower(clean(v.g[n].Type)), d)
	}
	tw.Flush()
	if v.partial {
		fmt.Println(T("\nчасть серверов приходит из proxy-providers и появится после flc start",
			"\nsome servers come from proxy-providers and will show up after flc start"))
	}
	return nil
}

func cmdSwitchServer(args []string) error {
	pos, err := parse(newFlags("switch-server"), args)
	if err != nil {
		return err
	}
	q := strings.Join(pos, " ")
	if q == "" {
		return errors.New(T("укажи сервер: flc switch-server <имя или номер>", "which server? flc switch-server <name or number>"))
	}
	unlock, err := lock()
	if err != nil {
		return err
	}
	defer unlock()

	st, err := loadStore()
	if err != nil {
		return err
	}
	p, err := st.active()
	if err != nil {
		return err
	}
	s, err := loadSettings()
	if err != nil {
		return err
	}
	v, err := load(st, p)
	if err != nil {
		return err
	}

	root := v.g.root(v.match, s.Mode)
	name, err := pick(v.g.servers(root), q)
	if err != nil {
		return err
	}
	steps := v.g.route(root, name)
	if len(steps) == 0 {
		return fmt.Errorf(T("«%s» сидит в автоматической группе, вручную его не выбрать", "%q is in an automatic group and can't be picked by hand"), clean(name))
	}

	if p.Selected == nil {
		p.Selected = map[string]string{}
	}
	for _, x := range steps {
		p.Selected[x[0]] = x[1]
	}
	if err := st.save(); err != nil {
		return err
	}
	if !v.live {
		fmt.Printf(T("сервер: %s (применится при flc start)\n", "server: %s (applies on flc start)\n"), clean(name))
		return nil
	}

	fmt.Printf(T("сервер: %s, перезапускаю VPN...\n", "server: %s, restarting the VPN...\n"), clean(name))
	if err := restart(p, s); err != nil {
		return err
	}
	fmt.Println(T("готово", "done"))
	return nil
}

func cmdProfileList(args []string) error {
	if err := noArgs("profile-list", args); err != nil {
		return err
	}
	st, err := loadStore()
	if err != nil {
		return err
	}
	if len(st.Profiles) == 0 {
		fmt.Println(T("профилей нет, добавь: flc add-profile --link <ссылка>", "no profiles yet, add one: flc add-profile --link <url>"))
		return nil
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, T("  #\tИМЯ\tСЕРВЕРОВ\tТРАФИК\tДО\tОБНОВЛЁН", "  #\tNAME\tSERVERS\tTRAFFIC\tEXPIRES\tUPDATED"))
	for i, p := range st.Profiles {
		mark := " "
		if p.ID == st.Current {
			mark = "*"
		}
		n := "?"
		if g, _, prov, err := fileGraph(p); err == nil {
			n = fmt.Sprint(len(g.servers("GLOBAL")))
			if prov {
				n += "+"
			}
		}
		fmt.Fprintf(tw, "%s %d\t%s\t%s\t%s\t%s\t%s\n", mark, i+1, clean(p.Name), n,
			p.Info, expires(p.Info), p.Updated.Local().Format("2006-01-02 15:04"))
	}
	return tw.Flush()
}

func cmdSwitchProfile(args []string) error {
	pos, err := parse(newFlags("switch-profile"), args)
	if err != nil {
		return err
	}
	unlock, err := lock()
	if err != nil {
		return err
	}
	defer unlock()

	st, err := loadStore()
	if err != nil {
		return err
	}
	p, err := st.find(strings.Join(pos, " "))
	if err != nil {
		return err
	}
	if p.ID == st.Current {
		fmt.Printf(T("«%s» и так активен\n", "%q is already active\n"), clean(p.Name))
		return nil
	}
	st.Current = p.ID
	if err := st.save(); err != nil {
		return err
	}
	fmt.Printf(T("активный профиль: %s\n", "active profile: %s\n"), clean(p.Name))

	if corePid() == 0 {
		return nil
	}
	s, err := loadSettings()
	if err != nil {
		return err
	}
	fmt.Println(T("перезапускаю VPN...", "restarting the VPN..."))
	return restart(p, s)
}

func cmdAddProfile(args []string) error {
	fs := newFlags("add-profile")
	link := fs.String("link", "", "")
	file := fs.String("file", "", "")
	name := fs.String("name", "", "")
	allowHTTP := fs.Bool("allow-http", false, "")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) > 0 || (*link == "") == (*file == "") {
		return errors.New(T("нужно ровно одно: --link <ссылка> или --file <путь>", "need exactly one of --link <url> or --file <path>"))
	}

	s, err := loadSettings()
	if err != nil {
		return err
	}
	p := &profile{ID: newID(), Updated: time.Now()}
	var raw []byte
	if *link != "" {
		fmt.Println(T("качаю подписку...", "fetching subscription..."))
		sb, err := download(*link, s, *allowHTTP)
		if err != nil {
			return err
		}
		raw, p.Name, p.Info, p.URL = sb.body, sb.name, sb.info, strings.TrimSpace(*link)
	} else {
		if raw, err = readLocal(*file); err != nil {
			return err
		}
		base := filepath.Base(*file)
		p.Name = tidyName(strings.TrimSuffix(base, filepath.Ext(base)))
	}
	cfg, err := normalize(raw)
	if err != nil {
		return err
	}

	unlock, err := lock()
	if err != nil {
		return err
	}
	defer unlock()
	st, err := loadStore()
	if err != nil {
		return err
	}
	if *name != "" {
		if p.Name, err = checkName(*name); err != nil {
			return err
		}
		if st.byName(p.Name, nil) != nil {
			return fmt.Errorf(T("профиль «%s» уже есть", "profile %q already exists"), clean(p.Name))
		}
	} else {
		if p.Name == "" {
			p.Name = "profile"
		}
		p.Name = st.uniq(p.Name)
	}

	if err := writeFile(p.path(), cfg); err != nil {
		return err
	}
	st.Profiles = append(st.Profiles, p)
	first := st.Current == ""
	if first {
		st.Current = p.ID
	}
	if err := st.save(); err != nil {
		os.Remove(p.path())
		return err
	}

	n := 0
	if g, _, _, err := fileGraph(p); err == nil {
		n = len(g.servers("GLOBAL"))
	}
	fmt.Printf(T("добавлен профиль «%s», серверов: %d\n", "added profile %q, %d servers\n"), clean(p.Name), n)
	if first {
		fmt.Println(T("он стал активным, запускай: flc start", "it's now active, go ahead: flc start"))
	}
	return nil
}

func cmdUpdate(args []string) error {
	fs := newFlags("update")
	allowHTTP := fs.Bool("allow-http", false, "")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	st, err := loadStore()
	if err != nil {
		return err
	}
	var p *profile
	if len(pos) > 0 {
		p, err = st.find(strings.Join(pos, " "))
	} else {
		p, err = st.active()
	}
	if err != nil {
		return err
	}
	if p.URL == "" {
		return fmt.Errorf(T("«%s» добавлен из файла, обновлять нечего", "%q was added from a file, nothing to update"), clean(p.Name))
	}
	s, err := loadSettings()
	if err != nil {
		return err
	}

	fmt.Println(T("качаю подписку...", "fetching subscription..."))
	sb, err := download(p.URL, s, *allowHTTP || strings.HasPrefix(p.URL, "http://"))
	if err != nil {
		return err
	}
	cfg, err := normalize(sb.body)
	if err != nil {
		return err
	}

	unlock, err := lock()
	if err != nil {
		return err
	}
	defer unlock()
	if st, err = loadStore(); err != nil {
		return err
	}
	if p = st.byID(p.ID); p == nil {
		return errors.New(T("пока качалась подписка, профиль удалили", "the profile was deleted while the subscription was downloading"))
	}
	if err := writeFile(p.path(), cfg); err != nil {
		return err
	}
	p.Updated, p.Info = time.Now(), sb.info
	if err := st.save(); err != nil {
		return err
	}
	fmt.Printf(T("профиль «%s» обновлён\n", "profile %q updated\n"), clean(p.Name))

	if st.Current == p.ID && corePid() > 0 {
		fmt.Println(T("перезапускаю VPN...", "restarting the VPN..."))
		return restart(p, s)
	}
	return nil
}

func cmdChangeName(args []string) error {
	pos, err := parse(newFlags("change-name"), args)
	if err != nil {
		return err
	}
	n, err := checkName(strings.Join(pos, " "))
	if err != nil {
		return err
	}
	unlock, err := lock()
	if err != nil {
		return err
	}
	defer unlock()

	st, err := loadStore()
	if err != nil {
		return err
	}
	p, err := st.active()
	if err != nil {
		return err
	}
	if st.byName(n, p) != nil {
		return fmt.Errorf(T("профиль «%s» уже есть", "profile %q already exists"), n)
	}
	old := p.Name
	p.Name = n
	if err := st.save(); err != nil {
		return err
	}
	fmt.Printf(T("«%s» → «%s»\n", "%q → %q\n"), clean(old), n)
	return nil
}

func editor() []string {
	if p, err := exec.LookPath("nano"); err == nil {
		return []string{p}
	}
	for _, env := range []string{"VISUAL", "EDITOR"} {
		if f := strings.Fields(os.Getenv(env)); len(f) > 0 {
			return f
		}
	}
	return []string{"vi"}
}

func cmdConfig(args []string) error {
	fs := newFlags("config")
	which := fs.String("profile", "", "")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) > 0 {
		return errors.New(T("профиль задаётся флагом: flc config --profile <имя>", "pick the profile with a flag: flc config --profile <name>"))
	}
	st, err := loadStore()
	if err != nil {
		return err
	}
	var p *profile
	if *which != "" {
		p, err = st.find(*which)
	} else {
		p, err = st.active()
	}
	if err != nil {
		return err
	}

	orig, err := os.ReadFile(p.path())
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(profDir(), ".edit-*.yaml")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	_, err = tmp.Write(orig)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}

	ed := editor()
	var b []byte
	for {
		c := exec.Command(ed[0], append(ed[1:], tmp.Name())...)
		c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
		if err := c.Run(); err != nil {
			return fmt.Errorf(T("редактор: %w", "editor: %w"), err)
		}
		if b, err = os.ReadFile(tmp.Name()); err != nil {
			return err
		}
		if bytes.Equal(b, orig) {
			fmt.Println(T("изменений нет", "no changes"))
			return nil
		}
		var m map[string]any
		err = yaml.Unmarshal(b, &m)
		if err == nil && m == nil {
			err = errors.New(T("файл пустой", "file is empty"))
		}
		if err == nil {
			break
		}
		fmt.Fprintf(os.Stderr, T("в конфиге ошибка: %v\n", "config has an error: %v\n"), err)
		if !confirm(T("открыть снова?", "open it again?"), true) {
			fmt.Println(T("изменения отброшены", "changes discarded"))
			return nil
		}
	}

	unlock, err := lock()
	if err != nil {
		return err
	}
	defer unlock()
	if st, err = loadStore(); err != nil {
		return err
	}
	still := st.byID(p.ID)
	if still == nil {
		return errors.New(T("пока ты редактировал, профиль удалили", "the profile was deleted while you were editing"))
	}
	if err := writeFile(p.path(), b); err != nil {
		return err
	}
	fmt.Println(T("сохранено", "saved"))

	if st.Current == p.ID && corePid() > 0 && confirm(T("VPN запущен с этим профилем, перезапустить?", "VPN is running with this profile, restart it?"), true) {
		s, err := loadSettings()
		if err != nil {
			return err
		}
		return restart(still, s)
	}
	return nil
}

func cmdDeleteProfile(args []string) error {
	fs := newFlags("delete-profile")
	yes := fs.Bool("y", false, "")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	unlock, err := lock()
	if err != nil {
		return err
	}
	defer unlock()

	st, err := loadStore()
	if err != nil {
		return err
	}
	p, err := st.find(strings.Join(pos, " "))
	if err != nil {
		return err
	}
	if st.Current == p.ID && corePid() > 0 {
		return errors.New(T("этот профиль сейчас используется, сначала flc stop", "this profile is in use, run flc stop first"))
	}
	if !*yes && !confirm(fmt.Sprintf(T("удалить профиль «%s»?", "delete profile %q?"), clean(p.Name)), false) {
		return nil
	}

	st.remove(p)
	if err := st.save(); err != nil {
		return err
	}
	if err := os.Remove(p.path()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	fmt.Printf(T("профиль «%s» удалён\n", "profile %q deleted\n"), clean(p.Name))
	if c := st.cur(); c != nil && c != p {
		fmt.Printf(T("активный теперь: %s\n", "active now: %s\n"), clean(c.Name))
	}
	return nil
}

func cmdDebug(args []string) error {
	if len(args) == 0 || args[0] != "log" {
		return errors.New(T("использование: flc debug log [-n 200] [-f]", "usage: flc debug log [-n 200] [-f]"))
	}
	fs := newFlags("debug log")
	n := fs.Int("n", 200, "")
	follow := fs.Bool("f", false, "")
	if _, err := parse(fs, args[1:]); err != nil {
		return err
	}

	f, err := os.Open(logPath())
	if errors.Is(err, os.ErrNotExist) {
		fmt.Println(T("лог пуст, ядро ещё не запускалось", "log is empty, the core hasn't run yet"))
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()

	const window = 512 << 10
	fi, err := f.Stat()
	if err != nil {
		return err
	}
	off := max(fi.Size()-window, 0)
	buf := make([]byte, fi.Size()-off)
	if _, err := f.ReadAt(buf, off); err != nil && err != io.EOF {
		return err
	}
	lines := strings.Split(strings.TrimRight(string(buf), "\n"), "\n")
	if off > 0 && len(lines) > 0 {
		lines = lines[1:]
	}
	if *n > 0 && len(lines) > *n {
		lines = lines[len(lines)-*n:]
	}
	for _, ln := range lines {
		fmt.Println(clean(ln))
	}
	if !*follow {
		return nil
	}

	pos := fi.Size()
	var rest string
	for {
		time.Sleep(500 * time.Millisecond)
		fi, err := os.Stat(logPath())
		if err != nil {
			continue
		}
		if fi.Size() < pos || !os.SameFile(fi, mustStat(f)) {
			f.Close()
			if f, err = os.Open(logPath()); err != nil {
				return err
			}
			pos, rest = 0, ""
		}
		if fi.Size() == pos {
			continue
		}
		buf := make([]byte, min(fi.Size()-pos, window))
		k, _ := f.ReadAt(buf, pos)
		pos += int64(k)
		chunk := rest + string(buf[:k])
		i := strings.LastIndexByte(chunk, '\n')
		if i < 0 {
			rest = chunk
			continue
		}
		for _, ln := range strings.Split(chunk[:i], "\n") {
			fmt.Println(clean(ln))
		}
		rest = chunk[i+1:]
	}
}

func mustStat(f *os.File) os.FileInfo {
	fi, err := f.Stat()
	if err != nil {
		return nil
	}
	return fi
}
