package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const watchArg = "__watch"

func watchLock() string { return filepath.Join(dir.run, "watch.lock") }
func watchLog() string  { return filepath.Join(dir.state, "watch.log") }
func checkPath() string { return filepath.Join(dir.state, "update.json") }

func spawnWatcher() {
	if watcherPid() > 0 {
		return
	}
	exe, err := self()
	if err != nil {
		return
	}
	c := exec.Command(exe, watchArg)
	c.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if c.Start() == nil {
		_ = c.Process.Release()
	}
}

func watcherPid() int {
	b, err := os.ReadFile(watchLock())
	if err != nil {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || pid <= 1 {
		return 0
	}
	cl, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil {
		return 0
	}
	args := strings.Split(strings.TrimRight(string(cl), "\x00"), "\x00")
	if len(args) != 2 || args[1] != watchArg {
		return 0
	}
	fi, err := os.Stat(fmt.Sprintf("/proc/%d", pid))
	if st, ok := fi.Sys().(*syscall.Stat_t); err != nil || !ok || int(st.Uid) != os.Getuid() {
		return 0
	}
	return pid
}

func stopWatcher() {
	if pid := watcherPid(); pid > 0 {
		_ = syscall.Kill(pid, syscall.SIGTERM)
	}
}

func cmdWatch(args []string) error {
	if len(args) > 0 {
		return errors.New("usage: flc " + watchArg)
	}
	f, err := os.OpenFile(watchLock(), os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		return nil
	}
	_ = f.Truncate(0)
	_, _ = f.WriteAt([]byte(strconv.Itoa(os.Getpid())+"\n"), 0)

	if fi, err := os.Stat(watchLog()); err == nil && fi.Size() > 1<<20 {
		_ = os.Remove(watchLog())
	}
	lf, err := os.OpenFile(watchLog(), os.O_CREATE|os.O_WRONLY|os.O_APPEND|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return err
	}
	defer lf.Close()
	logf := func(format string, a ...any) {
		fmt.Fprintf(lf, "%s %s\n", time.Now().Format(time.DateTime), fmt.Sprintf(format, a...))
	}
	os.Stdout, os.Stderr = lf, lf

	failed := map[string]time.Time{}
	miss := 0
	wait := 5 * time.Second
	for {
		time.Sleep(wait)
		wait = time.Minute
		if corePid() == 0 {
			if miss++; miss >= 2 {
				return nil
			}
			continue
		}
		miss = 0
		s, err := loadSettings()
		if err != nil {
			logf("%v", err)
			continue
		}
		if s.Interval > 0 {
			if err := subTick(s, failed, logf); err != nil {
				logf(T("подписка: %v", "subscription: %v"), err)
			}
		}
		if s.AutoCheck {
			updTick(logf)
		}
	}
}

func subTick(s settings, failed map[string]time.Time, logf func(string, ...any)) error {
	st, err := loadStore()
	if err != nil {
		return err
	}
	p := st.cur()
	if p == nil || p.URL == "" {
		return nil
	}
	every := time.Duration(s.Interval) * time.Minute
	if time.Since(p.Updated) < every {
		return nil
	}
	if t, ok := failed[p.ID]; ok && time.Since(t) < min(every, 15*time.Minute) {
		return nil
	}

	cfg, sb, note, err := fetchProfile(p, s, strings.HasPrefix(p.URL, "http://"))
	if err != nil {
		failed[p.ID] = time.Now()
		return err
	}
	delete(failed, p.ID)

	unlock, err := lock()
	if err != nil {
		return err
	}
	defer unlock()
	st, p, changed, err := saveProfile(p.ID, cfg, sb.info)
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}
	if note != "" {
		logf("%s", note)
	}
	logf(T("профиль «%s» обновлён", "profile %q updated"), clean(p.Name))
	if st.Current != p.ID || corePid() == 0 {
		return nil
	}
	if err := reloadLive(p, s); err != nil {
		logf(T("на лету не применилось (%v), перезапускаю", "couldn't apply live (%v), restarting"), err)
		return restart(p, s)
	}
	return nil
}

type checkState struct {
	Checked  int64  `json:"checked"`
	Tag      string `json:"tag"`
	Notified string `json:"notified"`
}

func readCheck() checkState {
	var c checkState
	if b, err := os.ReadFile(checkPath()); err == nil {
		_ = json.Unmarshal(b, &c)
	}
	if !tagRe.MatchString(c.Tag) {
		c.Tag = ""
	}
	return c
}

func updTick(logf func(string, ...any)) {
	c := readCheck()
	if time.Since(time.Unix(c.Checked, 0)) < 24*time.Hour {
		return
	}
	c.Checked = time.Now().Unix()
	r, err := latest(ghClient())
	if err != nil {
		logf("%v", err)
	} else {
		c.Tag = r.Tag
	}
	cur, _ := semver(version)
	if c.Tag != "" && newer(semverOr0(c.Tag), cur) && c.Notified != c.Tag {
		logf(T("вышла версия %s", "version %s is out"), c.Tag)
		if notify(c.Tag) {
			c.Notified = c.Tag
		}
	}
	if b, err := json.Marshal(c); err == nil {
		_ = writeFile(checkPath(), b)
	}
}

func notify(tag string) bool {
	if os.Getenv("WAYLAND_DISPLAY") == "" && os.Getenv("DISPLAY") == "" {
		return false
	}
	bin, err := exec.LookPath("notify-send")
	if err != nil {
		return false
	}
	c := exec.Command(bin, "-a", "FLClashX CLI", "FLClashX CLI "+tag,
		T("Вышла новая версия. Обновить: flc check-updates", "A new version is out. Update with: flc check-updates"))
	c.Stdout, c.Stderr = io.Discard, io.Discard
	return c.Run() == nil
}

func updateHint() {
	s, err := readSettings()
	if err != nil || !s.AutoCheck {
		return
	}
	c := readCheck()
	cur, _ := semver(version)
	if c.Tag != "" && newer(semverOr0(c.Tag), cur) {
		fmt.Fprintf(os.Stderr, T("\nвышла версия %s, обновить: flc check-updates\n", "\nversion %s is out, update with: flc check-updates\n"), c.Tag)
	}
}
