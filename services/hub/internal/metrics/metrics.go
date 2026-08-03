package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
	EventsConsumed = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "hub_events_consumed_total",
		Help: "Total events consumed from EVENTS stream.",
	})
	TriggersMatched = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "hub_triggers_matched_total",
		Help: "Total trigger matches.",
	})
	EventsRouted = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "hub_events_routed_total",
		Help: "Total events routed to agents.",
	})
	RoutingErrors = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "hub_routing_errors_total",
		Help: "Total routing errors.",
	})
	CronFires = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "hub_cron_fires_total",
		Help: "Total cron trigger fires published to agents.",
	})
)

func init() {
	prometheus.MustRegister(EventsConsumed, TriggersMatched, EventsRouted, RoutingErrors, CronFires)
}
