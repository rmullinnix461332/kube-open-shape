package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kube-open-shape/kube-open-shape/internal/edge/api"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/collector"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/graph"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/janitor"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/knowledge"
	"github.com/kube-open-shape/kube-open-shape/internal/edge/store"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
)

func main() {
	logger := logrus.New()
	logger.SetFormatter(&logrus.TextFormatter{})
	logger.SetLevel(logrus.InfoLevel)

	// Configuration (env vars with defaults)
	apiAddr := envOrDefault("KOS_API_ADDR", ":9090")
	dbPath := envOrDefault("KOS_DB_PATH", "/tmp/kos/knowledge.db")

	// Build Kubernetes client
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, nil).ClientConfig()
	if err != nil {
		logger.Fatalf("Failed to load kubeconfig: %v", err)
	}

	dynClient, err := dynamic.NewForConfig(config)
	if err != nil {
		logger.Fatalf("Failed to create dynamic client: %v", err)
	}

	// Initialize knowledge index
	index := knowledge.NewIndex()

	// Initialize collector with default resources
	resources := collector.DefaultResources()
	coll := collector.New(dynClient, index, resources, logger)

	// Start collection
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	logger.Info("Starting kube-open-shape edge controller")
	if err := coll.Start(ctx); err != nil {
		logger.Fatalf("Failed to start collector: %v", err)
	}

	logger.WithField("resources", index.Count()).Info("Initial collection complete")

	// Initialize SQLite store
	st, err := store.New(dbPath)
	if err != nil {
		logger.WithError(err).Warn("Failed to open SQLite store, continuing without persistence")
		st = nil
	} else {
		logger.WithField("path", dbPath).Info("SQLite store opened")
	}

	// Initialize janitor engine with findings table
	var jan *janitor.Engine
	if st != nil {
		if err := st.MigrateFindings(); err != nil {
			logger.WithError(err).Warn("Failed to migrate findings table")
		} else {
			rules := janitor.DefaultRules()
			g := graph.Build(index)
			jan = janitor.NewEngine(rules, st, index, g, logger)

			// Run initial evaluation after collection
			logger.Info("Running initial janitor evaluation")
			if err := jan.Evaluate(); err != nil {
				logger.WithError(err).Warn("Initial janitor evaluation failed")
			}
		}
	}

	// Start HTTP API server
	server := api.NewServer(index, st, jan, apiAddr)
	go func() {
		logger.WithField("addr", apiAddr).Info("Starting HTTP API server")
		if err := server.Start(); err != nil {
			logger.WithError(err).Fatal("HTTP API server failed")
		}
	}()

	// Schedule periodic janitor re-evaluation (every 4 hours)
	if jan != nil {
		go func() {
			ticker := time.NewTicker(4 * time.Hour)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					logger.Info("Running periodic janitor evaluation")
					if err := jan.Evaluate(); err != nil {
						logger.WithError(err).Warn("Periodic janitor evaluation failed")
					}
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	// Wait for shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down")
	cancel()
	if st != nil {
		st.Close()
	}
	fmt.Printf("Final resource count: %d\n", index.Count())
}

func envOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
