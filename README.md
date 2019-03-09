# dotproxy

**dotproxy** is a high-performance and fault-tolerant DNS-over-TLS proxy. It listens on both TCP and UDP transports and proxies DNS traffic transparently to configurable TLS-enabled upstream server(s).

dotproxy is intended to sit at the edge of a private network, encrypting traffic over an untrusted channel to and from external, public DNS servers like [Cloudflare DNS](https://developers.cloudflare.com/1.1.1.1/dns-over-tls/) or [Google DNS](https://developers.google.com/speed/public-dns/docs/dns-over-tls). As a plaintext-to-TLS proxy, dotproxy can be *transparently* inserted into existing network infrastructure without requiring DNS reconfiguration on existing clients.

## Features

* Intelligent client-side connection persistence and pooling to minimize TCP and TLS latency overhead
* Rudimentary load balancing policy among multiple upstream servers
* Rich metrics reporting via `statsd`: connection establishment/teardown events, network I/O events, upstream latency, and RTT latency
* Supports both TCP and UDP ingress (with automatic spec-compliant data reshaping to support UDP ingress to TCP/TLS egress, and vice versa)

dotproxy is stateless and generally not protocol-aware. This sacrifies some features (like upstream response caching behavior or domain-aware load balancing/sharding) in favor of slightly reduced proxy latency overhead (by not parsing request and response packets).

## Performance

dotproxy maintains a pool of persistent, long-lived TCP connections to upstream server(s). This helps amortize the cost of establishing TCP connections and performing TLS handshakes with the server, thus providing the client near-UDP levels of performance. Additionally, most network behavior parameters are exposed in application configuration, allowing for the proxy to be performance-tuned specifically for the deployment's environment.

Networks characterized by high request volume (in terms of QPS) will generally benefit from a larger upstream connection pool. On the other hand, networks characterized by low request volume will generally benefit from a smaller upstream connection pool; too large of a connection pool will decrease average performance due to excessive connection churn from server-side TCP timeouts. Cloudflare's DNS servers, for example, close client TCP connections after a 10 second period of inactivity.

Most use cases will benefit from a large number of maximum concurrent ingress UDP connections. Generally speaking, this value should be set to a responsible estimate of highest number of concurrent UDP clients.

## Usage

Download a precompiled binary for the target platform/architecture at the [releases index](https://dotproxy.static.kevinlin.info/releases/latest). Currently, binaries are built for most flavors of Linux.

Alternatively, to compile the project manually with a recent version of the Go toolchain:

```bash
$ make
$ ./bin/dotproxy-$OS-$ARCH --help
```

The versioned `systemd` unit file can serve as an example for how to daemonize the process.

## Configuration

### Configuration file

dotproxy must be passed a YAML configuration file path with the `--config` flag. The versioned `config.example.yaml` in the repository root can serve as an example of a valid configuration file.

The following table documents each field and its expected value:

|Key|Required|Description|
|-|-|-|
|`metrics.statsd.addr`|No|Address of the statsd server for metrics reporting|
|`metrics.statsd.sample_rate`|No|statsd sample rate, if enabled|
|`listener.tcp.addr`|Yes|Address to bind to for the TCP listener|
|`listener.tcp.read_timeout`|No|Time duration string for a client TCP read timeout|
|`listener.tcp.write_timeout`|No|Time duration string for a client TCP write timeout|
|`listener.udp.addr`|Yes|Address to bind to for the UDP listener|
|`listener.udp.read_timeout`|No|Time duration string for a client UDP read timeout; should generally be omitted or set to 0|
|`listener.udp.write_timeout`|No|Time duration string for a client UDP write timeout|
|`upstream.load_balacing_policy`|No|One of the `LoadBalancingPolicy` constants to control how requests are sharded among all specified upstream servers|
|`upstream.max_connection_retries`|No|Maximum number of times to retry an upstream I/O operation, per request|
|`upstream.servers[].addr`|Yes|The address of the upstream TLS-enabled DNS server|
|`upstream.servers[].server_name`|Yes|The TLS server hostname (used for server identity verification)|
|`upstream.servers[].connection_pool_size`|No|Size of the connection pool to maintain for this server; environments with high traffic and/or request concurrency will generally benefit from a larger connection pool|
|`upstream.servers[].connect_timeout`|No|Time duration string for an upstream TCP connection establishment timeout|
|`upstream.servers[].handshake_timeout`|No|Time duration string for an upstream TLS handshake timeout|
|`upstream.servers[].read_timeout`|No|Time duration string for an upstream TCP read timeout|
|`upstream.servers[].write_timeout`|No|Time duration string for an upstream TCP write timeout|
|`upstream.servers[].stale_timeout`|No|Time duration string describing the interval of time between consecutive open connection uses after which it should be considered stale and reestablished|

### Load balancing policies

When there exists more than one upstream DNS server in configuration, the `upstream.load_balancing_policy` field controls how dotproxy shards requests among the servers. The policies below are mostly stateless and protocol-agnostic.

|Policy|Description|
|-|-|
|`RoundRobin`|Select servers in [round-robin](https://en.wikipedia.org/wiki/Round-robin_scheduling), circular order. Simple, fair, but not fault tolerant.|
|`Random`|Select a server at random. Simple, fair, async-safe, but not fault tolerant.|
|`HistoricalConnections`|Select the server that has, up until the time of request, provided the fewest number of connections. Ideal if it is important that all servers share an equal amount of load, without regard to fault tolerance.|
|`Availability`|Randomly select an available server. A server is considered *available* if it is successful in providing a connection. Servers that fail to provide a connection are pulled out of the availability pool for exponentially increasing durations of time, preventing them from providing connections until their unavailability period has expired. Ideal for greatest fault tolerance while maintaining roughly equal load distribution and minimizing downstream latency impact, at the cost of running potentially expensive logic every time a connection is requested.|
|`Failover`|Prioritize a single primary server and failover to secondary server(s) only when the primary fails. Ideal if one server should serve all traffic, but there is a need for fault tolerance.|
