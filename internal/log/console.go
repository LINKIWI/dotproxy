package log

import (
	"fmt"
	"time"
)

// ConsoleLogger is a simple, leveled, standard output logging engine.
type ConsoleLogger struct {
	level Level
}

// NewConsoleLogger creates a logger limited to the specified level. Only log messages that are less
// verbose than the specified level are logged.
func NewConsoleLogger(level Level) Logger {
	return &ConsoleLogger{level}
}

// Debug logs a debug message, if permitted by the current level.
func (l *ConsoleLogger) Debug(format string, v ...interface{}) {
	l.log(Debug, format, v...)
}

// Info logs an informational message, if permitted by the current level.
func (l *ConsoleLogger) Info(format string, v ...interface{}) {
	l.log(Info, format, v...)
}

// Warn logs a warning message, if permitted by the current level.
func (l *ConsoleLogger) Warn(format string, v ...interface{}) {
	l.log(Warn, format, v...)
}

// Error logs an error message, if permitted by the current level.
func (l *ConsoleLogger) Error(format string, v ...interface{}) {
	l.log(Error, format, v...)
}

// Level reads the current logging level.
func (l *ConsoleLogger) Level() Level {
	return l.level
}

// log logs a message to standard output with a timestamp and level indicator, if permitted by the
// current level.
func (l *ConsoleLogger) log(level Level, format string, v ...interface{}) {
	if l.level.Enables(level) {
		fmt.Printf(
			"%s %s\t%s\n",
			time.Now().Format("2006-01-02 15:04:05"),
			level,
			fmt.Sprintf(format, v...),
		)
	}
}
