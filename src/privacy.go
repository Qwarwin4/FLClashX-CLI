package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const plainUA = "clash.meta"

var panelKeys = []string{"x-hwid", "x-device-os", "x-ver-os", "x-device-model"}

func agentFor(s settings) string {
	if s.HideUA {
		return plainUA
	}
	return userAgent()
}

func keyPath() string { return filepath.Join(dir.cfg, "hwid-key") }

func fakeKey() (string, error) {
	b, err := os.ReadFile(keyPath())
	if err == nil {
		if k := strings.TrimSpace(string(b)); len(k) == 64 {
			return k, nil
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	r := make([]byte, 32)
	if _, err := rand.Read(r); err != nil {
		return "", err
	}
	k := hex.EncodeToString(r)
	return k, writeFile(keyPath(), []byte(k+"\n"))
}

func fakeHWID(host string) string {
	k, err := fakeKey()
	if err != nil {
		return ""
	}
	sum := sha256.Sum256([]byte("flc-fake:" + k + ":" + strings.ToLower(host)))
	return hex.EncodeToString(sum[:16])
}

func panelID(s settings, host string) string {
	switch s.HWID {
	case "real":
		return hwid()
	case "fake":
		return fakeHWID(host)
	}
	return ""
}

func panelHeaders(s settings, host string) http.Header {
	h := http.Header{}
	h.Set("User-Agent", agentFor(s))
	id := panelID(s, host)
	if id == "" {
		return h
	}
	h.Set("x-hwid", id)
	d := deviceInfo()
	if !s.HideOS {
		h.Set("x-device-os", d.os)
		h.Set("x-ver-os", d.ver)
	}
	if !s.HideDevice {
		h.Set("x-device-model", d.model)
	}
	return h
}

func dropPanel(h http.Header) {
	for _, k := range panelKeys {
		h.Del(k)
	}
}
