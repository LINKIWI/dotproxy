package meta

import (
	"fmt"
	"io/ioutil"
	"time"

	"gopkg.in/yaml.v3"

	"dotproxy/internal/network"
)

// ApplicationConfig is a top-level block for application-level meta configuration.
type ApplicationConfig struct {
	SentryDSN string `yaml:"sentry_dsn"`
}

// MetricsConfig is a top-level block for metrics configuration.
type MetricsConfig struct {
	Statsd *struct {
		Address    string  `yaml:"addr"`
		SampleRate float64 `yaml:"sample_rate"`
	} `yaml:"statsd"`
}

// ListenerConfig is a top-level block for server listener configuration.
type ListenerConfig struct {
	TCP *struct {
		Address      string        `yaml:"addr"`
		ReadTimeout  time.Duration `yaml:"read_timeout"`
		WriteTimeout time.Duration `yaml:"write_timeout"`
	} `yaml:"tcp"`
	UDP *struct {
		Address                  string        `yaml:"addr"`
		MaxConcurrentConnections int           `yaml:"max_concurrent_connections"`
		ReadTimeout              time.Duration `yaml:"read_timeout"`
		WriteTimeout             time.Duration `yaml:"write_timeout"`
	} `yaml:"udp"`
}

// UpstreamServer describes parameters for a single upstream server.
type UpstreamServer struct {
	Address            string        `yaml:"addr"`
	ServerName         string        `yaml:"server_name"`
	ConnectionPoolSize int           `yaml:"connection_pool_size"`
	ConnectTimeout     time.Duration `yaml:"connect_timeout"`
	HandshakeTimeout   time.Duration `yaml:"handshake_timeout"`
	ReadTimeout        time.Duration `yaml:"read_timeout"`
	WriteTimeout       time.Duration `yaml:"write_timeout"`
	StaleTimeout       time.Duration `yaml:"stale_timeout"`
}

// UpstreamConfig is a top-level block for upstream configuration.
type UpstreamConfig struct {
	LoadBalancingPolicy  string           `yaml:"load_balancing_policy"`
	MaxConnectionRetries int              `yaml:"max_connection_retries"`
	Servers              []UpstreamServer `yaml:"servers"`
}

// Config describes all application configuration options.
type Config struct {
	Application *ApplicationConfig `yaml:"application"`
	Metrics     *MetricsConfig     `yaml:"metrics"`
	Listener    *ListenerConfig    `yaml:"listener"`
	Upstream    *UpstreamConfig    `yaml:"upstream"`
}

// ParseConfig parses a Config struct instance from a file specified as a path on disk.
func ParseConfig(path string) (*Config, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: error reading config: err=%v", err)
	}

	var cfg *Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("config: error parsing config: err=%v", err)
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// validate the contents of the configuration. Returns an error if validation failed; nil otherwise.
func (c *Config) validate() error {
	/* Metrics */

	// Users can omit the metrics block entirely to disable metrics reporting.
	if c.Metrics != nil && c.Metrics.Statsd != nil {
		if c.Metrics.Statsd.Address == "" {
			return fmt.Errorf("config: missing metrics statsd address")
		}

		if c.Metrics.Statsd.SampleRate < 0 || c.Metrics.Statsd.SampleRate > 1 {
			return fmt.Errorf("config: statsd sample rate must be in range [0.0, 1.0]")
		}
	}

	/* Listener */

	if c.Listener == nil {
		return fmt.Errorf("config: missing top-level listener config key")
	}

	if c.Listener.TCP == nil && c.Listener.UDP == nil {
		return fmt.Errorf("config: at least one TCP or UDP listener must be specified")
	}

	if c.Listener.TCP != nil && c.Listener.TCP.Address == "" {
		return fmt.Errorf("config: missing TCP server listening address")
	}

	if c.Listener.UDP != nil && c.Listener.UDP.Address == "" {
		return fmt.Errorf("config: missing UDP server listening address")
	}

	/* Upstream */

	if c.Upstream == nil {
		return fmt.Errorf("config: missing top-level upstream config key")
	}

	// Validate the load balancing policy, only if provided (empty signifies default).
	if c.Upstream.LoadBalancingPolicy != "" {
		if _, ok := network.ParseLoadBalancingPolicy(c.Upstream.LoadBalancingPolicy); !ok {
			return fmt.Errorf(
				"config: unknown load balancing policy: policy=%s",
				c.Upstream.LoadBalancingPolicy,
			)
		}
	}

	if len(c.Upstream.Servers) == 0 {
		return fmt.Errorf("config: no upstream servers specified")
	}

	for idx, server := range c.Upstream.Servers {
		if server.Address == "" {
			return fmt.Errorf("config: missing server address: idx=%d", idx)
		}

		if server.ServerName == "" {
			return fmt.Errorf("config: missing server TLS hostname: idx=%d", idx)
		}
	}

	return nil
}
