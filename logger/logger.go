package logger

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"
)

// LogLevel represents the severity of a log message
type LogLevel int

// various log levels
const (
	FATAL LogLevel = iota
	ERROR
	WARN
	INFO
	TRACE
)

// StringToLevel converts a string to a LogLevel in a case-insensitive manner.
func StringToLevel(level string) log.Level {
	level = strings.ToLower(level)
	switch level {
	case "fatal":
		return log.FatalLevel
	case "error":
		return log.ErrorLevel
	case "warn":
		return log.WarnLevel
	case "info":
		return log.InfoLevel
	case "debug":
		return log.DebugLevel
	case "trace":
		return log.TraceLevel
	default:
		log.Errorf("Unrecognized log level \"%s\", defaulting to \"trace\".\n", level)
		return log.TraceLevel
	}
}

// funcName gets the current function name from a pointer
func funcName(ptr uintptr) string {
	fname := runtime.FuncForPC(ptr).Name()
	lastDot := 0
	for i := 0; i < len(fname); i++ {
		if fname[i] == '.' {
			lastDot = i
		}
	}
	if lastDot == 0 {
		return filepath.Base(fname)
	}
	return fname[lastDot+1:] + "()"
}

// goroutineID fetches the current goroutine ID. Used solely for
// debugging which goroutine is doing what in the logs.
// Adapted from https://github.com/golang/net/blob/master/http2/gotrack.go
func goroutineID() uint64 {
	buf := make([]byte, 64)
	buf = buf[:runtime.Stack(buf, false)]
	// parse out # in the format "goroutine # "
	buf = bytes.TrimPrefix(buf, []byte("goroutine "))
	buf = buf[:bytes.IndexByte(buf, ' ')]
	id, _ := strconv.ParseUint(string(buf), 10, 64)
	return id
}

// Caller obtains the calling function's file and location at a certain point
// in the stack.
func Caller(level int) string {
	// go runtime witchcraft
	ptr, file, line, ok := runtime.Caller(level)
	var functionName string
	if ok {
		functionName = funcName(ptr)
	} else {
		functionName = "(unknown source)"
	}

	return fmt.Sprintf("%d:%s:%d:%s",
		goroutineID(), filepath.Base(file), line, functionName)
}

// LogrusFormatter returns a textformatter to be used during development
func LogrusFormatter() *log.TextFormatter {
	return &log.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02T15:04:05",
		CallerPrettyfier: func(f *runtime.Frame) (string, string) {
			filename := fmt.Sprintf("%s:%d", filepath.Base(f.File), f.Line)
			function := fmt.Sprintf(
				"%06d:%s()",
				goroutineID(),
				strings.Replace(f.Function, "github.com/jstaf/onedriver/", "", -1),
			)
			return function, filename
		},
	}
}

// LogTestSetup is a helper function purely used during tests.
func LogTestSetup() *os.File {
	logFile, _ := os.OpenFile("fusefs_tests.log", os.O_TRUNC|os.O_CREATE|os.O_RDWR, 0644)
	log.SetOutput(logFile)
	log.SetReportCaller(true)
	log.SetFormatter(LogrusFormatter())
	log.SetLevel(log.DebugLevel)
	return logFile
}
