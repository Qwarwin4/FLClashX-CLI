package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
)

const (
	sudoDropIn = "/etc/sudoers.d/flc"
	polkitRule = "/etc/polkit-1/rules.d/49-flclashx-cli.rules"
)

func sudoOn() bool {
	fi, err := os.Stat(sudoDropIn)
	return err == nil && fi.Mode().IsRegular()
}

func polkitOn() bool {
	fi, err := os.Stat(polkitRule)
	return err == nil && fi.Mode().IsRegular()
}

func sudoNameBad(s string) bool {
	return strings.IndexFunc(s, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' ||
			r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-')
	}) >= 0
}

func sudoUser() (string, error) {
	if u, err := user.Current(); err == nil && u.Username != "" && !sudoNameBad(u.Username) {
		return u.Username, nil
	}
	for _, k := range []string{"LOGNAME", "USER"} {
		if v := os.Getenv(k); v != "" && !sudoNameBad(v) {
			return v, nil
		}
	}
	return "", errors.New(T("не вышло определить пользователя", "can't figure out the user"))
}

// what flc puts under /usr and removes from there as root
func rootFiles() ([]string, string) {
	exe, err := self()
	if err != nil {
		return nil, ""
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
	return files, lib
}

func rootBins(name string) []string {
	var out []string
	seen := map[string]bool{}
	add := func(p string) {
		if !filepath.IsAbs(p) || seen[p] {
			return
		}
		if fi, err := os.Stat(p); err == nil && fi.Mode().IsRegular() && fi.Mode().Perm()&0o111 != 0 {
			seen[p] = true
			out = append(out, p)
		}
	}
	if p, err := exec.LookPath(name); err == nil {
		add(p)
	}
	for _, p := range []string{"/usr/bin/" + name, "/usr/sbin/" + name, "/bin/" + name, "/sbin/" + name} {
		add(p)
	}
	return out
}

func sudoersText() (string, error) {
	who, err := sudoUser()
	if err != nil {
		return "", err
	}
	files, lib := rootFiles()
	if len(files) < 2 {
		return "", errors.New(T("рядом с flc нет flc-core: переустанови flc через install.sh",
			"flc-core isn't next to flc: reinstall flc with install.sh"))
	}
	paths := append([]string{}, files...)
	if lib != "" {
		paths = append(paths, lib)
	}
	for _, p := range paths {
		if strings.ContainsAny(p, " \t\n\r") {
			return "", fmt.Errorf(T("в пути %q пробелы, правило sudo для такого пути не напишешь",
				"path %q has spaces, no sudo rule can cover it"), p)
		}
	}

	var cmnds []string
	add := func(s string) { cmnds = append(cmnds, "    "+s) }
	for _, p := range rootBins("install") {
		add(p + " -m 755 -o root -g root -- * " + files[0])
		add(p + " -m 755 -o root -g root -- * " + files[1])
	}
	for _, p := range rootBins("setcap") {
		add(p + " cap_net_admin,cap_net_bind_service=+ep " + files[1])
	}
	bins := strings.Join(files, " ")
	for _, p := range rootBins("rm") {
		add(p + " -f -- " + bins)
		add(p + " -f -- " + sudoDropIn + " " + polkitRule)
	}
	if lib != "" {
		for _, p := range rootBins("rmdir") {
			add(p + " -- " + lib)
		}
	}
	if len(cmnds) == 0 {
		return "", errors.New(T("в системе нет install, правило sudo не собрать",
			"no install on this system, can't build the sudo rule"))
	}

	alias := "Cmnd_Alias FLCX_CMDS = \\\n" + strings.Join(cmnds, ", \\\n")
	return "# Managed by FLClashX CLI, flc settings → skip password prompts.\n" +
		"# Lets " + who + " run the commands flc needs as root without the password.\n" +
		alias + "\n" +
		who + " ALL=(root) NOPASSWD: FLCX_CMDS\n", nil
}

// the four resolved actions sing-tun hits through resolvectl on TUN start and stop
func polkitText(who string) string {
	return "// Managed by FLClashX CLI, flc settings → skip password prompts.\n" +
		"polkit.addRule(function(action, subject) {\n" +
		"    if (subject.user != \"" + who + "\") {\n" +
		"        return;\n" +
		"    }\n" +
		"    switch (action.id) {\n" +
		"    case \"org.freedesktop.resolve1.set-domains\":\n" +
		"    case \"org.freedesktop.resolve1.set-dns-servers\":\n" +
		"    case \"org.freedesktop.resolve1.set-default-route\":\n" +
		"    case \"org.freedesktop.resolve1.revert\":\n" +
		"        return polkit.Result.YES;\n" +
		"    }\n" +
		"});\n"
}

func writeStateTemp(pattern, content string) (string, error) {
	f, err := os.CreateTemp(dir.state, pattern)
	if err != nil {
		return "", err
	}
	p := f.Name()
	if _, err := f.WriteString(content); err != nil {
		f.Close()
		os.Remove(p)
		return "", err
	}
	if err := f.Close(); err != nil {
		os.Remove(p)
		return "", err
	}
	return p, nil
}

// the rule files are settled when both are there, or none is
func promptsSettled(want bool) bool {
	has := sudoOn() || polkitOn()
	full := sudoOn() && polkitOn()
	return (want && full) || (!want && !has)
}

func setSkipPrompts(on bool) error {
	if !on {
		if !sudoOn() && !polkitOn() {
			return nil
		}
		// pinned by the sudoers rule, runs without the password
		return asRootQuiet("rm", "-f", "--", sudoDropIn, polkitRule)
	}
	su, err := rootTool()
	if err != nil {
		return err
	}
	if base := filepath.Base(su); base != "sudo" {
		return fmt.Errorf(T("flc берёт права через %s, исключения на него не влияют",
			"flc elevates through %s, the password exceptions don't apply to it"), base)
	}
	text, err := sudoersText()
	if err != nil {
		return err
	}
	who, err := sudoUser()
	if err != nil {
		return err
	}

	ptmp, err := writeStateTemp("polkit-*", polkitText(who))
	if err != nil {
		return err
	}
	defer os.Remove(ptmp)
	stmp, err := writeStateTemp("sudoers-*", text)
	if err != nil {
		return err
	}
	defer os.Remove(stmp)

	// polkit quiets the VPN start/stop prompts, sudoers quiets update and delete
	if err := asRoot("install", "-d", "-m", "755", "--", filepath.Dir(polkitRule)); err != nil {
		return err
	}
	if err := asRoot("install", "-m", "644", "-o", "root", "-g", "root", "--", ptmp, polkitRule); err != nil {
		return err
	}

	// sudo skips files with a dot in the name, so the rule is live only after the rename
	staged := "/etc/sudoers.d/.flc.new"
	if err := asRoot("install", "-m", "440", "-o", "root", "-g", "root", "--", stmp, staged); err != nil {
		return err
	}
	if vs := rootBins("visudo"); len(vs) > 0 {
		out, err := asRootOut(vs[0], "-c", "-f", staged)
		if err != nil {
			_ = asRootQuiet("rm", "-f", "--", staged)
			if m := strings.TrimSpace(out); m != "" {
				return errors.New(T("visudo не принял правило: ", "visudo rejected the rule: ") + m)
			}
			return fmt.Errorf(T("visudo не принял правило: %w", "visudo rejected the rule: %w"), err)
		}
	}
	return asRootQuiet("mv", "--", staged, sudoDropIn)
}

func syncPrompts() {
	s, err := readSettings()
	if err != nil {
		return
	}
	if promptsSettled(s.SkipPrompts) {
		return
	}
	if s.SkipPrompts {
		fmt.Println(T("записываю правила polkit и sudoers, понадобится пароль...",
			"writing the polkit and sudoers rules, the password will be needed..."))
	}
	if err := setSkipPrompts(s.SkipPrompts); err != nil {
		fmt.Fprintf(os.Stderr, T("исключение пароля: %v\n", "password exception: %v\n"), err)
	}
}
