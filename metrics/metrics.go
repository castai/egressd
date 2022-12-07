package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	exportedEventsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "egressd_exported_events_total",
		Help: "Counter for tracking exported events rate",
	})

	droppedEventsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "egressd_dropped_events_total",
		Help: "Counter for tracking dropped events rate",
	})

	conntrackEntriesCount = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "egressd_conntrack_entries",
		Help: "Gauge for conntrack entries count",
	})

	conntrackActiveEntriesCount = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "egressd_conntrack_entries_active",
		Help: "Gauge for conntrack active entries count",
	})
)

func init() {
	prometheus.MustRegister(
		exportedEventsTotal,
		droppedEventsTotal,
		conntrackEntriesCount,
		conntrackActiveEntriesCount,
	)
}

func IncExportedEvents() {
	exportedEventsTotal.Inc()
}

func IncDroppedEvents() {
	droppedEventsTotal.Inc()
}

func SetConntrackEntriesCount(val float64) {
	conntrackEntriesCount.Set(val)
}

func SetConntrackActiveEntriesCount(val float64) {
	conntrackActiveEntriesCount.Set(val)
}
