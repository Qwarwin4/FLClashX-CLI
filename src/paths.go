package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

var dir struct {
	cfg   string
	data  string
	state string
	run   string
}

func setDirs() error {
	home, err := os.UserHomeDir()
	if err != nil || !filepath.IsAbs(home) {
		return errors.New("can't figure out the home directory")
	}
	xdg := func(env, def string) string {
		if v := os.Getenv(env); filepath.IsAbs(v) {
			return filepath.Join(v, "flc")
		}
		return filepath.Join(home, def, "flc")
	}
	dir.cfg = xdg("XDG_CONFIG_HOME", ".config")
	dir.data = xdg("XDG_DATA_HOME", ".local/share")
	dir.state = xdg("XDG_STATE_HOME", ".local/state")
	if v := os.Getenv("XDG_RUNTIME_DIR"); filepath.IsAbs(v) {
		dir.run = filepath.Join(v, "flc")
	} else {
		dir.run = filepath.Join(dir.state, "run")
	}
	return nil
}

func mkDirs() error {
	for _, d := range []string{dir.cfg, profDir(), dir.data, coreHome(), dir.state, dir.run} {
		if err := mkPrivate(d); err != nil {
			return err
		}
	}
	return nil
}

func profDir() string  { return filepath.Join(dir.cfg, "profiles") }
func coreHome() string { return filepath.Join(dir.data, "core") }
func sockPath() string { return filepath.Join(dir.run, "core.sock") }
func pidPath() string  { return filepath.Join(dir.run, "core.pid") }
func runCfg() string   { return filepath.Join(dir.run, "config.yaml") }
func logPath() string  { return filepath.Join(dir.state, "core.log") }

func mkPrivate(p string) error {
	if err := os.MkdirAll(p, 0o700); err != nil {
		return err
	}
	fi, err := os.Lstat(p)
	if err != nil {
		return err
	}
	if !fi.IsDir() {
		return fmt.Errorf(T("%s: ожидался каталог", "%s: expected a directory"), p)
	}
	if st, ok := fi.Sys().(*syscall.Stat_t); !ok || int(st.Uid) != os.Getuid() {
		return fmt.Errorf(T("%s принадлежит другому пользователю", "%s is owned by another user"), p)
	}
	if fi.Mode().Perm() != 0o700 {
		return os.Chmod(p, 0o700)
	}
	return nil
}

func writeFile(p string, b []byte) error {
	f, err := os.CreateTemp(filepath.Dir(p), "."+filepath.Base(p)+".*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

func lock() (func(), error) {
	f, err := os.OpenFile(filepath.Join(dir.cfg, ".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, err
	}
	return func() { f.Close() }, nil
}
