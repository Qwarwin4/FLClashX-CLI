package main

import (
	"encoding/json"
	"io"
	"sync"

	"github.com/metacubex/chi"
	"github.com/metacubex/http"
	"github.com/metacubex/mihomo/hub/route"
	"github.com/metacubex/mihomo/tunnel"
)

var reloadMu sync.Mutex

func ctlRoutes(file string) {
	route.SetEmbedMode(true)
	route.Register(func(r chi.Router) {
		r.Put("/flc/mode", func(w http.ResponseWriter, r *http.Request) {
			var in struct {
				Mode tunnel.TunnelMode `json:"mode"`
			}
			if err := json.NewDecoder(io.LimitReader(r.Body, 1<<10)).Decode(&in); err != nil {
				reply(w, http.StatusBadRequest, err.Error())
				return
			}
			tunnel.SetMode(in.Mode)
			w.WriteHeader(http.StatusNoContent)
		})
		r.Post("/flc/reload", func(w http.ResponseWriter, r *http.Request) {
			reloadMu.Lock()
			defer reloadMu.Unlock()
			b, err := readCfg(file)
			if err == nil {
				err = apply(b)
			}
			if err != nil {
				reply(w, http.StatusUnprocessableEntity, err.Error())
				return
			}
			w.WriteHeader(http.StatusNoContent)
		})
	})
}

func reply(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"message": msg})
}
