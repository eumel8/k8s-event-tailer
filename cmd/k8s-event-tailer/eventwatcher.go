package main

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

const oldEventAgeMinutes = 5

type EventWatcher struct {
	client    rest.Interface
	namespace string
	logger    zerolog.Logger

	_startTime       time.Time
	_store           cache.Store
	_controller      cache.Controller
	startTimeGauge   prometheus.Gauge
	storeSizeGauge   prometheus.GaugeFunc
	addCounter       prometheus.Counter
	updateCounter    prometheus.Counter
	deleteCounter    prometheus.Counter
	oldEventsCounter prometheus.Counter
}

func (ew *EventWatcher) Run(stopChan chan struct{}, wg *sync.WaitGroup) {
	defer wg.Done()

	watchlist := cache.NewListWatchFromClient(ew.client, "events", ew.namespace, fields.Everything())
	store, controller := cache.NewInformer(watchlist, &corev1.Event{}, 0, ew)
	ew._store = store
	ew._controller = controller
	ew.logger = log.With().Str("component", "watcher").Logger()

	ew._startTime = time.Now().UTC()

	ew.setupStats()

	go controller.Run(stopChan)
	ew.logger.Info().Msg("Watcher started")
	<-stopChan
}

func (ew *EventWatcher) setupStats() {
	ew.startTimeGauge = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "informer_start_time",
		Help: "Start time for the informer",
	})
	ew.startTimeGauge.Set(float64(ew._startTime.Unix()))

	ew.storeSizeGauge = promauto.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "informer_store_size",
		Help: "Number of items in store",
	}, func() float64 {
		return float64(len(ew._store.ListKeys()))
	})

	ew.addCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name: "informer_events_add_total",
		Help: "Number of new events received by the informer",
	})

	ew.updateCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name: "informer_events_update_total",
		Help: "Number of update events received by the informer",
	})

	ew.deleteCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name: "informer_events_delete_total",
		Help: "Number of delete events received by the informer",
	})

	ew.oldEventsCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name: "informer_events_old_total",
		Help: "Number of old events ignored by the informer",
	})
}

func (ew *EventWatcher) isOldEvent(event *corev1.Event) bool {
	// events after app startup time are eligible
	if event.LastTimestamp.Time.UTC().After(ew._startTime) {
		return false
	}
	// for events before start time, they need to be within threshold
	return ew._startTime.UTC().Sub(event.LastTimestamp.Time.UTC()) > oldEventAgeMinutes*time.Minute
}

func (ew *EventWatcher) OnAdd(obj interface{}) {
	event := obj.(*corev1.Event)
	if !ew.isOldEvent(event) {
		ew.logEvent(event, "Event added")
		atomic.AddInt32(&addCounter, 1)
		ew.addCounter.Inc()
	} else {
		ew.oldEventsCounter.Inc()
	}
	ew.deleteEvent(obj)
}

func (ew *EventWatcher) OnUpdate(oldObj, newObj interface{}) {
	event := newObj.(*corev1.Event)
	if !ew.isOldEvent(event) {
		ew.logEvent(event, "Event updated")
		atomic.AddInt32(&updateCounter, 1)
		ew.updateCounter.Inc()
	} else {
		ew.oldEventsCounter.Inc()
	}
	ew.deleteEvent(newObj)
}

func (ew *EventWatcher) OnDelete(obj interface{}) {
	event := obj.(*corev1.Event)
	if !ew.isOldEvent(event) {
		ew.logEvent(event, "Event deleted")
		atomic.AddInt32(&deleteCounter, 1)
		ew.deleteCounter.Inc()
	} else {
		ew.oldEventsCounter.Inc()
	}
	// ew.deleteEvent(obj)
}

func (ew *EventWatcher) deleteEvent(obj interface{}) {
	if err := ew._store.Delete(obj); err != nil {
		ew.logger.Error().Err(err).Msg("Could not delete object")
	}
}

func (ew *EventWatcher) logEvent(event *corev1.Event, message string) {
	ew.logger.Info().
		Str("namespace", event.Namespace).
		Str("name", event.Name).
		Str("version", event.ResourceVersion).
		Str("eventMsg", event.Message).
		Str("lastTimestamp", event.LastTimestamp.UTC().Format(time.RFC3339)).
		Str("age", time.Since(event.LastTimestamp.Time).Round(time.Second).String()).
		Int32("count", event.Count).
		Msg(message)
}
