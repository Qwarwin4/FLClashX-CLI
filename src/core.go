package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/metacubex/mihomo/component/updater"
	"github.com/metacubex/mihomo/config"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/hub"
	"github.com/metacubex/mihomo/hub/executor"
	"github.com/metacubex/mihomo/log"
	"golang.org/x/sys/unix"
)

const maxCfg = 32 << 20

func coreMain() {
	if os.Getenv("FLC_LANG") == "ru" {
		lang = "ru"
	}
	home := flag.String("d", "", "core home directory")
	file := flag.String("f", "", "config file")
	sock := flag.String("s", "", "controller unix socket")
	test := flag.Bool("t", false, "check config and exit")
	ver := flag.Bool("v", false, "print version")
	flag.Parse()

	if *ver {
		fmt.Println("mihomo", C.Version)
		return
	}
	if *home == "" || *file == "" || (*sock == "" && !*test) {
		flag.Usage()
		os.Exit(2)
	}
	if os.Geteuid() == 0 {
		die(T("не запускай flc-core от root, ему хватает capabilities", "don't run flc-core as root, capabilities are enough"))
	}

	unix.Umask(0o077)
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		die("prctl: %v", err)
	}

	for _, p := range []*string{home, file, sock} {
		if *p != "" && !filepath.IsAbs(*p) {
			die(T("пути должны быть абсолютными: %s", "paths must be absolute: %s"), *p)
		}
	}
	if err := checkPrivate(*home); err != nil {
		die("%v", err)
	}
	if *sock != "" {
		if err := checkPrivate(filepath.Dir(*sock)); err != nil {
			die("%v", err)
		}
	}

	raw, err := readCfg(*file)
	if err != nil {
		die("%v", err)
	}

	C.SetHomeDir(*home)
	C.SetConfig(*file)
	if err := config.Init(*home); err != nil {
		die("init: %v", err)
	}

	if *test {
		if _, err := executor.ParseWithBytes(raw); err != nil {
			die("%v", err)
		}
		fmt.Println("ok")
		return
	}

	lk, err := os.OpenFile(filepath.Join(*home, "core.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		die("%v", err)
	}
	defer lk.Close()
	if err := unix.Flock(int(lk.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		die(T("ядро уже запущено", "the core is already running"))
	}

	opts := []hub.Option{hub.WithExternalControllerUnix(*sock), lockdown}
	if err := hub.Parse(raw, opts...); err != nil {
		die(T("конфиг: %v", "config: %v"), err)
	}
	if updater.GeoAutoUpdate() {
		updater.RegisterGeoUpdater()
	}
	defer executor.Shutdown()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	for s := range sig {
		if s != syscall.SIGHUP {
			log.Infoln(T("получен %s, выхожу", "got %s, exiting"), s)
			return
		}
		b, err := readCfg(*file)
		if err == nil {
			err = hub.Parse(b, opts...)
		}
		if err != nil {
			log.Errorln(T("перечитать конфиг не вышло: %v", "couldn't reload config: %v"), err)
		}
	}
}

func lockdown(c *config.Config) {
	ctl := c.Controller
	ctl.ExternalController = ""
	ctl.ExternalControllerTLS = ""
	ctl.ExternalControllerPipe = ""
	ctl.ExternalUI = ""
	ctl.ExternalUIURL = ""
	ctl.ExternalDohServer = ""
	ctl.Secret = ""

	g := c.General
	g.AllowLan = false
	g.BindAddress = "127.0.0.1"
	g.Port, g.SocksPort, g.RedirPort, g.TProxyPort = 0, 0, 0, 0
	g.TuicServer.Enable = false
	g.ShadowSocksConfig = ""
	g.VmessConfig = ""

	c.Listeners = map[string]C.InboundListener{}
	c.Tunnels = nil
	if c.IPTables != nil {
		c.IPTables.Enable = false
	}
	if c.NTP != nil {
		c.NTP.WriteToSystem = false
	}
	if c.DNS != nil {
		c.DNS.Listen = ""
	}
}

func checkPrivate(d string) error {
	fi, err := os.Lstat(d)
	if err != nil {
		return err
	}
	if !fi.IsDir() {
		return fmt.Errorf(T("%s не каталог", "%s is not a directory"), d)
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok || int(st.Uid) != os.Getuid() {
		return fmt.Errorf(T("%s принадлежит другому пользователю", "%s is owned by another user"), d)
	}
	if fi.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf(T("%s доступен другим пользователям (нужно 0700)", "%s is accessible by other users (needs 0700)"), d)
	}
	return nil
}

func readCfg(p string) ([]byte, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxCfg+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxCfg {
		return nil, fmt.Errorf(T("%s слишком большой", "%s is too big"), p)
	}
	return b, nil
}

func die(f string, a ...any) {
	fmt.Fprintf(os.Stderr, "flc-core: "+f+"\n", a...)
	os.Exit(1)
}
