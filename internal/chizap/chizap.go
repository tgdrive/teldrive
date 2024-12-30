// Package chizap provides log handling using the zap package for the go-chi/chi router.
package chizap

import (
	"context"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"regexp"
	"runtime/debug"
	"strings"
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

func defaultHandleRecovery(w http.ResponseWriter, r *http.Request, err interface{}) {
	w.WriteHeader(http.StatusInternalServerError)
}

func RecoveryWithZap(logger ZapLogger, stack bool) func(next http.Handler) http.Handler {
	return CustomRecoveryWithZap(logger, stack, defaultHandleRecovery)
}

func CustomRecoveryWithZap(logger ZapLogger, stack bool, recovery func(w http.ResponseWriter, r *http.Request, err interface{})) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					var brokenPipe bool
					if ne, ok := err.(*net.OpError); ok {
						if se, ok := ne.Err.(*os.SyscallError); ok {
							if strings.Contains(strings.ToLower(se.Error()), "broken pipe") ||
								strings.Contains(strings.ToLower(se.Error()), "connection reset by peer") {
								brokenPipe = true
							}
						}
					}

					httpRequest, _ := httputil.DumpRequest(r, false)
					if brokenPipe {
						logger.Error(r.URL.Path,
							zap.Any("error", err),
							zap.String("request", string(httpRequest)),
						)
						http.Error(w, "connection broken", http.StatusInternalServerError)
						return
					}

					if stack {
						logger.Error("[Recovery from panic]",
							zap.Time("time", time.Now()),
							zap.Any("error", err),
							zap.String("request", string(httpRequest)),
							zap.String("stack", string(debug.Stack())),
						)
					} else {
						logger.Error("[Recovery from panic]",
							zap.Time("time", time.Now()),
							zap.Any("error", err),
							zap.String("request", string(httpRequest)),
						)
					}
					recovery(w, r, err)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
