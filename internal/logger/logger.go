// Package logger 提供结构化日志能力。
package logger

import (
	"fmt"
	"log"
	"strings"
	"time"
)

// Level 日志级别。
type Level string

const (
	DebugLevel Level = "debug"
	InfoLevel  Level = "info"
	WarnLevel  Level = "warn"
	ErrorLevel Level = "error"
)

// Logger 提供结构化日志记录。
type Logger struct {
	name string
}

// New 创建一个新的日志记录器。
func New(name string) *Logger {
	return &Logger{name: name}
}

// logEvent 记录一个日志事件。
func (l *Logger) logEvent(level Level, message string, fields map[string]interface{}) {
	parts := []string{
		fmt.Sprintf("level=%s", level),
		fmt.Sprintf("logger=%s", l.name),
		fmt.Sprintf("ts=%s", time.Now().Format("2006-01-02 15:04:05.000")),
		fmt.Sprintf("msg=%q", message),
	}

	// 添加额外字段
	for key, value := range fields {
		parts = append(parts, fmt.Sprintf("%s=%v", key, value))
	}

	log.Println(strings.Join(parts, " "))
}

// Debug 记录 DEBUG 级别日志。
func (l *Logger) Debug(message string, fields map[string]interface{}) {
	l.logEvent(DebugLevel, message, fields)
}

// Info 记录 INFO 级别日志。
func (l *Logger) Info(message string, fields map[string]interface{}) {
	l.logEvent(InfoLevel, message, fields)
}

// Warn 记录 WARN 级别日志。
func (l *Logger) Warn(message string, fields map[string]interface{}) {
	l.logEvent(WarnLevel, message, fields)
}

// Error 记录 ERROR 级别日志。
func (l *Logger) Error(message string, fields map[string]interface{}) {
	l.logEvent(ErrorLevel, message, fields)
}

// WithField 创建一个包含预设字段的日志上下文。
type LogContext struct {
	logger *Logger
	fields map[string]interface{}
}

// WithFields 创建一个带有预设字段的日志上下文。
func (l *Logger) WithFields(fields map[string]interface{}) *LogContext {
	return &LogContext{
		logger: l,
		fields: fields,
	}
}

// Debug 记录 DEBUG 级别日志（带上下文）。
func (lc *LogContext) Debug(message string, extraFields map[string]interface{}) {
	merged := mergeFields(lc.fields, extraFields)
	lc.logger.logEvent(DebugLevel, message, merged)
}

// Info 记录 INFO 级别日志（带上下文）。
func (lc *LogContext) Info(message string, extraFields map[string]interface{}) {
	merged := mergeFields(lc.fields, extraFields)
	lc.logger.logEvent(InfoLevel, message, merged)
}

// Warn 记录 WARN 级别日志（带上下文）。
func (lc *LogContext) Warn(message string, extraFields map[string]interface{}) {
	merged := mergeFields(lc.fields, extraFields)
	lc.logger.logEvent(WarnLevel, message, merged)
}

// Error 记录 ERROR 级别日志（带上下文）。
func (lc *LogContext) Error(message string, extraFields map[string]interface{}) {
	merged := mergeFields(lc.fields, extraFields)
	lc.logger.logEvent(ErrorLevel, message, merged)
}

func mergeFields(base, extra map[string]interface{}) map[string]interface{} {
	if base == nil {
		base = make(map[string]interface{})
	}
	if extra == nil {
		return base
	}
	for k, v := range extra {
		base[k] = v
	}
	return base
}
