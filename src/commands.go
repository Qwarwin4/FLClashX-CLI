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
	markStart()
	printUp(st, p, s)
	updateHint()
	return nil
}

func printUp(st *store, p *profile, s settings) {
	fmt.Printf(T("VPN запущен, профиль «%s»\n", "VPN is up, profile %q\n"), show(p.Name))
	if v, err := load(st, p); err == nil {
		if cur := v.g.current(v.g.root(v.match, s.Mode)); cur != "" {
			fmt.Printf(T("сервер: %s\n", "server: %s\n"), show(cur))
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
	markStart()
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

	stopWatcher()
	ok, err := stopCore()
	if err != nil {
		return err
	}
	clearStart()
	if s, _ := readSettings(); s.WipeLog {
		wipeLogs()
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
			fmt.Printf(T("профиль: %s\n", "profile: %s\n"), show(p.Name))
		}
		return nil
	}

	fmt.Printf(T("VPN запущен (pid %d)\n", "VPN is running (pid %d)\n"), pid)
	var rows [][2]string
	defer func() {
		w := 0
		for _, r := range rows {
			w = max(w, cells(r[0]))
		}
		for _, r := range rows {
			fmt.Println(r[0] + ":" + strings.Repeat(" ", w-cells(r[0])+1) + r[1])
		}
		updateHint()
	}()
	if t, ok := upSince(pid); ok {
		rows = append(rows, [2]string{T("в работе", "uptime"), uptime(t)})
	}
	if p == nil {
		return nil
	}
	rows = append(rows, [2]string{T("профиль", "profile"), show(p.Name)})
	a := newAPI()
	c, err := a.configs()
	if err != nil {
		return fmt.Errorf(T("ядро не отвечает: %w", "core isn't responding: %w"), err)
	}
	v, err := load(st, p)
	if err == nil {
		if cur := v.g.current(v.g.root(v.match, c.Mode)); cur != "" {
			rows = append(rows, [2]string{T("сервер", "server"), show(cur)})
		}
	}
	tun := T("выкл", "off")
	if c.Tun.Enable {
		tun = T("вкл", "on")
	}
	rows = append(rows, [2]string{T("режим", "mode"),
		fmt.Sprintf(T("%s, TUN %s, прокси 127.0.0.1:%d", "%s, TUN %s, proxy 127.0.0.1:%d"), modeName(c.Mode), tun, c.MixedPort)})
	if s, err := readSettings(); err == nil && s.Interval > 0 && p.URL != "" {
		next := time.Until(p.Updated.Add(time.Duration(s.Interval) * time.Minute)).Round(time.Minute)
		rows = append(rows, [2]string{T("подписка", "subscription"),
			fmt.Sprintf(T("обновлена %s, следующая через %d мин", "updated %s, next in %d min"), p.Updated.Local().Format("15:04"), max(0, int(next.Minutes())))})
	}
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
	var t table
	for i, n := range list {
		mark := " "
		if n == cur {
			mark = "*"
		}
		d := ""
		if ms := v.g.delay(n); ms > 0 {
			d = fmt.Sprintf("%d ms", ms)
		}
		t.add(fmt.Sprintf("%s %d", mark, i+1), show(n), strings.ToLower(clean(v.g[n].Type)), d)
	}
	t.print()
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
		return fmt.Errorf(T("«%s» сидит в автоматической группе, вручную его не выбрать", "%q is in an automatic group and can't be picked by hand"), show(name))
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
		fmt.Printf(T("сервер: %s (применится при flc start)\n", "server: %s (applies on flc start)\n"), show(name))
		return nil
	}

	fmt.Printf(T("сервер: %s, перезапускаю VPN...\n", "server: %s, restarting the VPN...\n"), show(name))
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

	t := table{strings.Split(T("  #\tИМЯ\tСЕРВЕРОВ\tТРАФИК\tДО\tОБНОВЛЁН", "  #\tNAME\tSERVERS\tTRAFFIC\tEXPIRES\tUPDATED"), "\t")}
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
		t.add(fmt.Sprintf("%s %d", mark, i+1), show(p.Name), n,
			p.Info.String(), expires(p.Info), p.Updated.Local().Format("2006-01-02 15:04"))
	}
	t.print()
	return nil
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
		fmt.Printf(T("«%s» и так активен\n", "%q is already active\n"), show(p.Name))
		return nil
	}
	st.Current = p.ID
	if err := st.save(); err != nil {
		return err
	}
	fmt.Printf(T("активный профиль: %s\n", "active profile: %s\n"), show(p.Name))

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

	if *link != "" {
		st, err := loadStore()
		if err != nil {
			return err
		}
		if err := dupLink(st, *link); err != nil {
			return err
		}
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
	if p.URL != "" {
		if err := dupLink(st, p.URL); err != nil {
			return err
		}
	}
	if *name != "" {
		if p.Name, err = checkName(*name); err != nil {
			return err
		}
		if st.byName(p.Name, nil) != nil {
			return fmt.Errorf(T("профиль «%s» уже есть", "profile %q already exists"), show(p.Name))
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
	fmt.Printf(T("добавлен профиль «%s», серверов: %d\n", "added profile %q, %d servers\n"), show(p.Name), n)
	if first {
		fmt.Println(T("он стал активным, запускай: flc start", "it's now active, go ahead: flc start"))
	}
	return nil
}

func cmdUpdateProfile(args []string) error {
	fs := newFlags("update-profile")
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
		return fmt.Errorf(T("«%s» добавлен из файла, обновлять нечего", "%q was added from a file, nothing to update"), show(p.Name))
	}
	s, err := loadSettings()
	if err != nil {
		return err
	}

	fmt.Println(T("качаю подписку...", "fetching subscription..."))
	cfg, sb, note, err := fetchProfile(p, s, *allowHTTP || strings.HasPrefix(p.URL, "http://"))
	if err != nil {
		return err
	}

	unlock, err := lock()
	if err != nil {
		return err
	}
	defer unlock()
	if st, p, err = saveProfile(p.ID, cfg, sb.info); err != nil {
		return err
	}
	if note != "" {
		fmt.Println(note)
	}
	fmt.Printf(T("профиль «%s» обновлён\n", "profile %q updated\n"), show(p.Name))

	if st.Current == p.ID && corePid() > 0 {
		if err := reloadLive(p, s); err != nil {
			fmt.Println(T("на лету применить не вышло, перезапускаю VPN...", "couldn't apply it live, restarting the VPN..."))
			return restart(p, s)
		}
		fmt.Println(T("VPN подхватил новую подписку без перезапуска", "the VPN picked up the new subscription without a restart"))
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
	fmt.Printf(T("«%s» → «%s»\n", "%q → %q\n"), show(old), show(n))
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

func editYAML(orig []byte, tmpDir string, needMap bool) ([]byte, error) {
	tmp, err := os.CreateTemp(tmpDir, ".edit-*.yaml")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmp.Name())
	_, err = tmp.Write(orig)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return nil, err
	}

	ed := editor()
	for {
		c := exec.Command(ed[0], append(ed[1:], tmp.Name())...)
		c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
		if err := c.Run(); err != nil {
			return nil, fmt.Errorf(T("редактор: %w", "editor: %w"), err)
		}
		b, err := os.ReadFile(tmp.Name())
		if err != nil {
			return nil, err
		}
		if bytes.Equal(b, orig) {
			fmt.Println(T("изменений нет", "no changes"))
			return nil, nil
		}
		var m map[string]any
		err = yaml.Unmarshal(b, &m)
		if err == nil && m == nil && needMap {
			err = errors.New(T("файл пустой", "file is empty"))
		}
		if err == nil {
			return b, nil
		}
		fmt.Fprintf(os.Stderr, T("в конфиге ошибка: %v\n", "config has an error: %v\n"), err)
		if !confirm(T("открыть снова?", "open it again?"), true) {
			fmt.Println(T("изменения отброшены", "changes discarded"))
			return nil, nil
		}
	}
}

func cmdConfig(args []string) error {
	fs := newFlags("config")
	which := fs.String("profile", "", "")
	core := fs.Bool("core", false, "")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if *core {
		if *which != "" || len(pos) > 0 {
			return errors.New(T("либо --core, либо --profile", "use either --core or --profile"))
		}
		msg, err := editCore()
		if msg != "" {
			fmt.Println(msg)
		}
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
	b, err := editYAML(orig, profDir(), true)
	if err != nil || b == nil {
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
	still := st.byID(p.ID)
	if still == nil {
		return errors.New(T("пока ты редактировал, профиль удалили", "the profile was deleted while you were editing"))
	}
	if err := writeFile(p.path(), b); err != nil {
		return err
	}
	fmt.Println(T("сохранено", "saved"))

	if st.Current == p.ID && corePid() > 0 {
		return applyLive(still)
	}
	return nil
}

func dupLink(st *store, link string) error {
	if p := st.byLink(link); p != nil {
		return fmt.Errorf(T("эта подписка уже добавлена как «%s». Обновить её: flc update-profile %s",
			"this subscription is already added as %q. To refresh it: flc update-profile %s"), show(p.Name), show(p.Name))
	}
	return nil
}

func cmdDeleteProfile(args []string) error {
	fs := newFlags("delete-profile")
	yes := fs.Bool("y", false, "")
	all := fs.Bool("all", false, "")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if *all && len(pos) > 0 {
		return errors.New(T("либо имя профиля, либо --all", "give either a profile name or --all"))
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
	if *all {
		return deleteAll(st, *yes)
	}
	p, err := st.find(strings.Join(pos, " "))
	if err != nil {
		return err
	}
	if st.Current == p.ID && corePid() > 0 {
		return errors.New(T("этот профиль сейчас используется, сначала flc stop", "this profile is in use, run flc stop first"))
	}
	if !*yes && !confirm(fmt.Sprintf(T("удалить профиль «%s»?", "delete profile %q?"), show(p.Name)), false) {
		return nil
	}

	st.remove(p)
	if err := st.save(); err != nil {
		return err
	}
	if err := os.Remove(p.path()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	fmt.Printf(T("профиль «%s» удалён\n", "profile %q deleted\n"), show(p.Name))
	if c := st.cur(); c != nil && c != p {
		fmt.Printf(T("активный теперь: %s\n", "active now: %s\n"), show(c.Name))
	}
	return nil
}

func deleteAll(st *store, yes bool) error {
	n := len(st.Profiles)
	if n == 0 {
		fmt.Println(T("профилей и так нет", "there are no profiles"))
		return nil
	}
	if corePid() > 0 {
		return errors.New(T("VPN запущен, сначала flc stop", "the VPN is running, run flc stop first"))
	}
	if !yes && !confirm(fmt.Sprintf(T("удалить все профили (%d)?", "delete all %d profiles?"), n), false) {
		return nil
	}

	ps := st.Profiles
	st.Profiles, st.Current = nil, ""
	if err := st.save(); err != nil {
		return err
	}
	var bad []string
	for _, p := range ps {
		if err := os.Remove(p.path()); err != nil && !errors.Is(err, os.ErrNotExist) {
			bad = append(bad, show(p.Name))
		}
	}
	if len(bad) > 0 {
		return fmt.Errorf(T("не удалось удалить файлы профилей: %s", "couldn't delete profile files: %s"), strings.Join(bad, ", "))
	}
	fmt.Printf(T("удалено профилей: %d\n", "deleted %d profiles\n"), n)
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
