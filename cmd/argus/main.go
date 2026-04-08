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

	"github.com/charliewilco/argus/config"
	"github.com/charliewilco/argus/internal/api"
	"github.com/charliewilco/argus/internal/connections"
	"github.com/charliewilco/argus/internal/dlq"
	"github.com/charliewilco/argus/internal/oauth"
	"github.com/charliewilco/argus/internal/queue"
	"github.com/charliewilco/argus/internal/store/sqlite"
	"github.com/charliewilco/argus/internal/triggers"
	"github.com/charliewilco/argus/providers"
	githubprovider "github.com/charliewilco/argus/providers/github"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("argus server failed: %v", err)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	app, err := newServerApp(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := app.Close(); closeErr != nil {
			log.Printf("argus server close failed: %v", closeErr)
		}
	}()

	return app.Run(ctx)
}

type serverApp struct {
	httpServer *http.Server
	closeStore func() error
}

func newServerApp(ctx context.Context) (*serverApp, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("cmd.argus.newServerApp: load config: %w", err)
	}

	store, err := sqlite.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("cmd.argus.newServerApp: open store: %w", err)
	}

	githubClient, err := githubprovider.NewProvider(githubprovider.Config{
		ClientID:      cfg.GitHub.ClientID,
		ClientSecret:  cfg.GitHub.ClientSecret,
		BaseURL:       cfg.BaseURL,
		WebhookSecret: cfg.GitHub.WebhookSecret,
	})
	if err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("cmd.argus.newServerApp: init github provider: %w", err)
	}

	providerRegistry, err := providers.NewRegistry(githubClient)
	if err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("cmd.argus.newServerApp: init provider registry: %w", err)
	}

	oauthManager, err := oauth.NewManager(store, oauth.Options{SecretKey: cfg.SecretKey})
	if err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("cmd.argus.newServerApp: init oauth manager: %w", err)
	}

	connectionService, err := connections.NewService(store, oauthManager, providerRegistry, time.Now)
	if err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("cmd.argus.newServerApp: init connection service: %w", err)
	}

	jobQueue := queue.NewMemoryQueue()
	triggerMatcher, err := triggers.NewTriggerMatcher(store)
	if err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("cmd.argus.newServerApp: init trigger matcher: %w", err)
	}

	dlqStore, err := dlq.NewStore(store, jobQueue, time.Now)
	if err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("cmd.argus.newServerApp: init dlq store: %w", err)
	}

	handler, err := api.NewRouter(api.RouterOptions{
		BaseURL:     cfg.BaseURL,
		TenantID:    cfg.TenantID,
		Now:         time.Now,
		OAuth:       oauthManager,
		Providers:   providerRegistry,
		Connections: connectionService,
		Pipelines:   store,
		Events:      store,
		Matcher:     triggerMatcher,
		Queue:       jobQueue,
		DLQ:         dlqStore,
	})
	if err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("cmd.argus.newServerApp: init router: %w", err)
	}

	return &serverApp{
		httpServer: &http.Server{
			Addr:    fmt.Sprintf(":%d", cfg.Port),
			Handler: handler,
		},
		closeStore: store.Close,
	}, nil
}

func (a *serverApp) Run(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		if err := a.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("cmd.argus.Run: listen and serve: %w", err)
			return
		}
		errCh <- nil
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := a.httpServer.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("cmd.argus.Run: shutdown: %w", err)
	}

	return <-errCh
}

func (a *serverApp) Close() error {
	if a == nil || a.closeStore == nil {
		return nil
	}

	return a.closeStore()
}
