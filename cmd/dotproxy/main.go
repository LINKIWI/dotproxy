package main

import (
	"flag"
	"fmt"
	"os"

	"dotproxy/internal/log"
	"dotproxy/internal/meta"
	"dotproxy/internal/metrics"
	"dotproxy/internal/network"
	"dotproxy/internal/protocol"

	"github.com/getsentry/raven-go"
)

func main() {
	configPath := flag.String(
		"config",
		os.Getenv("DOTPROXY_CONFIG"),
		"path to the configuration file on disk",
	)
	version := flag.Bool(
		"version",
		false,
		"print the compiled dotproxy version SHA",
	)
	verbosity := flag.String(
		"verbosity",
		"error",
		"desired logging verbosity: one of error, warn, info, debug",
	)
	flag.Parse()

	// Report the compiled version and exit
	if *version {
		fmt.Printf("dotproxy/%s\n", meta.VersionSHA)
		return
	}

	// Logging configuration; default to log.Error verbosity
	level, _ := log.ParseLevel(*verbosity)
	logger := log.NewConsoleLogger(level)
	logger.Debug("main: initialized logger: level=%v", level)

	// Parse application configuration
	logger.Debug("main: reading and parsing config: path=%s", *configPath)
	config, err := meta.ParseConfig(*configPath)
	if err != nil {
		panic(err)
	}

	// Configure error reporting
	if config.Application != nil && config.Application.SentryDSN != "" {
		raven.SetDSN(config.Application.SentryDSN)
		raven.SetRelease(meta.VersionSHA)
	}

	// Configure metrics reporting
	clientCxLifecycleHook := metrics.NewNoopConnectionLifecycleHook()
	upstreamCxLifecycleHook := metrics.NewNoopConnectionLifecycleHook()
	clientCxIOHook := metrics.NewNoopConnectionIOHook()
	upstreamCxIOHook := metrics.NewNoopConnectionIOHook()
	proxyHook := metrics.NewNoopProxyHook()

	if config.Metrics != nil && config.Metrics.Statsd != nil {
		logger.Info(
			"main: configuring statsd metrics reporting: addr=%s sample_rate=%f",
			config.Metrics.Statsd.Address,
			config.Metrics.Statsd.SampleRate,
		)

		if clientCxLifecycleHook, err = metrics.NewAsyncStatsdConnectionLifecycleHook(
			"client",
			config.Metrics.Statsd.Address,
			config.Metrics.Statsd.SampleRate,
			meta.VersionSHA,
		); err != nil {
			panic(err)
		}

		if upstreamCxLifecycleHook, err = metrics.NewAsyncStatsdConnectionLifecycleHook(
			"upstream",
			config.Metrics.Statsd.Address,
			config.Metrics.Statsd.SampleRate,
			meta.VersionSHA,
		); err != nil {
			panic(err)
		}

		if clientCxIOHook, err = metrics.NewAsyncStatsdConnectionIOHook(
			"client",
			config.Metrics.Statsd.Address,
			config.Metrics.Statsd.SampleRate,
			meta.VersionSHA,
		); err != nil {
			panic(err)
		}

		if upstreamCxIOHook, err = metrics.NewAsyncStatsdConnectionIOHook(
			"upstream",
			config.Metrics.Statsd.Address,
			config.Metrics.Statsd.SampleRate,
			meta.VersionSHA,
		); err != nil {
			panic(err)
		}

		if proxyHook, err = metrics.NewAsyncStatsdProxyHook(
			config.Metrics.Statsd.Address,
			config.Metrics.Statsd.SampleRate,
			meta.VersionSHA,
		); err != nil {
			panic(err)
		}
	} else {
		logger.Warn("main: no metrics output engine specified; disabling metrics")
	}

	// Configure upstreams
	var servers []network.Client
	for _, server := range config.Upstream.Servers {
		opts := network.TLSClientOpts{
			ConnectTimeout:   server.ConnectTimeout,
			HandshakeTimeout: server.HandshakeTimeout,
			ReadTimeout:      server.ReadTimeout,
			WriteTimeout:     server.WriteTimeout,
			PoolOpts: network.PersistentConnPoolOpts{
				Capacity:     server.ConnectionPoolSize,
				StaleTimeout: server.StaleTimeout,
			},
		}

		logger.Info(
			"main: starting TLS client for upstream server: addr=%s name=%s conns=%d",
			server.Address,
			server.ServerName,
			opts.PoolOpts.Capacity,
		)

		client, err := network.NewTLSClient(
			server.Address,
			server.ServerName,
			upstreamCxLifecycleHook,
			opts,
		)

		if err != nil {
			panic(err)
		}

		servers = append(servers, client)
	}

	// Create sharded client for all upstreams
	lbPolicy, ok := network.ParseLoadBalancingPolicy(config.Upstream.LoadBalancingPolicy)
	if !ok {
		logger.Warn(
			"main: unknown load balancing policy; use default: supplied=%s default=%s",
			config.Upstream.LoadBalancingPolicy,
			lbPolicy,
		)
	}

	logger.Debug("main: using load balancing policy for request sharding: policy=%s", lbPolicy)
	client, _ := network.NewShardedClient(servers, lbPolicy)

	// Configure server listeners
	h := &protocol.DNSProxyHandler{
		Upstream:         client,
		ClientCxIOHook:   clientCxIOHook,
		UpstreamCxIOHook: upstreamCxIOHook,
		ProxyHook:        proxyHook,
		Logger:           logger,
		Opts: protocol.DNSProxyOpts{
			MaxUpstreamRetries: config.Upstream.MaxConnectionRetries,
		},
	}

	if config.Listener.UDP != nil {
		logger.Info(
			"main: configuring UDP server listener: addr=%s max_concurrent_conns=%d",
			config.Listener.UDP.Address,
			config.Listener.UDP.MaxConcurrentConnections,
		)

		opts := network.UDPServerOpts{
			MaxConcurrentConnections: config.Listener.UDP.MaxConcurrentConnections,
			ReadTimeout:              config.Listener.UDP.ReadTimeout,
			WriteTimeout:             config.Listener.UDP.WriteTimeout,
		}

		udpServer := network.NewUDPServer(config.Listener.UDP.Address, opts)

		go func() {
			if err := udpServer.ListenAndServe(h); err != nil {
				panic(err)
			}
		}()
	}

	if config.Listener.TCP != nil {
		logger.Info(
			"main: configuring TCP server listener: addr=%s",
			config.Listener.TCP.Address,
		)

		opts := network.TCPServerOpts{
			ReadTimeout:  config.Listener.TCP.ReadTimeout,
			WriteTimeout: config.Listener.TCP.WriteTimeout,
		}

		tcpServer := network.NewTCPServer(
			config.Listener.TCP.Address,
			clientCxLifecycleHook,
			opts,
		)

		go func() {
			if err := tcpServer.ListenAndServe(h); err != nil {
				panic(err)
			}
		}()
	}

	// Serve indefinitely
	logger.Info("main: serving indefinitely")
	<-make(chan bool)
}
