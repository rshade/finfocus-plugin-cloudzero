package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/rs/zerolog"
	"github.com/rshade/finfocus-spec/sdk/go/pluginsdk"

	"github.com/rshade/finfocus-plugin-cloudzero/internal/config"
	"github.com/rshade/finfocus-plugin-cloudzero/internal/czclient"
	"github.com/rshade/finfocus-plugin-cloudzero/internal/plugin"
)

var version = "0.1.0"

func main() {
	if err := run(); err != nil {
		os.Exit(1)
	}
}

func run() error {
	flag.Parse()

	level := zerolog.InfoLevel
	if lvl := pluginsdk.GetLogLevel(); lvl != "" {
		if parsed, err := zerolog.ParseLevel(lvl); err == nil {
			level = parsed
		}
	}

	logger := pluginsdk.NewPluginLogger(plugin.PluginName, version, level, nil)

	port := pluginsdk.ParsePortFlag()
	if port == 0 {
		port = pluginsdk.GetPort()
	}

	if port > 0 {
		logger.Debug().Int("port", port).Msg("using configured port")
	} else {
		logger.Debug().Msg("using ephemeral port")
	}

	cfg, err := config.Load()
	if err != nil {
		logger.Error().Err(err).Msg("invalid configuration")
		return err
	}
	czClient := czclient.New(cfg, version, logger)
	czPlugin := plugin.New(version, cfg, czClient, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		logger.Info().Msg("received shutdown signal")
		cancel()
	}()

	serveConfig := pluginsdk.ServeConfig{
		Plugin:     czPlugin,
		Port:       port,
		PluginInfo: plugin.Info(version),
	}

	err = pluginsdk.Serve(ctx, serveConfig)
	if err != nil {
		logger.Error().Err(err).Msg("server error")
		return err
	}

	return nil
}
