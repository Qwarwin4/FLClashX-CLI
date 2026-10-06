package main

import (
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/metacubex/mihomo/common/convert"
	"go.yaml.in/yaml/v3"
)

const maxSize = 16 << 20

type sub struct {
	body []byte
	name string
	info *subInfo
}

func download(raw string, s settings, allowHTTP bool) (*sub, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return nil, errors.New(T("некорректная ссылка", "invalid link"))
	}
	switch u.Scheme {
	case "https":
	case "http":
		if !allowHTTP {
			return nil, errors.New(T("ссылка без https: подписка с ключами пойдёт открытым текстом. Если уверен — добавь --allow-http",
				"link is not https: the subscription and its keys would go in plain text. If you're sure, add --allow-http"))
		}
	default:
		return nil, fmt.Errorf(T("схема %q не поддерживается", "scheme %q is not supported"), u.Scheme)
	}

	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	cl := &http.Client{
		Timeout:   30 * time.Second,
		Transport: tr,
		CheckRedirect: func(r *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New(T("слишком много редиректов", "too many redirects"))
			}
			if via[0].URL.Scheme == "https" && r.URL.Scheme != "https" {
				return errors.New(T("редирект с https на http", "redirect from https to http"))
			}
			return nil
		},
	}

	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, errors.New(T("некорректная ссылка", "invalid link"))
	}
	req.Header.Set("User-Agent", s.UA)
	if s.HWID {
		if id := hwid(); id != "" {
			req.Header.Set("x-hwid", id)
			req.Header.Set("x-device-os", "Linux")
		}
	}

	resp, err := cl.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return nil, fmt.Errorf("%s: %v", u.Hostname(), err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(T("%s ответил %s", "%s answered %s"), u.Hostname(), resp.Status)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxSize+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxSize {
		return nil, errors.New(T("подписка больше 16 МБ, что-то не так", "subscription is over 16 MB, something is off"))
	}

	sb := &sub{body: body, info: parseInfo(resp.Header.Get("subscription-userinfo"))}
	sb.name = title(resp.Header)
	if sb.name == "" {
		sb.name = tidyName(u.Hostname())
	}
	return sb, nil
}

func readLocal(p string) ([]byte, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf(T("%s не обычный файл", "%s is not a regular file"), p)
	}
	b, err := io.ReadAll(io.LimitReader(f, maxSize+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxSize {
		return nil, fmt.Errorf(T("%s больше 16 МБ", "%s is over 16 MB"), p)
	}
	return b, nil
}

func normalize(b []byte) ([]byte, error) {
	var m map[string]any
	if yaml.Unmarshal(b, &m) == nil && m != nil {
		if _, ok := m["proxies"]; ok {
			return b, nil
		}
		if _, ok := m["proxy-providers"]; ok {
			return b, nil
		}
	}

	list, err := convert.ConvertsV2Ray(b)
	if err != nil || len(list) == 0 {
		return nil, errors.New(T("это не clash-конфиг и не список ссылок на серверы", "this is neither a clash config nor a list of server links"))
	}
	var names []string
	for _, p := range list {
		if n, ok := p["name"].(string); ok {
			names = append(names, n)
		}
	}
	return yaml.Marshal(map[string]any{
		"proxies": list,
		"proxy-groups": []map[string]any{
			{"name": "PROXY", "type": "select", "proxies": names},
		},
		"rules": []string{"MATCH,PROXY"},
	})
}

func title(h http.Header) string {
	if t := h.Get("profile-title"); t != "" {
		if rest, ok := strings.CutPrefix(t, "base64:"); ok {
			if b, err := base64.StdEncoding.DecodeString(rest); err == nil {
				t = string(b)
			}
		}
		return tidyName(t)
	}
	if _, ps, err := mime.ParseMediaType(h.Get("Content-Disposition")); err == nil {
		if fn := filepath.Base(ps["filename"]); fn != "" && fn != "." && fn != "/" {
			return tidyName(strings.TrimSuffix(fn, filepath.Ext(fn)))
		}
	}
	return ""
}

func parseInfo(v string) *subInfo {
	if v == "" {
		return nil
	}
	in := &subInfo{}
	for _, kv := range strings.Split(v, ";") {
		k, val, ok := strings.Cut(strings.TrimSpace(kv), "=")
		if !ok {
			continue
		}
		n, err := strconv.ParseInt(strings.TrimSpace(val), 10, 64)
		if err != nil {
			continue
		}
		switch strings.TrimSpace(k) {
		case "upload":
			in.Up = n
		case "download":
			in.Down = n
		case "total":
			in.Total = n
		case "expire":
			in.Expire = n
		}
	}
	return in
}

func hwid() string {
	b, err := os.ReadFile("/etc/machine-id")
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(append([]byte("flc:"), bytes.TrimSpace(b)...))
	return hex.EncodeToString(sum[:16])
}
