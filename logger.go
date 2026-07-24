package main

import (
	// "bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"boot.dev/linko/internal/linkoerr"
	"github.com/lmittmann/tint"
	"github.com/mattn/go-isatty"
	"github.com/natefinch/lumberjack"
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
		logger = slog.New(tint.NewTextHandler(os.Stderr, &tint.Options{
			NoColor: !(isatty.IsCygwinTerminal(os.Stderr.Fd()) || isatty.IsTerminal(os.Stderr.Fd())),
		}))
		return logger, nil, nil
	} else {
		debugHandler := tint.NewTextHandler(os.Stderr, &tint.Options{
			NoColor: !(isatty.IsCygwinTerminal(os.Stderr.Fd()) || isatty.IsTerminal(os.Stderr.Fd())),
			Level:       slog.LevelDebug,
			ReplaceAttr: replaceAttr,
		})
		// file, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		// if err != nil {
		// 	return nil, nil, fmt.Errorf("Failed to open log file: %v", err)
		// }
		// bufferedWriter := bufio.NewWriterSize(file, 8192)

		rotateLogger := &lumberjack.Logger{
			Filename: logFile,
			MaxSize: 1,
			MaxAge: 28,
			MaxBackups: 10,
			LocalTime: false,
			Compress: true,
		}

		infoHandler := slog.NewJSONHandler(rotateLogger, &slog.HandlerOptions{
			Level:       slog.LevelInfo,
			ReplaceAttr: replaceAttr,
		})
		logger = slog.New(slog.NewMultiHandler(
			debugHandler,
			infoHandler,
		))
		cls := func() error {
			// err := bufferedWriter.Flush()
			// if err != nil {
			// 	return err
			// }
			// file.Close()
			rotateLogger.Close()
			return nil
		}
		return logger, cls, nil
	}
}
