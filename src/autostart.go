package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const unitName = "flclashx-cli.service"

func unitPath() string {
	return filepath.Join(filepath.Dir(dir.cfg), "systemd", "user", unitName)
}

func desktopPath() string {
	return filepath.Join(filepath.Dir(dir.cfg), "autostart", "flclashx-cli.desktop")
}

func userSystemd() bool {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return false
	}
	return exec.Command("systemctl", "--user", "show-environment").Run() == nil
}

func systemctl(args ...string) error {
	out, err := exec.Command("systemctl", append([]string{"--user"}, args...)...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl %s: %s", strings.Join(args, " "), strings.TrimSpace(clean(string(out))))
	}
	return nil
}

func autostartState() string {
	if _, err := os.Stat(unitPath()); err == nil {
		return "systemd"
	}
	if _, err := os.Stat(desktopPath()); err == nil {
		return "xdg"
	}
	return ""
}

func setAutostart(on bool) error {
	if !on {
		var errs []error
		if _, err := os.Stat(unitPath()); err == nil {
			if userSystemd() {
				_ = systemctl("disable", unitName)
			}
			errs = append(errs, rmFile(unitPath()))
			if userSystemd() {
				_ = systemctl("daemon-reload")
			}
		}
		errs = append(errs, rmFile(desktopPath()))
		return errors.Join(errs...)
	}

	exe, err := self()
	if err != nil {
		return err
	}
	if strings.ContainsAny(exe, "\"\\\n%") {
		return fmt.Errorf(T("путь %q не подходит для автозапуска", "path %q can't be used for autostart"), exe)
	}

	if userSystemd() {
		unit := fmt.Sprintf(`[Unit]
Description=FLClashX CLI
After=graphical-session.target network-online.target

[Service]
Type=oneshot
RemainAfterExit=yes
ExecStart="%[1]s" start
ExecStop="%[1]s" stop

[Install]
WantedBy=default.target
`, exe)
		if err := os.MkdirAll(filepath.Dir(unitPath()), 0o755); err != nil {
			return err
		}
		if err := writeFile(unitPath(), []byte(unit)); err != nil {
			return err
		}
		_ = rmFile(desktopPath())
		if err := systemctl("daemon-reload"); err != nil {
			return err
		}
		return systemctl("enable", unitName)
	}

	desk := fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=FLClashX CLI
Exec="%s" start
Terminal=false
NoDisplay=true
X-GNOME-Autostart-enabled=true
`, exe)
	if err := os.MkdirAll(filepath.Dir(desktopPath()), 0o755); err != nil {
		return err
	}
	return writeFile(desktopPath(), []byte(desk))
}

func rmFile(p string) error {
	if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
