// Package chizap provides log handling using the zap package for the go-chi/chi router.
package chizap

import (
	"context"
	"net/http"
	"regexp"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Fn func(ctx context.Context) []zapcore.Field

type Skipper func(ctx context.Context) bool

type ZapLogger interface {
	Info(msg string, fields ...zap.Field)
	Error(msg string, fields ...zap.Field)
}

type Config struct {
	TimeFormat      string
	UTC             bool
	SkipPaths       []string
	SkipPathRegexps []*regexp.Regexp
	Context         Fn
	DefaultLevel    zapcore.Level

	Skipper Skipper
}

func Chizap(logger ZapLogger, timeFormat string, utc bool) func(next http.Handler) http.Handler {
	return ChizapWithConfig(logger, &Config{TimeFormat: timeFormat, UTC: utc, DefaultLevel: zapcore.InfoLevel})
}

func ChizapWithConfig(logger ZapLogger, conf *Config) func(next http.Handler) http.Handler {
	skipPaths := make(map[string]bool, len(conf.SkipPaths))
	for _, path := range conf.SkipPaths {
		skipPaths[path] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			path := r.URL.Path
			query := r.URL.RawQuery

			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			defer func() {
				track := true

				if _, ok := skipPaths[path]; ok || (conf.Skipper != nil && conf.Skipper(r.Context())) {
					track = false
				}

				if track && len(conf.SkipPathRegexps) > 0 {
					for _, reg := range conf.SkipPathRegexps {
						if !reg.MatchString(path) {
							continue
						}

						track = false
						break
					}
				}

				if track {
					end := time.Now()
					latency := end.Sub(start)
					if conf.UTC {
						end = end.UTC()
					}

					fields := []zapcore.Field{
						zap.Int("status", ww.Status()),
						zap.String("method", r.Method),
						zap.String("path", path),
						zap.String("query", query),
						zap.String("ip", r.RemoteAddr),
						zap.String("user-agent", r.UserAgent()),
						zap.Duration("latency", latency),
					}
					if conf.TimeFormat != "" {
						fields = append(fields, zap.String("time", end.Format(conf.TimeFormat)))
					}

					if conf.Context != nil {
						fields = append(fields, conf.Context(r.Context())...)
					}

					if ww.Status() >= 400 {
						logger.Error("", fields...)
					} else {
						if zl, ok := logger.(*zap.Logger); ok {
							zl.Log(conf.DefaultLevel, "", fields...)
						} else if conf.DefaultLevel == zapcore.InfoLevel {
							logger.Info("", fields...)
						} else {
							logger.Error("", fields...)
						}
					}
				}
			}()

			next.ServeHTTP(ww, r)
		})
	}
}
