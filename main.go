package main

import (
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gopkg.in/alecthomas/kingpin.v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	defaultKubeconfig   = "~/.kube/config"
	defaultStatsSeconds = 10
)

var (
	kubeconfig    = kingpin.Flag("kubeconfig", "Path to kubeconfig or set in env(KUBECONFIG)").Default(defaultKubeconfig).Short('k').Envar("KUBECONFIG").String()
	verbose       = kingpin.Flag("verbose", "Debug logging").Short('v').Bool()
	namespace     = kingpin.Flag("namespace", "Namespace").Default(corev1.NamespaceAll).Short('n').String()
	statsSeconds  = kingpin.Flag("stats-interval", "Seconds after which stats are printed").Default(strconv.Itoa(defaultStatsSeconds)).Short('s').Int()
	addCounter    int32
	updateCounter int32
	deleteCounter int32
)

func setup() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})
	log.Logger = log.Logger.Level(zerolog.InfoLevel)
	kingpin.CommandLine.HelpFlag.Short('h')
	kingpin.Parse()
	if *verbose {
		log.Logger = log.Logger.Level(zerolog.DebugLevel)
	}

	if strings.HasPrefix(*kubeconfig, "~/") {
		*kubeconfig = strings.Replace(*kubeconfig, "~/", os.Getenv("HOME")+"/", 1)
	}
}

func getKubeClient() *kubernetes.Clientset {
	// build config
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		log.Fatal().Err(err).Msg("Could not create kube config")
	}
	log.Debug().Msgf("API host: %v", config.Host)

	// get client from config
	return kubernetes.NewForConfigOrDie(config)
}

func logEvent(event *corev1.Event, message string) {
	log.Info().
		Str("namespace", event.Namespace).
		Str("name", event.Name).
		Str("version", event.ResourceVersion).
		Str("eventMsg", event.Message).
		Str("lastTimestamp", event.LastTimestamp.UTC().Format(time.RFC3339)).
		Int32("count", event.Count).
		Msg(message)
}

func printStats(store cache.Store, stopChan chan struct{}, wg *sync.WaitGroup) {
	defer wg.Done()
	if *statsSeconds <= 0 {
		log.Info().Msg("Disabling stats")
		return
	}

	ticker := time.NewTicker(time.Duration(*statsSeconds) * time.Second)
	log.Info().Msg("Starting stats")
	var prevAdd, prevUpdate, prevDelete int32
	for {
		select {
		case <-stopChan:
			log.Info().Msg("Stopping stats")
			ticker.Stop()
			return
		case <-ticker.C:
			addC, updateC, deleteC := atomic.LoadInt32(&addCounter), atomic.LoadInt32(&updateCounter), atomic.LoadInt32(&deleteCounter)
			log.Info().Msgf("STATS: Number of items in store: %d", len(store.ListKeys()))
			log.Info().Msgf("STATS: addCounter: %d, updateCounter: %d, deleteCounter: %d", addC, updateC, deleteC)
			log.Info().Msgf("STATS: added: %d, updated: %d, deleted: %d", addC-prevAdd, updateC-prevUpdate, deleteC-prevDelete)
			prevAdd, prevUpdate, prevDelete = addC, updateC, deleteC
		}
	}
}

func main() {
	setup()
	log.Info().Msgf("Using kubeconfig: %v", *kubeconfig)
	clientset := getKubeClient()
	// pods, err := clientset.CoreV1().Pods(*namespace).List(context.TODO(), metav1.ListOptions{})
	// if err != nil {
	// 	log.Error().Err(err).Msg("Could not list pods")
	// 	return
	// }
	// for _, pod := range pods.Items {
	// 	fmt.Println(pod.Namespace, "/", pod.Name, "/", pod.Spec.NodeName)
	// }

	watchlist := cache.NewListWatchFromClient(clientset.CoreV1().RESTClient(), "events", *namespace, fields.Everything())
	store, controller := cache.NewInformer(
		watchlist,
		&corev1.Event{},
		0,
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				event := obj.(*corev1.Event)
				logEvent(event, "Event added")
				atomic.AddInt32(&addCounter, 1)
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				event := newObj.(*corev1.Event)
				logEvent(event, "Event updated")
				atomic.AddInt32(&updateCounter, 1)
			},
			DeleteFunc: func(obj interface{}) {
				event := obj.(*corev1.Event)
				logEvent(event, "Event deleted")
				atomic.AddInt32(&deleteCounter, 1)
			},
		},
	)

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)

	stopChan := make(chan struct{})
	wg := new(sync.WaitGroup)
	go controller.Run(stopChan)
	wg.Add(1)
	go printStats(store, stopChan, wg)
	log.Info().Msg("Controller started")

	<-signalChan
	log.Warn().Msg("Signal to terminate received")
	close(stopChan)
	wg.Wait()

}
