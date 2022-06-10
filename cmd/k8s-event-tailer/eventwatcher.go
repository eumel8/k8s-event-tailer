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
	client               rest.Interface
	namespace            string
	statsIntervalSeconds int
	logger               zerolog.Logger

	_startTime       time.Time
	_store           cache.Store
	_controller      cache.Controller
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
	ew.setupStats()
	ew._startTime = time.Now().UTC()

	go controller.Run(stopChan)
	ew.logger.Info().Msg("Watcher started")

	iwg := new(sync.WaitGroup)
	iwg.Add(1)
	go ew.runStatsPrint(stopChan, iwg)
	iwg.Wait()
}

func (ew *EventWatcher) setupStats() {
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

func (ew *EventWatcher) OnAdd(obj interface{}) {
	event := obj.(*corev1.Event)
	if event.LastTimestamp.Time.UTC().After(ew._startTime) || time.Since(event.LastTimestamp.Time.UTC()) < oldEventAgeMinutes*time.Minute {
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
	if event.LastTimestamp.Time.UTC().After(ew._startTime) || time.Since(event.LastTimestamp.Time.UTC()) < oldEventAgeMinutes*time.Minute {
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
	if event.LastTimestamp.Time.UTC().After(ew._startTime) || time.Since(event.LastTimestamp.Time.UTC()) < oldEventAgeMinutes*time.Minute {
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

func (ew *EventWatcher) runStatsPrint(stopChan chan struct{}, wg *sync.WaitGroup) {
	defer wg.Done()
	if ew.statsIntervalSeconds <= 0 {
		ew.logger.Info().Msg("Disabling stats")
		return
	}

	ticker := time.NewTicker(time.Duration(ew.statsIntervalSeconds) * time.Second)
	ew.logger.Info().Msg("Starting stats")
	var prevAdd, prevUpdate, prevDelete int32
	for {
		select {
		case <-stopChan:
			ew.logger.Info().Msg("Stopping stats")
			ticker.Stop()
			return
		case <-ticker.C:
			addC, updateC, deleteC := atomic.LoadInt32(&addCounter), atomic.LoadInt32(&updateCounter), atomic.LoadInt32(&deleteCounter)
			ew.logger.Info().Msgf("STATS: Number of items in store: %d", len(ew._store.ListKeys()))
			ew.logger.Info().Msgf("STATS: addCounter: %d, updateCounter: %d, deleteCounter: %d", addC, updateC, deleteC)
			ew.logger.Info().Msgf("STATS: added: %d, updated: %d, deleted: %d", addC-prevAdd, updateC-prevUpdate, deleteC-prevDelete)
			prevAdd, prevUpdate, prevDelete = addC, updateC, deleteC
		}
	}
}
