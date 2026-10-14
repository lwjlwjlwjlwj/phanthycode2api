// Package logx 提供极简的分级日志。
//
// 默认 info 级别：每个请求一行摘要，外加真正的错误。
// 排查问题时把 log_level 调成 debug，可以看到换号、上游状态码等逐次尝试的细节。
package logx

import (
	"log"
	"strings"
	"sync/atomic"
)

// Level 日志级别，数值越大越重要。
type Level int32

const (
	// LevelDebug 逐次尝试、上游原始响应等排查细节。
	LevelDebug Level = iota
	// LevelInfo 每个请求一行摘要，默认级别。
	LevelInfo
	// LevelError 只输出失败与异常。
	LevelError
)

var current atomic.Int32

func init() { current.Store(int32(LevelInfo)) }

// Parse 解析级别名；无法识别时回落到 info，保证配置写错也不会把日志全关掉。
func Parse(name string) Level {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "debug", "verbose", "trace":
		return LevelDebug
	case "error", "warn", "warning", "quiet":
		return LevelError
	default:
		return LevelInfo
	}
}

// String 返回级别名。
func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "debug"
	case LevelError:
		return "error"
	default:
		return "info"
	}
}

// Set 按名字设置全局级别，并返回实际生效的级别。
func Set(name string) Level {
	return SetLevel(Parse(name))
}

// SetLevel 直接设置全局级别，并返回生效的级别。
func SetLevel(l Level) Level {
	current.Store(int32(l))
	return l
}

// Current 返回当前级别。
func Current() Level { return Level(current.Load()) }

// Enabled 报告某级别是否会输出。
func Enabled(l Level) bool { return current.Load() <= int32(l) }

// Debugf 输出调试日志。
func Debugf(format string, args ...any) { output(LevelDebug, format, args...) }

// Infof 输出常规日志。
func Infof(format string, args ...any) { output(LevelInfo, format, args...) }

// Errorf 输出错误日志。
func Errorf(format string, args ...any) { output(LevelError, format, args...) }

func output(l Level, format string, args ...any) {
	if !Enabled(l) {
		return
	}
	log.Printf(format, args...)
}
