package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
)

type closeFunc func()error

func requestLogger(logger *log.Logger) func(http.Handler) http.Handler{
	return func(next http.Handler) http.Handler{
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request){
			next.ServeHTTP(w, r)
			logger.Printf("Served request: %v %v", r.Method, r.URL.Path)
		})
	}
}

func initializeLogger() (*log.Logger, closeFunc, error){
	logFile := os.Getenv("LINKO_LOG_FILE")	
	var logger *log.Logger
	if logFile == ""{
		logger = log.New(os.Stderr, "", log.LstdFlags)
		return logger, nil, nil
	}else{
		file, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
		if err != nil {
			return nil, nil, fmt.Errorf("Failed to open log file: %v", err)
		}
		bufferedWriter := bufio.NewWriterSize(file, 8192)
		multiWriter := io.MultiWriter(bufferedWriter, os.Stderr)
		logger = log.New(multiWriter, "", log.LstdFlags)
		cls := func()error{
			file.Close()
			err := bufferedWriter.Flush()
			if err != nil{
				return err
			}
			return nil
		}
		return logger, cls, nil
	}
}
