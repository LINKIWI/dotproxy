package log

import (
	"fmt"
	"time"
)

// Logger is a simple, leveled, standard output logging engine.
type Logger struct {
	level Level
}

// NewLogger creates a logger limited to the specified level. Only log messages that are less
// verbose than the specified level are logged.
func NewLogger(level Level) *Logger {
	return &Logger{level}
}

// Debug logs a debug message, if permitted by the current level.
func (l *Logger) Debug(format string, v ...interface{}) {
	l.log(Debug, format, v...)
}

// Info logs an informational message, if permitted by the current level.
func (l *Logger) Info(format string, v ...interface{}) {
	l.log(Info, format, v...)
}

// Warn logs a warning message, if permitted by the current level.
func (l *Logger) Warn(format string, v ...interface{}) {
	l.log(Warn, format, v...)
}

// Error logs an error message, if permitted by the current level.
func (l *Logger) Error(format string, v ...interface{}) {
	l.log(Error, format, v...)
}

// log logs a message to standard output with a timestamp and level indicator, if permitted by the
// current level.
func (l *Logger) log(level Level, format string, v ...interface{}) {
	if l.level.Enables(level) {
		fmt.Printf(
			"%s %s\t%s\n",
			time.Now().Format("2006-01-02 15:04:05"),
			level,
			fmt.Sprintf(format, v...),
		)
	}
}
