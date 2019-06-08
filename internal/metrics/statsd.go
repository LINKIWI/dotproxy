package metrics

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/cactus/go-statsd-client/statsd"
)

// StatsdClient is an abstraction over a UDP statsd emitter.
type StatsdClient struct {
	backend     statsd.Statter
	defaultTags map[string]string
	sampleRate  float32
}

// NewStatsdClient creates a new statsd client pointing the specified listener/server address with
// an optional prefix and set of default tags to include with every metric.
func NewStatsdClient(addr string, prefix string, defaultTags map[string]string, sampleRate float32) (*StatsdClient, error) {
	client, err := statsd.NewClient(addr, prefix)
	if err != nil {
		return nil, fmt.Errorf("statsd: error creating statsd client: err=%v", err)
	}

	return &StatsdClient{
		backend:     client,
		defaultTags: defaultTags,
		sampleRate:  sampleRate,
	}, nil
}

// Count emits a count metric with a configurable delta.
func (c *StatsdClient) Count(metric string, delta int64, tags map[string]string) error {
	return c.backend.Inc(c.formatMetric(metric, tags), delta, c.sampleRate)
}

// Gauge emits a gauge metric.
func (c *StatsdClient) Gauge(metric string, value int64, tags map[string]string) error {
	return c.backend.Gauge(c.formatMetric(metric, tags), value, c.sampleRate)
}

// Timing emits a time duration metric.
func (c *StatsdClient) Timing(metric string, duration time.Duration, tags map[string]string) error {
	return c.backend.TimingDuration(c.formatMetric(metric, tags), duration, c.sampleRate)
}

// Size emits a file size metric as the number of bytes.
func (c *StatsdClient) Size(metric string, size int64, tags map[string]string) error {
	// Size metrics share the same semantics with timing metrics; they are interpreted and
	// aggregated in the same way.
	return c.backend.Timing(c.formatMetric(metric, tags), size, c.sampleRate)
}

// formatMetric serializes a metric and a map of tags (in addition to any default tags) into a
// single string to ship to the time-series database backend.
func (c *StatsdClient) formatMetric(metric string, tags map[string]string) string {
	// Some characters, like colons, are incompatible with the statsd protocol.
	// This standardizes on URL escaping to encode such characters that may appear in the metric
	// name or tag keys/values.
	escapedMetric := url.QueryEscape(metric)

	if tags == nil {
		tags = make(map[string]string)
	}

	if len(c.defaultTags)+len(tags) == 0 {
		return escapedMetric
	}

	// Merge specified tags with the default tags, if available.
	mergedTags := make(map[string]string)
	for key, value := range c.defaultTags {
		mergedTags[key] = value
	}
	for key, value := range tags {
		mergedTags[key] = value
	}

	// Tags are delimited InfluxDB-style.
	var components []string
	for key, value := range mergedTags {
		components = append(
			components,
			fmt.Sprintf("%s=%s", url.QueryEscape(key), url.QueryEscape(value)),
		)
	}

	return fmt.Sprintf("%s,%s", escapedMetric, strings.Join(components, ","))
}
