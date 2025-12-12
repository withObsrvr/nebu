package main

import (
	"fmt"
	"os"
)

// Global flag for quiet mode
var quietMode bool

// logInfo writes informational/progress messages to stderr.
// These messages are suppressed when --quiet flag is set.
func logInfo(format string, args ...interface{}) {
	if !quietMode {
		fmt.Fprintf(os.Stderr, format+"\n", args...)
	}
}

// logError writes error messages to stderr.
// These messages are always shown, even in quiet mode.
func logError(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
}

// logWarning writes warning messages to stderr.
// These messages are always shown, even in quiet mode.
func logWarning(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "Warning: "+format+"\n", args...)
}
