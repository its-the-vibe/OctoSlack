package main

import (
	"log"
	"strings"
)

// LogLevel represents the logging level
type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
)

// Logger holds the current log level
type Logger struct {
	level LogLevel
}

var logger *Logger

// initLogger initializes the global logger with the configured log level
func initLogger(levelStr string) {
	level := INFO // default
	switch strings.ToUpper(levelStr) {
	case "DEBUG":
		level = DEBUG
	case "INFO":
		level = INFO
	case "WARN":
		level = WARN
	case "ERROR":
		level = ERROR
	}
	logger = &Logger{level: level}
}

// Debug logs debug messages
func (l *Logger) Debug(format string, v ...interface{}) {
	if l.level <= DEBUG {
		log.Printf("[DEBUG] "+format, v...)
	}
}

// Info logs informational messages
func (l *Logger) Info(format string, v ...interface{}) {
	if l.level <= INFO {
		log.Printf("[INFO] "+format, v...)
	}
}

// Warn logs warning messages
func (l *Logger) Warn(format string, v ...interface{}) {
	if l.level <= WARN {
		log.Printf("[WARN] "+format, v...)
	}
}

// Error logs error messages
func (l *Logger) Error(format string, v ...interface{}) {
	if l.level <= ERROR {
		log.Printf("[ERROR] "+format, v...)
	}
}

// Fatal logs fatal messages and exits
func (l *Logger) Fatal(format string, v ...interface{}) {
	log.Fatalf("[FATAL] "+format, v...)
}
