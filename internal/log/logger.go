package log

// Logger defines a common interface shared by logging engines.
type Logger interface {
	// Debug logs a debug message.
	Debug(format string, v ...interface{})

	// Info logs an informational message.
	Info(format string, v ...interface{})

	// Warn logs a warning message.
	Warn(format string, v ...interface{})

	// Error logs an error message.
	Error(format string, v ...interface{})

	// Level returns the currently configured logging level.
	Level() Level
}
