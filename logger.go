package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"boot.dev/linko/internal/linkoerr"
	pkgerrors "github.com/pkg/errors"
)

type closeFunc func() error

type stackTracer interface {
	error
	StackTrace() pkgerrors.StackTrace
}

type multiError interface {
	error
	Unwrap() []error
}

const logContextKey contextKey = "log_context"

type LogContext struct {
	Username string
	Error error
}

func httpError(ctx context.Context, w http.ResponseWriter, status int, err error) {
	if logCtx, ok := ctx.Value(logContextKey).(*LogContext); ok {
		logCtx.Error = err
	}
	http.Error(w, err.Error(), status)
}

func requestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			timeNow := time.Now()

			spyWriter := &spyResponseWriter{ResponseWriter: w}
			spyReader := &spyReadCloser{ReadCloser: r.Body}
			r.Body = spyReader

			logCtx := &LogContext{}
			newCtx := context.WithValue(r.Context(), logContextKey, logCtx)
			r = r.WithContext(newCtx)

			next.ServeHTTP(spyWriter, r)

			logAttrs := []any{
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.String("client_ip", r.RemoteAddr),
				slog.Duration("duration", time.Since(timeNow)),
				slog.Int("request_body_bytes", spyReader.bytesRead),
				slog.Int("response_status", spyWriter.statusCode),
				slog.Int("response_body_bytes", spyWriter.bytesWritten),
			}

			if logCtx, ok := r.Context().Value(logContextKey).(*LogContext); ok {
				if logCtx.Username != ""{
					logAttrs = append(logAttrs, slog.String("user", logCtx.Username))
				}
				if logCtx.Error != nil{
					logAttrs = append(logAttrs, slog.Any("error", logCtx.Error))
				}
			}

			logger.Info("Served request", logAttrs...)
		})
	}
}

func errorAttrs(err error) []slog.Attr {
	if err == nil {
		return []slog.Attr{}
	}

	group := []slog.Attr{
		{
			Key:   "message",
			Value: slog.StringValue(err.Error()),
		},
	}

	attrs := linkoerr.Attrs(err)
	group = append(group, attrs...)

	var st stackTracer
	if errors.As(err, &st) {
		group = append(group, slog.Attr{
			Key:   "stack_trace",
			Value: slog.StringValue(fmt.Sprintf("%+v", st.StackTrace())),
		})
	}

	return group
}

func replaceAttr(groups []string, a slog.Attr) slog.Attr {
	if a.Key != "error" {
		return a
	}

	errVal, ok := a.Value.Any().(error)
	if !ok || errVal == nil {
		return a
	}

	var mErr multiError
	if errors.As(errVal, &mErr) {
		var errGroups []slog.Attr
		for i, err := range mErr.Unwrap() {
			errGroups = append(errGroups, slog.GroupAttrs(fmt.Sprintf("error_%d", i+1), errorAttrs(err)...))
		}
		return slog.GroupAttrs("errors", errGroups...)
	}

	return slog.GroupAttrs("error", errorAttrs(errVal)...)
}

func initializeLogger() (*slog.Logger, closeFunc, error) {
	logFile := os.Getenv("LINKO_LOG_FILE")
	var logger *slog.Logger
	if logFile == "" {
		logger = slog.New(slog.NewTextHandler(os.Stderr, nil))
		return logger, nil, nil
	} else {
		debugHandler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level:       slog.LevelDebug,
			ReplaceAttr: replaceAttr,
		})
		file, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return nil, nil, fmt.Errorf("Failed to open log file: %v", err)
		}
		bufferedWriter := bufio.NewWriterSize(file, 8192)
		infoHandler := slog.NewJSONHandler(bufferedWriter, &slog.HandlerOptions{
			Level:       slog.LevelInfo,
			ReplaceAttr: replaceAttr,
		})
		logger = slog.New(slog.NewMultiHandler(
			debugHandler,
			infoHandler,
		))
		cls := func() error {
			err := bufferedWriter.Flush()
			if err != nil {
				return err
			}
			file.Close()
			return nil
		}
		return logger, cls, nil
	}
}
