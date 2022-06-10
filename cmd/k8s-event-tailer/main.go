package main

import (
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gopkg.in/alecthomas/kingpin.v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	defaultKubeconfig   = "~/.kube/config"
	defaultStatsSeconds = 10
	defaultPort         = 8000
)

var (
	kubeconfig   = kingpin.Flag("kubeconfig", "Path to kubeconfig or set in env(KUBECONFIG)").Default(defaultKubeconfig).Short('k').Envar("KUBECONFIG").String()
	verbose      = kingpin.Flag("verbose", "Debug logging").Short('v').Bool()
	namespace    = kingpin.Flag("namespace", "Namespace").Default(corev1.NamespaceAll).Short('n').String()
	statsSeconds = kingpin.Flag("stats-interval", "Seconds after which stats are printed").Default(strconv.Itoa(defaultStatsSeconds)).Short('s').Int()
	port         = kingpin.Flag("port", "HTTP port for metrics").Default(strconv.Itoa(defaultPort)).Short('p').Int()

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

	// create client from config
	return kubernetes.NewForConfigOrDie(config)
}

func main() {
	setup()
	log.Info().Msgf("Using kubeconfig: %v", *kubeconfig)
	clientset := getKubeClient()
	watcher := EventWatcher{
		client:               clientset.CoreV1().RESTClient(),
		namespace:            *namespace,
		statsIntervalSeconds: *statsSeconds,
	}

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)

	stopChan := make(chan struct{})
	wg := new(sync.WaitGroup)

	wg.Add(1)
	go watcher.Run(stopChan, wg)

	wg.Add(1)
	go NewWebServer(*port).Run(stopChan, wg)

	<-signalChan
	log.Warn().Msg("Signal to terminate received")
	close(stopChan)
	wg.Wait()

}
