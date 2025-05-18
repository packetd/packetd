// Copyright 2025 The packetd Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package logger

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

type Level string

const (
	LevelDebug Level = "debug"
	LevelInfo  Level = "info"
	LevelWarn  Level = "warn"
	LevelError Level = "error"
)

func toZapLevel(l string) zapcore.Level {
	levels := map[Level]zapcore.Level{
		LevelDebug: zapcore.DebugLevel,
		LevelInfo:  zapcore.InfoLevel,
		LevelWarn:  zapcore.WarnLevel,
		LevelError: zapcore.ErrorLevel,
	}
	if level, ok := levels[Level(l)]; ok {
		return level
	}
	return zapcore.DebugLevel
}

type Options struct {
	Stdout     bool   `config:"stdout"`
	Level      string `config:"level"`
	Filename   string `config:"filename"`
	MaxSize    int    `config:"maxSize"` // unit: MB
	MaxAge     int    `config:"maxAge"`  // unit: days
	MaxBackups int    `config:"maxBackups"`
}

type Logger struct {
	sugared *zap.SugaredLogger
}

func (l Logger) Debugf(template string, args ...any) {
	l.sugared.Debugf(template, args...)
}

func (l Logger) Infof(template string, args ...any) {
	l.sugared.Infof(template, args...)
}

func (l Logger) Warnf(template string, args ...any) {
	l.sugared.Warnf(template, args...)
}

func (l Logger) Errorf(template string, args ...any) {
	l.sugared.Errorf(template, args...)
}

// New 创建并返回标准 Logger 实例
func New(opt Options) Logger {
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
		enc.AppendString(t.Local().Format("2006-01-02 15:04:05.000"))
	}
	encoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder
	encoder := zapcore.NewConsoleEncoder(encoderConfig)

	var w zapcore.WriteSyncer
	switch {
	case opt.Stdout:
		w = zapcore.AddSync(os.Stdout)
	default:
		// 初始化日志目录
		if err := os.MkdirAll(filepath.Dir(opt.Filename), os.ModePerm); err != nil {
			panic(err)
		}

		w = zapcore.AddSync(&lumberjack.Logger{
			Filename:   opt.Filename,
			MaxSize:    opt.MaxSize,
			MaxBackups: opt.MaxBackups,
			MaxAge:     opt.MaxAge,
			LocalTime:  true,
		})
	}

	level := toZapLevel(opt.Level)
	core := zapcore.NewCore(encoder, w, level)
	logger := zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1))
	return Logger{
		sugared: logger.Sugar(),
	}
}

var (
	stdOpt = Options{Stdout: true}
	std    = New(stdOpt)
)

// SetOptions 设置全局 Logger 配置
func SetOptions(opt Options) {
	stdOpt = opt
	std = New(opt)
}

// SetLoggerLevel 设置全局 Logger 日志级别
func SetLoggerLevel(s string) {
	stdOpt.Level = strings.ToLower(strings.TrimSpace(s))
	std = New(stdOpt)
}

func Debugf(template string, args ...any) {
	std.Debugf(template, args...)
}

func Infof(template string, args ...any) {
	std.Infof(template, args...)
}

func Warnf(template string, args ...any) {
	std.Warnf(template, args...)
}

func Errorf(template string, args ...any) {
	std.Errorf(template, args...)
}
