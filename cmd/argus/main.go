package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/charliewilco/argus/internal/api"
	"github.com/charliewilco/argus/internal/app"
	"github.com/charliewilco/argus/internal/worker"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	runtime, err := app.New(ctx, app.Options{})
	if err != nil {
		log.Fatalf("argus: initialize runtime: %v", err)
	}
	defer func() {
		if closeErr := runtime.Close(); closeErr != nil {
			log.Printf("argus: runtime close failed: %v", closeErr)
		}
	}()

	router, err := api.NewRouter(api.RouterOptions{
		BaseURL:     runtime.Config.BaseURL,
		TenantID:    runtime.Config.TenantID,
		OAuth:       runtime.OAuth,
		Providers:   runtime.Providers,
		Connections: runtime.Connections,
		Pipelines:   runtime.Store,
		Events:      runtime.Store,
		Matcher:     runtime.Matcher,
		Queue:       runtime.Queue,
		DLQ:         runtime.DLQ,
	})
	if err != nil {
		log.Fatalf("argus: create router: %v", err)
	}

	executionWorker, err := worker.New(runtime.Queue, runtime.Store, runtime.Executor, runtime.DLQ, log.Default(), nil)
	if err != nil {
		log.Fatalf("argus: create worker: %v", err)
	}

	go func() {
		if runErr := executionWorker.Run(runtime.Context); runErr != nil && !errors.Is(runErr, context.Canceled) {
			log.Printf("argus: worker stopped with error: %v", runErr)
		}
	}()

	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", runtime.Config.Port),
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		<-runtime.Context.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if shutdownErr := server.Shutdown(shutdownCtx); shutdownErr != nil {
			log.Printf("argus: graceful shutdown failed: %v", shutdownErr)
		}
	}()

	log.Printf("argus listening on %s", server.Addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("argus: serve: %v", err)
	}
}
