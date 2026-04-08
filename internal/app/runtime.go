package app

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/charliewilco/argus/config"
	"github.com/charliewilco/argus/internal/actions"
	"github.com/charliewilco/argus/internal/connections"
	"github.com/charliewilco/argus/internal/dlq"
	"github.com/charliewilco/argus/internal/oauth"
	"github.com/charliewilco/argus/internal/pipeline"
	"github.com/charliewilco/argus/internal/queue"
	"github.com/charliewilco/argus/internal/store"
	"github.com/charliewilco/argus/internal/store/sqlite"
	"github.com/charliewilco/argus/internal/triggers"
	"github.com/charliewilco/argus/providers"
	providergithub "github.com/charliewilco/argus/providers/github"
)

type QueueFactory func(cfg config.Config) (queue.Queue, error)

type Options struct {
	QueueFactory QueueFactory
}

type Runtime struct {
	Config      config.Config
	Context     context.Context
	Store       store.Store
	Queue       queue.Queue
	Providers   *providers.Registry
	OAuth       *oauth.Manager
	Connections *connections.Service
	DLQ         *dlq.Store
	Matcher     *triggers.TriggerMatcher
	Dispatcher  *actions.Dispatcher
	Executor    *pipeline.Executor

	cancel context.CancelFunc
	once   sync.Once
}

func New(ctx context.Context, opts Options) (*Runtime, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("app.New: load config: %w", err)
	}

	return NewWithConfig(ctx, cfg, opts)
}

func NewWithConfig(ctx context.Context, cfg config.Config, opts Options) (*Runtime, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	runtimeCtx, cancel := context.WithCancel(ctx)
	closeWithCancel := func(run *Runtime, err error) (*Runtime, error) {
		cancel()
		if run != nil {
			_ = run.Close()
		}
		return nil, err
	}

	storeInstance, err := sqlite.Open(runtimeCtx, cfg.DatabaseURL)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("app.NewWithConfig: open sqlite store: %w", err)
	}

	queueFactory := opts.QueueFactory
	if queueFactory == nil {
		queueFactory = defaultQueueFactory
	}
	queueInstance, err := queueFactory(cfg)
	if err != nil {
		_ = storeInstance.Close()
		cancel()
		return nil, fmt.Errorf("app.NewWithConfig: build queue: %w", err)
	}

	provider, err := providergithub.NewProvider(providergithub.Config{
		ClientID:      cfg.GitHub.ClientID,
		ClientSecret:  cfg.GitHub.ClientSecret,
		BaseURL:       cfg.BaseURL,
		WebhookSecret: cfg.GitHub.WebhookSecret,
	})
	if err != nil {
		_ = storeInstance.Close()
		cancel()
		return nil, fmt.Errorf("app.NewWithConfig: build github provider: %w", err)
	}

	registry, err := providers.NewRegistry(provider)
	if err != nil {
		_ = storeInstance.Close()
		cancel()
		return nil, fmt.Errorf("app.NewWithConfig: build provider registry: %w", err)
	}

	oauthManager, err := oauth.NewManager(storeInstance, oauth.Options{SecretKey: cfg.SecretKey})
	if err != nil {
		_ = storeInstance.Close()
		cancel()
		return nil, fmt.Errorf("app.NewWithConfig: build oauth manager: %w", err)
	}

	connectionService, err := connections.NewService(storeInstance, oauthManager, registry, nil)
	if err != nil {
		return closeWithCancel(nil, fmt.Errorf("app.NewWithConfig: build connections service: %w", err))
	}

	dlqStore, err := dlq.NewStore(storeInstance, queueInstance, nil)
	if err != nil {
		return closeWithCancel(nil, fmt.Errorf("app.NewWithConfig: build DLQ store: %w", err))
	}

	matcher, err := triggers.NewTriggerMatcher(storeInstance)
	if err != nil {
		return closeWithCancel(nil, fmt.Errorf("app.NewWithConfig: build trigger matcher: %w", err))
	}

	dispatcher, err := actions.NewDispatcher(connectionService, registry, nil)
	if err != nil {
		return closeWithCancel(nil, fmt.Errorf("app.NewWithConfig: build action dispatcher: %w", err))
	}

	executor, err := pipeline.NewExecutor(dispatcher, dlqStore, nil)
	if err != nil {
		return closeWithCancel(nil, fmt.Errorf("app.NewWithConfig: build pipeline executor: %w", err))
	}

	return &Runtime{
		Config:      cfg,
		Context:     runtimeCtx,
		Store:       storeInstance,
		Queue:       queueInstance,
		Providers:   registry,
		OAuth:       oauthManager,
		Connections: connectionService,
		DLQ:         dlqStore,
		Matcher:     matcher,
		Dispatcher:  dispatcher,
		Executor:    executor,
		cancel:      cancel,
	}, nil
}

func (r *Runtime) Close() error {
	if r == nil {
		return nil
	}

	var err error
	r.once.Do(func() {
		r.cancel()
		err = errors.Join(err, r.Store.Close())
	})

	return err
}

func defaultQueueFactory(_ config.Config) (queue.Queue, error) {
	return queue.NewMemoryQueue(), nil
}
