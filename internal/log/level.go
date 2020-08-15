//go:generate go run golang.org/x/tools/cmd/stringer -type=Level -linecomment=true

package log

import (
	"strings"
)

// Level parametrizes supported log verbosity levels.
type Level int

const (
	// Debug messages trace application-level behaviors.
	Debug Level = iota // DEBUG
	// Info messages convey general events.
	Info // INFO
	// Warn messages describe non-erroring divergences from the ideal code path.
	Warn // WARN
	// Error messages indicate behavior that is not intended and should be corrected.
	Error // ERROR
)

// ParseLevel looks up a Level constant by its stringified (case-insensitive) representation.
func ParseLevel(level string) (Level, bool) {
	knownLevels := []Level{Debug, Info, Warn, Error}

	for _, knownLevel := range knownLevels {
		if strings.ToLower(level) == strings.ToLower(knownLevel.String()) {
			return knownLevel, true
		}
	}

	return Error, false
}

// Enables indicates whether the current log level enables logging at another level.
//
// For example,
//	Debug enables Debug, Info, Warn, and Error
//	Info enables Warn and Error, but not Debug
//	Error enables Error, but not Debug, Info, or Warn
func (l Level) Enables(other Level) bool {
	return l <= other
}
