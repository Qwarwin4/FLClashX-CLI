package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/metacubex/mihomo/constant/features"
	"go.yaml.in/yaml/v3"
)

var stripKeys = []string{
	"external-controller", "external-controller-tls", "external-controller-unix",
	"external-controller-pipe", "external-controller-cors", "external-ui",
	"external-ui-url", "external-ui-name", "external-doh-server", "secret",
	"listeners", "tunnels", "iptables", "tuic-server", "ss-config", "vmess-config",
	"port", "socks-port", "redir-port", "tproxy-port", "bind-address",
	"authentication", "skip-auth-prefixes", "lan-allowed-ips", "lan-disallowed-ips",
}

func buildConfig(p *profile, s settings) ([]byte, error) {
	b, err := os.ReadFile(p.path())
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := yaml.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf(T("профиль «%s» не читается: %w", "can't read profile %q: %w"), p.Name, err)
	}
	if m == nil {
		return nil, fmt.Errorf(T("профиль «%s» пустой", "profile %q is empty"), p.Name)
	}

	for _, k := range stripKeys {
		delete(m, k)
	}
	m["allow-lan"] = false
	m["mixed-port"] = s.MixedPort
	m["mode"] = s.Mode
	m["log-level"] = s.LogLevel
	m["profile"] = map[string]any{"store-selected": false, "store-fake-ip": true}
	if ntp, ok := m["ntp"].(map[string]any); ok {
		ntp["write-to-system"] = false
	}

	dns, _ := m["dns"].(map[string]any)
	if dns != nil {
		delete(dns, "listen")
	}
	if s.Tun {
		m["tun"] = map[string]any{
			"enable":                true,
			"stack":                 stack(s.Stack),
			"auto-route":            true,
			"auto-detect-interface": true,
			"dns-hijack":            []string{"any:53"},
		}
		if dns == nil {
			dns = map[string]any{
				"enhanced-mode": "fake-ip",
				"fake-ip-range": "198.18.0.1/16",
			}
		}
		dns["enable"] = true
		if ns, _ := dns["nameserver"].([]any); len(ns) == 0 {
			dns["nameserver"] = []string{"https://1.1.1.1/dns-query", "https://8.8.8.8/dns-query"}
			dns["default-nameserver"] = []string{"1.1.1.1", "8.8.8.8"}
		}
		m["dns"] = dns
	} else {
		m["tun"] = map[string]any{"enable": false}
	}

	return yaml.Marshal(m)
}

func stack(want string) string {
	if !features.WithGVisor {
		return "system"
	}
	return want
}

func findCore() (string, error) {
	if p := os.Getenv("FLC_CORE"); p != "" {
		return p, nil
	}
	exe, err := os.Executable()
	if err == nil {
		exe, err = filepath.EvalSymlinks(exe)
	}
	if err != nil {
		return "", err
	}
	d := filepath.Dir(exe)
	for _, c := range []string{filepath.Join(d, "..", "lib", "flc", "flc-core"), filepath.Join(d, "flc-core")} {
		if fi, err := os.Stat(c); err == nil && fi.Mode().IsRegular() {
			return filepath.Clean(c), nil
		}
	}
	return "", errors.New(T("не найден flc-core, переустанови flc через install.sh", "flc-core not found, reinstall flc with install.sh"))
}

func corePid() int {
	b, err := os.ReadFile(pidPath())
	if err != nil {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || pid <= 1 {
		return 0
	}
	cl, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil || len(cl) == 0 {
		return 0
	}
	args := strings.Split(strings.TrimRight(string(cl), "\x00"), "\x00")
	if filepath.Base(args[0]) != "flc-core" || !slices.Contains(args, sockPath()) {
		return 0
	}
	stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return 0
	}
	for _, ln := range strings.Split(string(stat), "\n") {
		if f := strings.Fields(ln); len(f) > 1 && f[0] == "Uid:" {
			if f[1] != strconv.Itoa(os.Getuid()) {
				return 0
			}
			return pid
		}
	}
	return 0
}

func coreEnv() []string {
	var env []string
	for _, k := range []string{"HOME", "PATH", "LANG", "LC_ALL", "TZ", "SSL_CERT_FILE", "SSL_CERT_DIR"} {
		if v, ok := os.LookupEnv(k); ok {
			env = append(env, k+"="+v)
		}
	}
	return append(env, "FLC_LANG="+lang)
}

func startCore(p *profile, s settings) error {
	cfg, err := buildConfig(p, s)
	if err != nil {
		return err
	}
	core, err := findCore()
	if err != nil {
		return err
	}
	if err := writeFile(runCfg(), cfg); err != nil {
		return err
	}

	_ = os.Rename(logPath(), logPath()+".1")
	lf, err := os.OpenFile(logPath(), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer lf.Close()
	fmt.Fprintf(lf, T("flc %s: старт, профиль «%s»\n", "flc %s: start, profile %q\n"), time.Now().Format(time.DateTime), p.Name)

	cmd := exec.Command(core, "-d", coreHome(), "-f", runCfg(), "-s", sockPath())
	cmd.Dir = coreHome()
	cmd.Env = coreEnv()
	cmd.Stdout, cmd.Stderr = lf, lf
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		if errors.Is(err, fs.ErrPermission) {
			return errors.New(T("нет прав на запуск flc-core, переустанови flc через install.sh",
				"no permission to run flc-core, reinstall flc with install.sh"))
		}
		return err
	}
	if err := writeFile(pidPath(), []byte(strconv.Itoa(cmd.Process.Pid))); err != nil {
		_ = cmd.Process.Kill()
		return err
	}

	died := make(chan error, 1)
	go func() { died <- cmd.Wait() }()
	if err := waitReady(died); err != nil {
		if corePid() > 0 {
			_ = cmd.Process.Kill()
		}
		cleanup()
		return err
	}

	a := newAPI()
	for g, n := range p.Selected {
		if err := a.choose(g, n); err != nil {
			fmt.Fprintf(os.Stderr, T("не получилось вернуть %q в группе %q: %v\n", "couldn't restore %q in group %q: %v\n"), clean(n), clean(g), err)
		}
	}
	if s.Mode == "global" {
		fixGlobal(a, p)
	}
	if s.Tun {
		if c, err := a.configs(); err == nil && !c.Tun.Enable {
			fmt.Fprintln(os.Stderr, T("внимание: TUN не поднялся, работает только прокси на 127.0.0.1. Причина в flc debug log",
				"warning: TUN didn't come up, only the proxy on 127.0.0.1 works. See flc debug log for why"))
		}
	}
	return nil
}

func waitReady(died <-chan error) error {
	a := newAPI()
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	slow := time.After(5 * time.Second)
	limit := time.After(90 * time.Second)
	for {
		select {
		case err := <-died:
			if err == nil {
				err = errors.New(T("процесс завершился", "process exited"))
			}
			return fmt.Errorf(T("ядро упало при старте (%v), смотри flc debug log", "core crashed on start (%v), see flc debug log"), err)
		case <-slow:
			fmt.Println(T("ядро стартует дольше обычного, при первом запуске оно качает geo-базы...",
				"the core is taking a while, on first run it downloads geo databases..."))
		case <-limit:
			return errors.New(T("ядро не ответило за 90 секунд, смотри flc debug log", "core didn't respond in 90 seconds, see flc debug log"))
		case <-tick.C:
			if a.ping() == nil {
				return nil
			}
		}
	}
}

func stopCore() (bool, error) {
	pid := corePid()
	if pid == 0 {
		cleanup()
		return false, nil
	}
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		return false, err
	}
	for i := 0; i < 100 && corePid() == pid; i++ {
		time.Sleep(100 * time.Millisecond)
	}
	if corePid() == pid {
		_ = syscall.Kill(pid, syscall.SIGKILL)
		fmt.Fprintln(os.Stderr, T("ядро не завершилось за 10 секунд, пришлось убить", "core didn't exit in 10 seconds, killed it"))
	}
	cleanup()
	return true, nil
}

func cleanup() {
	for _, p := range []string{pidPath(), sockPath(), runCfg()} {
		_ = os.Remove(p)
	}
}

func restart(p *profile, s settings) error {
	if _, err := stopCore(); err != nil {
		return err
	}
	return startCore(p, s)
}
