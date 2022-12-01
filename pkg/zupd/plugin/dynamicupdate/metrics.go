package dynamicupdate

import (
	"github.com/coredns/coredns/plugin"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// requestCount exports a prometheus metric that is incremented every time a query is seen by the plugin.
var requestCount = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: plugin.Namespace,
	Subsystem: "dynamicupdate",
	Name:      "request_count_total",
	Help:      "Counter of requests made.",
}, []string{"server"})

//var once sync.Once
