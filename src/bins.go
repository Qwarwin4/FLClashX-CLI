package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

func self() (string, error) {
	exe, err := os.Executable()
	if err == nil {
		exe, err = filepath.EvalSymlinks(exe)
	}
	return exe, err
}

func siblingCore(exe string) (string, error) {
	d := filepath.Dir(exe)
	for _, c := range []string{filepath.Join(d, "flc-core"), filepath.Join(d, "..", "lib", "flc", "flc-core")} {
		if fi, err := os.Lstat(c); err == nil && fi.Mode().IsRegular() {
			return filepath.Clean(c), nil
		}
	}
	return "", errors.New(T("рядом с flc нет flc-core: распакуй архив релиза целиком",
		"flc-core isn't next to flc: unpack the whole release archive"))
}

func checkSource(p string) error {
	fi, err := os.Lstat(p)
	if err != nil {
		return err
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !fi.Mode().IsRegular() || !ok {
		return fmt.Errorf(T("%s не обычный файл", "%s is not a regular file"), p)
	}
	if st.Uid != 0 && int(st.Uid) != os.Getuid() {
		return fmt.Errorf(T("%s принадлежит чужому пользователю", "%s belongs to another user"), p)
	}
	if fi.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf(T("%s доступен на запись другим, не буду ставить его от root", "%s is writable by others, not installing it as root"), p)
	}
	return nil
}

func rootTool() (string, error) {
	for _, su := range []string{"sudo", "doas", "run0"} {
		if p, err := exec.LookPath(su); err == nil {
			return p, nil
		}
	}
	return "", errors.New(T("нужен sudo или doas", "sudo or doas is needed"))
}

func asRoot(args ...string) error {
	su, err := rootTool()
	if err != nil {
		return err
	}
	c := exec.Command(su, args...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("%s: %w", args[0], err)
	}
	return nil
}

func asRootQuiet(args ...string) error {
	su, err := rootTool()
	if err != nil {
		return err
	}
	c := exec.Command(su, args...)
	c.Stdin = os.Stdin
	return c.Run()
}

func cmdDelete(args []string) error {
	fs := newFlags("delete")
	yes := fs.Bool("y", false, "")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) > 0 {
		return errors.New(T("delete не принимает аргументов", "delete takes no arguments"))
	}
	if !*yes && !confirm(T("удалить flc полностью: профили, настройки, логи и программу?",
		"remove flc completely: profiles, settings, logs and the program?"), false) {
		return nil
	}

	if _, err := stopCore(); err != nil {
		return err
	}
	for _, d := range []string{dir.run, dir.cfg, dir.data, dir.state} {
		if err := os.RemoveAll(d); err != nil {
			return err
		}
	}
	fmt.Println(T("данные удалены", "data removed"))
	return removeBins()
}

func removeBins() error {
	exe, err := self()
	if err != nil {
		return err
	}
	if filepath.Base(exe) != "flc" {
		return fmt.Errorf(T("%s не похож на flc, удали его вручную", "%s doesn't look like flc, remove it by hand"), exe)
	}
	files := []string{exe}
	lib := ""
	if core, err := siblingCore(exe); err == nil {
		files = append(files, core)
		if d := filepath.Dir(core); filepath.Base(d) == "flc" && d != filepath.Dir(exe) {
			lib = d
			files = append(files, filepath.Join(lib, "uninstall.sh"))
		}
	}

	var locked []string
	for _, f := range files {
		err := os.Remove(f)
		switch {
		case err == nil, errors.Is(err, os.ErrNotExist):
		case errors.Is(err, os.ErrPermission):
			locked = append(locked, f)
		default:
			return err
		}
	}
	if len(locked) > 0 {
		fmt.Println(T("удаляю программу, понадобятся права root...", "removing the program, root is needed..."))
		if err := asRoot(append([]string{"rm", "-f", "--"}, locked...)...); err != nil {
			return err
		}
	}
	if lib != "" {
		if os.Remove(lib) != nil {
			_ = asRootQuiet("rmdir", "--", lib)
		}
	}
	fmt.Println(T("flc удалён", "flc removed"))
	return nil
}

func replaceBins(from, exe, core string) error {
	src := filepath.Join(from, "flc")
	if err := checkSource(src); err != nil {
		return err
	}
	for _, dst := range []string{exe, core} {
		if err := replace(src, dst); err != nil {
			return err
		}
	}
	return asRoot("setcap", "cap_net_admin,cap_net_bind_service=+ep", core)
}

func replace(src, dst string) error {
	err := copyOver(src, dst)
	if !errors.Is(err, os.ErrPermission) {
		return err
	}
	return asRoot("install", "-m", "755", "-o", "root", "-g", "root", "--", src, dst)
}

func copyOver(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".flc-new-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	_, err = io.Copy(tmp, in)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Chmod(tmp.Name(), 0o755)
	}
	if err != nil {
		return err
	}
	return os.Rename(tmp.Name(), dst)
}
