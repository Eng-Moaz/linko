package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
)

func requestLogger(logger *log.Logger) func(http.Handler) http.Handler{
	return func(next http.Handler) http.Handler{
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request){
			next.ServeHTTP(w, r)
			logger.Printf("Served request: %v %v", r.Method, r.URL.Path)
		})
	}
}

func initializeLogger() (*log.Logger, *os.File, error){
	logFile := os.Getenv("LINKO_LOG_FILE")	
	var file *os.File
	var logger *log.Logger
	if logFile == ""{
		logger = log.New(os.Stderr, "", log.LstdFlags)
	}else{
		file, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
		if err != nil {
			return nil, file, fmt.Errorf("Failed to open log file: %v", err)
		}
		multiWriter := io.MultiWriter(file, os.Stderr)
		logger = log.New(multiWriter, "", log.LstdFlags)
	}
	return logger, file, nil
}
