package logger

import (
	"fmt"
	"time"
)

// Define color constants using ANSI escape codes
const (
	Reset  = "\033[0m"
	Red    = "\033[31m"
	Green  = "\033[32m"
	Yellow = "\033[33m"
	Blue   = "\033[34m"
	Cyan   = "\033[36m"
)

// Info prints a blue informational message
func Info(format string, a ...interface{}) {
	prefix := fmt.Sprintf("%s[%s]%s ", Blue, time.Now().Format("15:04:05"), Reset)
	fmt.Printf(prefix+format+"\n", a...)
}

// Success prints a green success message
func Success(format string, a ...interface{}) {
	prefix := fmt.Sprintf("%s[+]%s ", Green, Reset)
	fmt.Printf(prefix+format+"\n", a...)
}

// Error prints a red error message
func Error(format string, a ...interface{}) {
	prefix := fmt.Sprintf("%s[!]%s ", Red, Reset)
	fmt.Printf(prefix+format+"\n", a...)
}

// Warning prints a yellow warning message
func Warning(format string, a ...interface{}) {
	prefix := fmt.Sprintf("%s[!]%s ", Yellow, Reset)
	fmt.Printf(prefix+format+"\n", a...)
}
