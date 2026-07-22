package main

import (
	"bufio"
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

func requestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			timeNow := time.Now()

			spyWriter := &spyResponseWriter{ResponseWriter: w}
			spyReader := &spyReadCloser{ReadCloser: r.Body}
			r.Body = spyReader

			next.ServeHTTP(spyWriter, r)
			logger.Info("Served request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.String("client_ip", r.RemoteAddr),
				slog.Duration("duration", time.Since(timeNow)),
				slog.Int("request_body_bytes", spyReader.bytesRead),
				slog.Int("response_status", spyWriter.statusCode),
				slog.Int("response_body_bytes", spyWriter.bytesWritten),

			)
		})
	}
}

func errorAttrs(err error) []slog.Attr {
	fmt.Printf("errorAttrs called with err=%#v, type=%T\n", err, err)
	if err == nil {
        	return []slog.Attr{}
    	    }
	group := []slog.Attr{}
	group = append(group, slog.Attr{
		Key:   "message",
		Value: slog.StringValue(err.Error()),
	})

	attrs := linkoerr.Attrs(err)
	group = append(group, attrs...)

	if stackErr, ok := errors.AsType[stackTracer](err); ok {
		group = append(group, slog.Attr{
			Key:   "stack_trace",
			Value: slog.StringValue(fmt.Sprintf("%+v", stackErr.StackTrace())),
		})
	}

	return group
}

func replaceAttr(groups []string, a slog.Attr) slog.Attr {
	v := a.Value.Any()
	if a.Key != "error" {
		return a
	}

	err, ok := v.(error)
	if !ok {
		return a
	}
	if err == nil{
		return a
	}

	if err, ok := errors.AsType[multiError](err); ok {
		groups := []slog.Attr{}
		errs := err.Unwrap()
		for i, err := range errs {
			groupErr := slog.GroupAttrs(fmt.Sprint("error_", i+1), errorAttrs(err)...)
			groups = append(groups, groupErr)
		}
		return slog.GroupAttrs("errors", groups...)
	} else {
		group := errorAttrs(err)
		return slog.GroupAttrs("error", group...)
	}
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
