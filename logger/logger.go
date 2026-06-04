package logger

import (
	"container/ring"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"
	"time"
)

type LogLevel string

const (
	INFO  LogLevel = "INFO"
	WARN  LogLevel = "WARN"
	ERROR LogLevel = "ERROR"
)

type LogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Level     LogLevel  `json:"level"`
	Instance  string    `json:"instance,omitempty"`
	Message   string    `json:"message"`
}

var (
	ringBuffer *ring.Ring
	bufferMu   sync.Mutex
	fileLogger *log.Logger
	stdLogger  *log.Logger
)

func Init(logFile string) error {
	ringBuffer = ring.New(100)
	
	stdLogger = log.New(os.Stdout, "", 0)

	if logFile != "" {
		f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			return err
		}
		fileLogger = log.New(f, "", 0)
	}

	return nil
}

func logMessage(level LogLevel, instance, format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     level,
		Instance:  instance,
		Message:   msg,
	}

	logLine := fmt.Sprintf("[%s] [%s]", entry.Timestamp.Format(time.RFC3339), entry.Level)
	if instance != "" {
		logLine += fmt.Sprintf(" [%s]", instance)
	}
	logLine += " " + entry.Message

	// Log to stdout
	if stdLogger != nil {
		stdLogger.Println(logLine)
	}

	// Log to file
	if fileLogger != nil {
		fileLogger.Println(logLine)
	}

	// Log to ring buffer
	bufferMu.Lock()
	defer bufferMu.Unlock()
	ringBuffer.Value = entry
	ringBuffer = ringBuffer.Next()
}

func Info(instance, format string, v ...interface{}) {
	logMessage(INFO, instance, format, v...)
}

func Warn(instance, format string, v ...interface{}) {
	logMessage(WARN, instance, format, v...)
}

func Error(instance, format string, v ...interface{}) {
	logMessage(ERROR, instance, format, v...)
}

func GetRecentLogs() []LogEntry {
	bufferMu.Lock()
	defer bufferMu.Unlock()

	var logs []LogEntry
	ringBuffer.Do(func(p interface{}) {
		if p != nil {
			logs = append(logs, p.(LogEntry))
		}
	})

	// Do() iterates forward from the current pointer. In a ring buffer where we just added elements, 
	// the oldest element is at the current pointer, and the newest element is the one just before it.
	// So logs will be ordered from oldest to newest. We want to return newest to oldest or keep it chronological?
	// The requirement is "last 100 log lines in a scrollable monospace box". Chronological (oldest to newest) is standard for tailing logs.
	return logs
}

func GetRecentLogsJSON() ([]byte, error) {
	logs := GetRecentLogs()
	return json.Marshal(logs)
}
