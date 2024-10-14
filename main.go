package main

import (
	"io"
	"log"
	"net/http"

	"github.com/alecthomas/kingpin/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/collectors/version"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/promslog"
	promslogflag "github.com/prometheus/common/promslog/flag"
	"github.com/prometheus/exporter-toolkit/web"
	webflag "github.com/prometheus/exporter-toolkit/web/kingpinflag"
)

func main() {
	webConfig := webflag.AddFlags(kingpin.CommandLine, ":8081")
	metricsPath := kingpin.Flag("web.telemetry-path", "Path under which to expose metrics.").Default("/metrics").String()
	timeout := kingpin.Flag("scrape-timeout", "Maximum duration for a scrape.").Default("1m").Duration()

	promslogConfig := &promslog.Config{}
	promslogflag.AddFlags(kingpin.CommandLine, promslogConfig)

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

	logger := promslog.New(promslogConfig)
	server := &http.Server{}

	if err := web.ListenAndServe(server, webConfig, logger); err != nil {
		log.Fatal(err)
	}
}
