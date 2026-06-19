package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const defaultMaxSize = 25 * 1024 * 1024

var GlobalWriter io.Writer = os.Stdout

type RotatingWriter struct {
	mu      sync.Mutex
	file    *os.File
	path    string
	size    int64
	maxSize int64
}

func NewRotatingWriter(path string, maxSizeBytes int64) (*RotatingWriter, error) {
	rw := &RotatingWriter{path: path, maxSize: maxSizeBytes}
	if err := rw.openOrCreate(); err != nil {
		return nil, err
	}
	return rw, nil
}

func (rw *RotatingWriter) openOrCreate() error {
	f, err := os.OpenFile(rw.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	rw.file = f
	rw.size = info.Size()
	return nil
}

func (rw *RotatingWriter) rotate() error {
	if rw.file != nil {
		rw.file.Close()
		rw.file = nil
	}
	ts := time.Now().Format("20060102_150405")
	ext := filepath.Ext(rw.path)
	base := rw.path[:len(rw.path)-len(ext)]
	newPath := fmt.Sprintf("%s_%s%s", base, ts, ext)
	_ = os.Rename(rw.path, newPath)
	return rw.openOrCreate()
}

func (rw *RotatingWriter) Write(p []byte) (int, error) {
	rw.mu.Lock()
	defer rw.mu.Unlock()
	if rw.size+int64(len(p)) >= rw.maxSize {
		if err := rw.rotate(); err != nil {
			return 0, err
		}
	}
	n, err := rw.file.Write(p)
	rw.size += int64(n)
	return n, err
}

func (rw *RotatingWriter) Close() error {
	rw.mu.Lock()
	defer rw.mu.Unlock()
	if rw.file != nil {
		return rw.file.Close()
	}
	return nil
}

func rotateOnStartup(path string) {
	info, err := os.Stat(path)
	if err != nil || info.Size() == 0 {
		return
	}
	ts := time.Now().Format("20060102_150405")
	ext := filepath.Ext(path)
	base := path[:len(path)-len(ext)]
	newPath := fmt.Sprintf("%s_%s%s", base, ts, ext)
	_ = os.Rename(path, newPath)
}

func Init(logPath string) (io.Closer, error) {
	if logPath == "" {
		logPath = "logs/server-transfer.log"
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0755); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}
	rotateOnStartup(logPath)
	rw, err := NewRotatingWriter(logPath, defaultMaxSize)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}
	GlobalWriter = rw
	log.SetOutput(rw)
	log.SetFlags(log.LstdFlags)
	return rw, nil
}
