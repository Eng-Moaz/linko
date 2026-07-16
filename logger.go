package main

import (
	"bufio"
	"fmt"
	"log/slog"
	"net/http"
	"os"
)

type closeFunc func()error

func requestLogger(logger *slog.Logger) func(http.Handler) http.Handler{
	return func(next http.Handler) http.Handler{
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request){
			next.ServeHTTP(w, r)
			logger.Info("Served request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.String("client_ip", r.RemoteAddr),
			)
		})
	}
}

func initializeLogger() (*slog.Logger, closeFunc, error){
	logFile := os.Getenv("LINKO_LOG_FILE")	
	var logger *slog.Logger
	if logFile == ""{
		logger = slog.New(slog.NewTextHandler(os.Stderr, nil))
		return logger, nil, nil
	}else{
		debugHandler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})
		file, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return nil, nil, fmt.Errorf("Failed to open log file: %v", err)
		}
		bufferedWriter := bufio.NewWriterSize(file, 8192)
		infoHandler := slog.NewJSONHandler(bufferedWriter, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})
		logger = slog.New(slog.NewMultiHandler(
			debugHandler,
			infoHandler,
		))
		cls := func()error{
			err := bufferedWriter.Flush()
			if err != nil{
				return err
			}
			file.Close()
			return nil
		}
		return logger, cls, nil
	}
}
