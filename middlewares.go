package main

import (
	"context"
	"crypto/rand"
	"log/slog"
	"net/http"
	"time"
)


func requestIdMiddleware(next http.Handler) http.Handler{
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request){
		reqId := r.Header.Get("X-Request-ID")
		if reqId == ""{
			reqId = rand.Text()
		}
		w.Header().Set("X-Request-ID", reqId)
		next.ServeHTTP(w, r)
	})

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
				slog.String("request_id", spyWriter.Header().Get("X-Request-ID")),
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

