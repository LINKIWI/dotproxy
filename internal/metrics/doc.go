// Package metrics contains abstractions for emission of metrics generated throughout the lifetime
// of the application. Currently, the only supported metrics output engine is statsd.
//
// The nature of the application is such that metrics are generated at various points in time
// throughout a single request lifecycle. Thus, the metrics emissions in this package are structured
// around the notion of hooks: a hook interface defines methods that are invoked by the server's
// main logic routines while serving a client request. Thus, they "hook" into lifecycle points in
// logic. Implementations of hook interfaces actually output the metrics to a backend engine; this
// responsibility is decoupled from the semantics of "hooking" into business logic.
package metrics
