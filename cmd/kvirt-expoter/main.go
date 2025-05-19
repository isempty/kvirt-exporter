package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/version"
	"github.com/isempty/kvirt-exporter/collector"
)

var (
	listenAddress = flag.String("web.listen-address", ":9257", "Address to listen on for web interface and telemetry.")
	metricsPath   = flag.String("web.telemetry-path", "/metrics", "Path under which to expose metrics.")
)

func main() {
	flag.Parse()

	// Prometheus 버전 정보 등록
	prometheus.MustRegister(version.NewCollector("kvirt-exporter"))

	// VM CPU 수집기 등록
	collector, err := collector.NewVMCPUCollector()
	if err != nil {
		log.Fatalf("Failed to create collector: %v", err)
	}
	prometheus.MustRegister(collector)

	// HTTP 서버 설정
	http.Handle(*metricsPath, promhttp.Handler())
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `<html>
			<head><title>VM CPU Exporter</title></head>
			<body>
			<h1>VM CPU Exporter</h1>
			<p><a href="%s">Metrics</a></p>
			</body>
			</html>`, *metricsPath)
	})

	log.Printf("Starting VM CPU Exporter on %s", *listenAddress)
	if err := http.ListenAndServe(*listenAddress, nil); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
