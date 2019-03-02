//go:generate stringer -type=Level -linecomment=true

package log

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

// Enables indicates whether the current log level enables logging at another level.
//
// For example,
//	Debug enables Debug, Info, Warn, and Error
//	Info enables Warn and Error, but not Debug
//	Error enables Error, but not Debug, Info, or Warn
func (l Level) Enables(other Level) bool {
	return l <= other
}
