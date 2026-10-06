package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

type api struct{ hc *http.Client }

func newAPI() *api {
	sock := sockPath()
	return &api{hc: &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			Proxy: nil,
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", sock)
			},
		},
	}}
}

func (a *api) do(method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, "http://core"+path, body)
	if err != nil {
		return err
	}
	resp, err := a.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		var e struct {
			Message string `json:"message"`
		}
		_ = json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&e)
		return fmt.Errorf(T("ядро ответило %s %s", "core answered %s %s"), resp.Status, clean(e.Message))
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 64<<20)).Decode(out)
}

func (a *api) ping() error {
	return a.do(http.MethodGet, "/version", nil, nil)
}

func (a *api) graph() (graph, error) {
	var r struct {
		Proxies graph `json:"proxies"`
	}
	if err := a.do(http.MethodGet, "/proxies", nil, &r); err != nil {
		return nil, err
	}
	return r.Proxies, nil
}

func (a *api) choose(group, name string) error {
	return a.do(http.MethodPut, "/proxies/"+url.PathEscape(group), map[string]string{"name": name}, nil)
}

func (a *api) matchTarget() string {
	var r struct {
		Rules []struct {
			Type  string `json:"type"`
			Proxy string `json:"proxy"`
		} `json:"rules"`
	}
	if a.do(http.MethodGet, "/rules", nil, &r) != nil {
		return ""
	}
	for i := len(r.Rules) - 1; i >= 0; i-- {
		if r.Rules[i].Type == "Match" {
			return r.Rules[i].Proxy
		}
	}
	return ""
}

func (a *api) setMode(m string) error {
	return a.do(http.MethodPatch, "/configs", map[string]string{"mode": m}, nil)
}

type coreCfg struct {
	Mode      string `json:"mode"`
	MixedPort int    `json:"mixed-port"`
	Tun       struct {
		Enable bool `json:"enable"`
	} `json:"tun"`
}

func (a *api) configs() (*coreCfg, error) {
	c := &coreCfg{}
	return c, a.do(http.MethodGet, "/configs", nil, c)
}
