package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func probeHandler(w http.ResponseWriter, r *http.Request, timeout time.Duration) {
	ctx := r.Context()

	if timeout != 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()

		r = r.WithContext(ctx)
	}

	targetRaw := r.URL.Query().Get("target")
	if targetRaw == "" {
		http.Error(w, `Missing "target" parameter`, http.StatusBadRequest)
		return
	}

	target, err := url.Parse(targetRaw)
	if err != nil {
		http.Error(w, fmt.Sprintf(`Parsing target URL failed: %v`, err), http.StatusBadRequest)
		return
	}

	reg := prometheus.NewPedanticRegistry()
	reg.MustRegister(newCollector(ctx, target))

	h := promhttp.HandlerFor(reg, promhttp.HandlerOpts{})
	h.ServeHTTP(w, r)
}
