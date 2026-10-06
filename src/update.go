package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

var repo = "Qwarwin4/flc"

const maxSrc = 128 << 20

var (
	tagRe = regexp.MustCompile(`^v?\d+\.\d+\.\d+$`)
	verRe = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)`)

	errNotFound = errors.New("not found")
)

type release struct {
	Tag    string `json:"tag_name"`
	URL    string `json:"html_url"`
	Body   string `json:"body"`
	Assets []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

func cmdCheckUpdates(args []string) error {
	if err := noArgs("check-updates", args); err != nil {
		return err
	}
	fmt.Println(T("проверяю обновления...", "checking for updates..."))
	cl := ghClient()
	r, err := latest(cl)
	if err != nil {
		return err
	}

	cur, _ := semver(version)
	if !newer(semverOr0(r.Tag), cur) {
		fmt.Printf(T("у тебя последняя версия (%s)\n", "you're up to date (%s)\n"), version)
		return nil
	}

	fmt.Printf(T("доступна версия %s, у тебя %s\n", "version %s is available, you have %s\n"), r.Tag, version)
	if n := notes(r.Body); n != "" {
		fmt.Println()
		fmt.Println(n)
		fmt.Println()
	}
	if r.URL != "" {
		fmt.Println(clean(r.URL))
	}
	if !confirm(T("обновить?", "update?"), true) {
		return nil
	}
	return upgrade(cl, r)
}

func ghClient() *http.Client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	return &http.Client{
		Timeout:   2 * time.Minute,
		Transport: tr,
		CheckRedirect: func(r *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New(T("слишком много редиректов", "too many redirects"))
			}
			if r.URL.Scheme != "https" || !trusted(r.URL.Hostname()) {
				return fmt.Errorf(T("неожиданный редирект на %s", "unexpected redirect to %s"), r.URL.Host)
			}
			return nil
		},
	}
}

var ghHosts = map[string]bool{
	"api.github.com":                       true,
	"github.com":                           true,
	"codeload.github.com":                  true,
	"objects.githubusercontent.com":        true,
	"release-assets.githubusercontent.com": true,
}

func trusted(h string) bool { return ghHosts[h] }

func ghGet(cl *http.Client, url string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "flc/"+version)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := cl.Do(req)
	if err != nil {
		return nil, err
	}
	switch resp.StatusCode {
	case http.StatusOK:
		return resp, nil
	case http.StatusNotFound:
		resp.Body.Close()
		return nil, errNotFound
	}
	resp.Body.Close()
	return nil, fmt.Errorf(T("GitHub ответил %s", "GitHub answered %s"), resp.Status)
}

func ghJSON(cl *http.Client, url string, out any) error {
	resp, err := ghGet(cl, url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out)
}

func latest(cl *http.Client) (*release, error) {
	api := "https://api.github.com/repos/" + repo
	r := &release{}
	err := ghJSON(cl, api+"/releases/latest", r)
	if errors.Is(err, errNotFound) {
		var tags []struct {
			Name string `json:"name"`
		}
		if err := ghJSON(cl, api+"/tags?per_page=100", &tags); err != nil {
			return nil, err
		}
		for _, t := range tags {
			if tagRe.MatchString(t.Name) && (r.Tag == "" || newer(semverOr0(t.Name), semverOr0(r.Tag))) {
				r.Tag = t.Name
			}
		}
		r.URL = "https://github.com/" + repo + "/tree/" + r.Tag
		err = nil
	}
	if err != nil {
		return nil, fmt.Errorf(T("не удалось проверить обновления: %w", "couldn't check for updates: %w"), err)
	}
	if r.Tag == "" {
		return nil, errors.New(T("в репозитории пока нет версий", "the repo has no versions yet"))
	}
	if !tagRe.MatchString(r.Tag) {
		return nil, fmt.Errorf(T("странный тег версии %q", "odd version tag %q"), r.Tag)
	}
	return r, nil
}

func semver(s string) ([3]int, bool) {
	m := verRe.FindStringSubmatch(s)
	if m == nil {
		return [3]int{}, false
	}
	var v [3]int
	for i := range v {
		v[i], _ = strconv.Atoi(m[i+1])
	}
	return v, true
}

func semverOr0(s string) [3]int {
	v, _ := semver(s)
	return v
}

func newer(a, b [3]int) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return false
}

func notes(s string) string {
	lines := strings.Split(strings.TrimSpace(strings.ReplaceAll(s, "\r", "")), "\n")
	if len(lines) > 15 {
		lines = append(lines[:15], "...")
	}
	for i := range lines {
		lines[i] = "  " + clean(lines[i])
	}
	return strings.TrimRight(strings.Join(lines, "\n"), " ")
}

func upgrade(cl *http.Client, r *release) error {
	exe, err := self()
	if err != nil {
		return err
	}
	core, err := siblingCore(exe)
	if err != nil {
		return err
	}

	work, err := os.MkdirTemp(dir.state, "update-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)

	name := fmt.Sprintf("flc-%s-linux-%s.tar.gz", r.Tag, runtime.GOARCH)
	var bin, sums string
	for _, a := range r.Assets {
		switch a.Name {
		case name:
			bin = a.URL
		case "SHA256SUMS":
			sums = a.URL
		}
	}
	var from string
	if bin != "" && sums != "" {
		from, err = fromRelease(cl, work, name, bin, sums)
	} else {
		from, err = fromSource(cl, work, r.Tag)
	}
	if err != nil {
		return err
	}
	if err := replaceBins(from, exe, core); err != nil {
		return fmt.Errorf(T("не получилось заменить файлы: %w", "couldn't replace the files: %w"), err)
	}
	fmt.Printf(T("flc обновлён до %s\n", "flc updated to %s\n"), r.Tag)

	if corePid() == 0 || !confirm(T("VPN запущен, перезапустить на новой версии?", "VPN is running, restart it on the new version?"), true) {
		return nil
	}
	c := exec.Command(exe, "restart")
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	return c.Run()
}

func fromRelease(cl *http.Client, work, name, bin, sums string) (string, error) {
	for _, u := range []string{bin, sums} {
		if p, err := url.Parse(u); err != nil || p.Scheme != "https" || !trusted(p.Hostname()) {
			return "", fmt.Errorf(T("подозрительная ссылка на файл релиза: %s", "suspicious release file link: %s"), clean(u))
		}
	}

	resp, err := ghGet(cl, sums)
	if err != nil {
		return "", err
	}
	list, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	resp.Body.Close()
	if err != nil {
		return "", err
	}
	want := ""
	for _, ln := range strings.Split(string(list), "\n") {
		if f := strings.Fields(ln); len(f) == 2 && strings.TrimPrefix(f[1], "*") == name {
			want = strings.ToLower(f[0])
		}
	}
	if len(want) != 64 {
		return "", fmt.Errorf(T("в SHA256SUMS нет %s", "%s is missing from SHA256SUMS"), name)
	}

	fmt.Printf(T("качаю %s...\n", "downloading %s...\n"), name)
	resp, err = ghGet(cl, bin)
	if err != nil {
		return "", err
	}
	path := filepath.Join(work, name)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		resp.Body.Close()
		return "", err
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), io.LimitReader(resp.Body, maxSrc+1))
	resp.Body.Close()
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return "", err
	}
	if n > maxSrc {
		return "", errors.New(T("архив слишком большой", "archive is too big"))
	}
	if hex.EncodeToString(h.Sum(nil)) != want {
		return "", errors.New(T("контрольная сумма не совпала, обновлять не буду", "checksum mismatch, not updating"))
	}

	f, err = os.Open(path)
	if err != nil {
		return "", err
	}
	out, err := untar(f, filepath.Join(work, "x"))
	f.Close()
	if err != nil {
		return "", fmt.Errorf(T("архив релиза битый: %w", "release archive is broken: %w"), err)
	}

	return out, nil
}

func fromSource(cl *http.Client, work, tag string) (string, error) {
	fmt.Printf(T("готовой сборки под %s нет, собираю %s из исходников...\n",
		"no prebuilt build for %s, building %s from source...\n"), runtime.GOARCH, tag)
	resp, err := ghGet(cl, "https://codeload.github.com/"+repo+"/tar.gz/refs/tags/"+tag)
	if err != nil {
		return "", err
	}
	src, err := untar(io.LimitReader(resp.Body, maxSrc), work)
	resp.Body.Close()
	if err != nil {
		return "", fmt.Errorf(T("архив с исходниками битый: %w", "source archive is broken: %w"), err)
	}
	for _, f := range []string{"install.sh", "go.mod", "src"} {
		if _, err := os.Stat(filepath.Join(src, f)); err != nil {
			return "", fmt.Errorf(T("в архиве нет %s, обновлять не буду", "%s is missing from the archive, not updating"), f)
		}
	}

	c := exec.Command("/bin/sh", filepath.Join(src, "install.sh"))
	c.Dir = src
	c.Env = append(os.Environ(), "FLC_VERSION="+tag, "FLC_BUILD_ONLY=1")
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := c.Run(); err != nil {
		return "", errors.New(T("сборка не удалась, старая версия осталась на месте",
			"build failed, the old version is still in place"))
	}
	out := filepath.Join(src, "build")
	if err := os.Chmod(filepath.Join(out, "flc"), 0o700); err != nil {
		return "", err
	}
	return out, nil
}

func untar(r io.Reader, dst string) (string, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return "", err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)

	var top string
	var total int64
	for n := 0; ; n++ {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		if n > 10000 {
			return "", errors.New(T("слишком много файлов", "too many files"))
		}
		if h.Typeflag != tar.TypeDir && h.Typeflag != tar.TypeReg {
			continue
		}

		name := filepath.Clean(h.Name)
		if filepath.IsAbs(name) || name == ".." || strings.HasPrefix(name, "../") {
			return "", fmt.Errorf(T("недопустимый путь %q", "bad path %q"), h.Name)
		}
		root, _, _ := strings.Cut(name, "/")
		if top == "" {
			top = root
		} else if root != top {
			return "", errors.New(T("в архиве несколько корней", "archive has more than one root"))
		}

		p := filepath.Join(dst, name)
		if h.Typeflag == tar.TypeDir {
			if err := os.MkdirAll(p, 0o700); err != nil {
				return "", err
			}
			continue
		}
		if total += h.Size; total > maxSrc {
			return "", errors.New(T("архив слишком большой", "archive is too big"))
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			return "", err
		}
		mode := os.FileMode(0o600)
		if h.Mode&0o111 != 0 {
			mode = 0o700
		}
		f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
		if err != nil {
			return "", err
		}
		_, err = io.Copy(f, io.LimitReader(tr, h.Size))
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return "", err
		}
	}
	if top == "" {
		return "", errors.New(T("архив пустой", "archive is empty"))
	}
	return filepath.Join(dst, top), nil
}
