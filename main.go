package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"

	"mystrom-exporter/pkg/mystrom"
	"mystrom-exporter/pkg/version"
)

// -- MystromRequestStatusType represents the request to MyStrom device status
type MystromReqStatus uint32

//go:generate stringer -type=MystromReqStatus
const (
	OK MystromReqStatus = iota
	ERROR_SOCKET
	ERROR_TIMEOUT
	ERROR_PARSING_VALUE
)

const namespace = "mystrom_exporter"

var (
	listenAddress = flag.String("web.listen-address", ":9452",
		"Address to listen on")
	metricsPath = flag.String("web.metrics-path", "/metrics",
		"Path under which to expose exporters own metrics")
	devicePath = flag.String("web.device-path", "/device",
		"Path under which the metrics of the devices are fetched")
	showVersion = flag.Bool("version", false,
		"Show version information.")
	targetIP     = flag.String("target", "", "IP of the mystrom gagi")
	municipality = flag.String("municipality", "", "Municipality from where the power cost is calculated")
	powerCost    = flag.String("power-category", "H4", "Power cost category")
)
var (
	mystromDurationCounterVec *prometheus.CounterVec
	mystromRequestsCounterVec *prometheus.CounterVec
)

func main() {

	flag.Parse()

	// -- show version information
	if *showVersion {
		v, err := version.Print("mystrom_exporter")
		if err != nil {
			log.Fatalf("Failed to print version information: %#v", err)
		}

		fmt.Fprintln(os.Stdout, v)
		os.Exit(0)
	}

	// -- create a new registry for the exporter telemetry
	telemetryRegistry := setupMetrics()

	router := mux.NewRouter()
	router.Handle(*metricsPath, promhttp.HandlerFor(telemetryRegistry, promhttp.HandlerOpts{}))
	// router.HandleFunc(*devicePath, scrapeHandler)
	router.HandleFunc("/", scrapeHandler)
	log.Infoln("Listening on address " + *listenAddress)
	log.Fatal(http.ListenAndServe(*listenAddress, router))
}

func scrapeHandler(w http.ResponseWriter, r *http.Request) {
	// target := r.URL.Query().Get("target")
	target := *targetIP
	if target == "" {
		http.Error(w, "'target' parameter must be specified", http.StatusBadRequest)
		return
	}

	log.Infof("got scrape request for target '%v'", target)
	exporter := mystrom.NewExporter(target, *municipality, *powerCost)

	start := time.Now()
	gatherer, err := exporter.Scrape()
	duration := time.Since(start).Seconds()
	if err != nil {
		if strings.Contains(fmt.Sprintf("%v", err), "unable to connect to target") {
			mystromRequestsCounterVec.WithLabelValues(target, fmt.Sprintf("%d", ERROR_SOCKET)).Inc()
		} else if strings.Contains(fmt.Sprintf("%v", err), "i/o timeout") {
			mystromRequestsCounterVec.WithLabelValues(target, fmt.Sprintf("%d", ERROR_TIMEOUT)).Inc()
		} else {
			mystromRequestsCounterVec.WithLabelValues(target, fmt.Sprintf("%d", ERROR_PARSING_VALUE)).Inc()
		}
		http.Error(
			w,
			fmt.Sprintf("failed to scrape target '%v': %v", target, err),
			http.StatusInternalServerError,
		)
		log.Error(err)
		return
	}
	mystromDurationCounterVec.WithLabelValues(target).Add(duration)
	mystromRequestsCounterVec.WithLabelValues(target, fmt.Sprintf("%d", OK)).Inc()

	promhttp.HandlerFor(gatherer, promhttp.HandlerOpts{}).ServeHTTP(w, r)
}

// -- setupMetrics creates a new registry for the exporter telemetry
func setupMetrics() *prometheus.Registry {
	registry := prometheus.NewRegistry()
	registry.MustRegister(collectors.NewGoCollector())
	registry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

	mystromDurationCounterVec = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "request_duration_seconds_total",
			Help:      "Total duration of mystrom successful requests by target in seconds",
		},
		[]string{"target"})
	registry.MustRegister(mystromDurationCounterVec)

	mystromRequestsCounterVec = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "requests_total",
			Help:      "Number of mystrom request by status and target",
		},
		[]string{"target", "status"})
	registry.MustRegister(mystromRequestsCounterVec)

	// -- make the build information is available through a metric
	buildInfo := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "build_info",
			Help:      "A metric with a constant '1' value labeled by build information.",
		},
		[]string{"version", "revision", "branch", "goversion", "builddate", "builduser"},
	)
	buildInfo.WithLabelValues(version.Version, version.Revision, version.Branch, version.GoVersion, version.BuildDate, version.BuildUser).Set(1)
	registry.MustRegister(buildInfo)

	return registry
}
