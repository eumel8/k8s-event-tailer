package main

import (
	"context"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type WebServer struct {
	server           *http.Server
	logger           zerolog.Logger
	storeListHandler http.Handler
}

func NewWebServer(port int) *WebServer {
	ws := &WebServer{
		server: &http.Server{
			Addr: fmt.Sprintf(":%d", port),
		},
		logger: log.With().Str("component", "web").Logger(),
	}
	http.HandleFunc("/healthz", ws.healthHandler)
	http.Handle("/metrics", promhttp.Handler())
	return ws
}

func (ws *WebServer) Run(stopchan chan struct{}, wg *sync.WaitGroup) {
	ws.logger.Info().Msgf("Starting web server listening to %s", ws.server.Addr)
	ctx, cancel := context.WithCancel(context.Background())
	go ws.stop(ctx, wg)
	go func() {
		if err := ws.server.ListenAndServe(); err != http.ErrServerClosed {
			ws.logger.Err(err).Msg("Error stopping webserver")
		}
	}()
	<-stopchan
	cancel()
}

func (ws *WebServer) SetStoreListHandler(handler http.HandlerFunc) {
	ws.storeListHandler = handler
	http.Handle("/store", ws.storeListHandler)
}

func (ws *WebServer) stop(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	<-ctx.Done()
	stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := ws.server.Shutdown(stopCtx); err != nil && err != http.ErrServerClosed {
		ws.logger.Err(err).Send()
	}
	ws.logger.Info().Msg("Shut down web server")
}

func (ws *WebServer) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", "application/json; charset=UTF-8")
	_, _ = w.Write([]byte(`{"status": "GOOD"}` + "\n"))
}
