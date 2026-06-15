package logger

import (
	"fmt"
	"os"
	"time"
)

// Level represents how serious a log message is.
// We define it as a custom type based on int.
// This is better than using plain integers because the compiler
// will catch mistakes like passing a random number where a Level is expected.
type Level int

// These are the four log levels we support.
// iota is a Go keyword that auto-increments: DEBUG=0, INFO=1, WARN=2, ERROR=3
// This means DEBUG is the lowest (most verbose) and ERROR is the highest (most serious).
const (
	DEBUG Level = iota
	INFO
	WARN
	ERROR
)

// ANSI color codes.
// These are special escape sequences that terminals understand.
// When printed, they change the color of the text that follows.
// "\033[" starts the escape sequence, the number picks the color, "m" ends it.
// "\033[0m" resets back to the default terminal color.
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
	colorGray   = "\033[90m"
)

// Logger is our main struct.
// A struct in Go is like a class in other languages — it groups related data together.
// This struct holds the configuration for our logger.
type Logger struct {
	// level is the minimum level to print.
	// If level is WARN, then DEBUG and INFO messages are silently ignored.
	level Level

	// debugMode is a separate flag specifically for debug output.
	// We keep it separate so you can enable debug without changing the level filter logic.
	debugMode bool
}

// New creates and returns a new Logger.
// This is the constructor pattern in Go.
// Go doesn't have classes or constructors, so by convention we write a function
// called New() that returns a ready-to-use instance of a struct.
func New(debugMode bool) *Logger {
	// Set the default level based on whether debug mode is on.
	level := INFO
	if debugMode {
		level = DEBUG
	}

	// The & operator returns a pointer to the Logger struct.
	// We return a pointer (*Logger) so the caller shares the same instance
	// instead of getting a copy. This is important for consistency.
	return &Logger{
		level:     level,
		debugMode: debugMode,
	}
}

// log is the internal (private) function that actually prints the message.
// Notice it starts with a lowercase letter — in Go, lowercase = private to this package.
// Only functions inside the logger package can call this.
func (l *Logger) log(lvl Level, color string, label string, msg string) {
	// If the message level is below our minimum, don't print it.
	// Example: if logger level is WARN, a DEBUG message is skipped.
	if lvl < l.level {
		return
	}

	// Get the current time and format it as HH:MM:SS
	// time.Now() returns the current local time.
	// .Format() takes a layout string — Go uses a reference time (Jan 2 15:04:05 2006)
	// instead of strftime codes like %H:%M:%S. This is unusual but intentional in Go.
	timestamp := time.Now().Format("15:04:05")

	// Build the final formatted message.
	// %s is the placeholder for a string argument.
	// We combine: gray timestamp + color + label + reset + message
	line := fmt.Sprintf("%s[%s]%s %s[%s]%s %s",
		colorGray, timestamp, colorReset,
		color, label, colorReset,
		msg,
	)

	// ERROR messages go to stderr (os.Stderr), everything else to stdout (os.Stdout).
	// Why? Because in Unix pipelines, stderr and stdout are separate streams.
	// If someone pipes your output to another tool, errors won't pollute the data stream.
	if lvl == ERROR {
		fmt.Fprintln(os.Stderr, line)
	} else {
		fmt.Fprintln(os.Stdout, line)
	}
}

// Debug prints a debug-level message.
// The (l *Logger) part is called a "receiver" — it means this function
// belongs to the Logger type. This is Go's version of a method.
func (l *Logger) Debug(format string, args ...any) {
	// args ...any means "zero or more arguments of any type"
	// This lets us call Debug("found %d urls", 42) like fmt.Printf
	msg := fmt.Sprintf(format, args...)
	l.log(DEBUG, colorGray, "DBG", msg)
}

// Info prints an informational message.
func (l *Logger) Info(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	l.log(INFO, colorGreen, "INF", msg)
}

// Warn prints a warning message.
func (l *Logger) Warn(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	l.log(WARN, colorYellow, "WRN", msg)
}

// Error prints an error message to stderr.
func (l *Logger) Error(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	l.log(ERROR, colorRed, "ERR", msg)
}

// Fatal prints an error message then exits the program with code 1.
// Use this for unrecoverable errors where continuing makes no sense.
func (l *Logger) Fatal(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	l.log(ERROR, colorRed, "FTL", msg)
	// os.Exit(1) immediately stops the program.
	// Exit code 1 is the Unix convention for "something went wrong".
	os.Exit(1)
}
