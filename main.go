package main

import (
	"io"
	"log"
	"net/http"

	"github.com/alecthomas/kingpin/v2"
	kitlog "github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/version"
	"github.com/prometheus/exporter-toolkit/web"
	webflag "github.com/prometheus/exporter-toolkit/web/kingpinflag"
)

func main() {
	webConfig := webflag.AddFlags(kingpin.CommandLine, ":8081")
	metricsPath := kingpin.Flag("web.telemetry-path", "Path under which to expose metrics.").Default("/metrics").String()
	timeout := kingpin.Flag("scrape-timeout", "Maximum duration for a scrape.").Default("1m").Duration()

	kingpin.Parse()

	reg := prometheus.NewPedanticRegistry()
	reg.MustRegister(
		collectors.NewBuildInfoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		collectors.NewGoCollector(),
		version.NewCollector("prometheus_ai_on_the_edge_device_exporter"),
	)

	http.Handle(*metricsPath, promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	http.HandleFunc("/probe", func(w http.ResponseWriter, r *http.Request) {
		probeHandler(w, r, *timeout)
	})
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `<html>
			<head><title>AI-on-the-edge-device Exporter</title></head>
			<body>
			<h1>AI-on-the-edge-device Exporter</h1>
			<p><a href="`+*metricsPath+`">Metrics</a></p>
			</body>
			</html>`)
	})

	logger := kitlog.NewLogfmtLogger(kitlog.StdlibWriter{})
	server := &http.Server{}

	if err := web.ListenAndServe(server, webConfig, logger); err != nil {
		log.Fatal(err)
	}
}
