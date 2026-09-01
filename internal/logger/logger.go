package logger

// Logger interface defines the logging contract for the application
// This allows backend services to emit logs without direct dependencies on the UI layer
type Logger interface {
	// Debug logs verbose diagnostic information (only shown when debug mode is enabled)
	Debug(message string)

	// Info logs important operational events (service started, connected, etc.)
	Info(message string)

	// Warning logs issues that don't prevent operation but need attention
	Warning(message string)

	// Error logs errors that prevent normal operation
	Error(message string)
}
