package common

import (
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// Logger 封装logrus.Logger
type Logger struct {
	*logrus.Logger
}

// LoggerOptions 日志配置选项
type LoggerOptions struct {
	Level        logrus.Level
	ReportCaller bool
}

const (
	Reset  = "\033[0m"
	Red    = "\033[31m"
	Green  = "\033[32m"
	Yellow = "\033[33m"
	Blue   = "\033[34m"
)

var (
	instance *Logger
	once     sync.Once
	mu       sync.RWMutex
)

func GetSimpleFuncName(depth int) string {
	pc, _, _, _ := runtime.Caller(depth)
	fullFuncName := runtime.FuncForPC(pc).Name()
	parts := strings.Split(fullFuncName, ".")
	return parts[len(parts)-1] // 返回最后的部分，即函数名
}

// GetLogger 获取日志单例
func GetLogger() *Logger {
	once.Do(func() {
		instance = newLogger(&LoggerOptions{
			Level:        logrus.DebugLevel,
			ReportCaller: true,
		})
	})
	return instance
}

// newLogger 创建新的日志实例
func newLogger(opts *LoggerOptions) *Logger {
	l := logrus.New()
	l.SetLevel(opts.Level)
	l.SetFormatter(&CustomFormatter{})
	l.SetReportCaller(opts.ReportCaller)
	return &Logger{Logger: l}
}

// CustomFormatter 自定义日志格式
type CustomFormatter struct{}

func (f *CustomFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	// 获取调用栈信息
	pc, file, line, _ := runtime.Caller(8)

	// 获取简单文件名（去掉路径）
	simpleFile := filepath.Base(file)

	// 获取简单函数名
	funcName := "unknown"
	if fn := runtime.FuncForPC(pc); fn != nil {
		fullName := fn.Name()
		parts := strings.Split(fullName, ".")
		funcName = parts[len(parts)-1]
	}

	// 根据日志级别设置颜色
	var levelColor string
	switch entry.Level {
	case logrus.DebugLevel:
		levelColor = Blue
	case logrus.InfoLevel:
		levelColor = Green
	case logrus.WarnLevel:
		levelColor = Yellow
	case logrus.ErrorLevel, logrus.FatalLevel, logrus.PanicLevel:
		levelColor = Red
	default:
		levelColor = Reset
	}

	// 格式化日志
	timestamp := entry.Time.Format(time.DateTime)
	var fields string
	if len(entry.Data) > 0 {
		fields = fmt.Sprintf(" %s{%+v}%s", Blue, entry.Data, Reset)
	}

	// 使用 simpleFile/funcName 格式
	logMessage := fmt.Sprintf("%s %s[%s]%s %s[%s/%s:%d]%s : %s%s\n",
		timestamp,
		levelColor,
		entry.Level.String(),
		Reset,
		Blue,
		simpleFile,
		funcName,
		line,
		Reset,
		entry.Message,
		fields,
	)

	return []byte(logMessage), nil
}

// SetOutput 设置日志输出文件路径
func SetOutput(filePath string) error {
	mu.Lock()
	defer mu.Unlock()

	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return fmt.Errorf("failed to open log file: %v", err)
	}

	GetLogger().SetOutput(file)
	return nil
}

// SetLevel 设置日志级别
func SetLevel(level logrus.Level) {
	mu.Lock()
	defer mu.Unlock()
	GetLogger().SetLevel(level)
}

// DisableLogging 禁用日志输出
func DisableLogging() {
	mu.Lock()
	defer mu.Unlock()
	GetLogger().SetOutput(io.Discard)
}

// 添加一个随机概率打印的辅助函数
func LogWithProbability(probability float64, level string, format string, args ...interface{}) {
	if probability >= 1.0 || rand.Float64() < probability {
		switch level {
		case "debug":
			Debug(format, args...)
		case "info":
			Info(format, args...)
		case "warn":
			Warn(format, args...)
		case "error":
			Error(format, args...)
		}
	}
}

// AddHook 添加钩子
func AddHook(hook logrus.Hook) {
	mu.Lock()
	defer mu.Unlock()
	GetLogger().AddHook(hook)
}

// 封装常用的日志方法
func Debug(format string, args ...interface{}) {
	if len(args) > 0 {
		GetLogger().Debugf(format, args...)
	} else {
		GetLogger().Debug(format)
	}
}

func Info(format string, args ...interface{}) {
	if len(args) > 0 {
		GetLogger().Infof(format, args...)
	} else {
		GetLogger().Info(format)
	}
}

func Warn(format string, args ...interface{}) {
	if len(args) > 0 {
		GetLogger().Warnf(format, args...)
	} else {
		GetLogger().Warn(format)
	}
}

func Error(format string, args ...interface{}) {
	if len(args) > 0 {
		GetLogger().Errorf(format, args...)
	} else {
		GetLogger().Error(format)
	}
}

func Fatal(format string, args ...interface{}) {
	if len(args) > 0 {
		GetLogger().Fatalf(format, args...)
	} else {
		GetLogger().Fatal(format)
	}
}

func WithFields(fields logrus.Fields) *logrus.Entry {
	return GetLogger().WithFields(fields)
}
