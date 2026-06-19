package handlers

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type logFileInfo struct {
	Name       string    `json:"name"`
	Size       int64     `json:"size"`
	ModifiedAt time.Time `json:"modifiedAt"`
}

// HandleLogList handles GET /logs
func (h *Handler) HandleLogList(w http.ResponseWriter, r *http.Request) {
	files := make([]logFileInfo, 0)

	if entries, err := os.ReadDir(h.LogDir); err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".log") {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			files = append(files, logFileInfo{
				Name:       e.Name(),
				Size:       info.Size(),
				ModifiedAt: info.ModTime().UTC(),
			})
		}
	}

	processDir := filepath.Join(h.LogDir, "process")
	if entries, err := os.ReadDir(processDir); err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".log") {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			files = append(files, logFileInfo{
				Name:       "process/" + e.Name(),
				Size:       info.Size(),
				ModifiedAt: info.ModTime().UTC(),
			})
		}
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].ModifiedAt.After(files[j].ModifiedAt)
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"files": files,
		"total": len(files),
	})
}

// HandleLogFile handles GET /logs/{path}
func (h *Handler) HandleLogFile(w http.ResponseWriter, r *http.Request) {
	relPath := strings.TrimPrefix(r.URL.Path, "/logs/")
	relPath = filepath.Clean(relPath)

	if relPath == "" || relPath == "." {
		http.Error(w, `{"error":"filename required"}`, http.StatusBadRequest)
		return
	}
	if strings.Contains(relPath, "..") {
		http.Error(w, `{"error":"access denied"}`, http.StatusForbidden)
		return
	}
	if !strings.HasSuffix(relPath, ".log") {
		http.Error(w, `{"error":"only .log files are accessible"}`, http.StatusForbidden)
		return
	}

	fullPath := filepath.Join(h.LogDir, relPath)

	absLog, _ := filepath.Abs(h.LogDir)
	absFile, _ := filepath.Abs(fullPath)
	if !strings.HasPrefix(absFile, absLog+string(os.PathSeparator)) && absFile != absLog {
		http.Error(w, `{"error":"access denied"}`, http.StatusForbidden)
		return
	}

	data, err := os.ReadFile(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, `{"error":"file not found"}`, http.StatusNotFound)
		} else {
			http.Error(w, `{"error":"cannot read file"}`, http.StatusInternalServerError)
		}
		return
	}

	allLines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	if len(allLines) > 0 && allLines[len(allLines)-1] == "" {
		allLines = allLines[:len(allLines)-1]
	}

	tailN := 200
	if t := r.URL.Query().Get("tail"); t != "" {
		if n, err := strconv.Atoi(t); err == nil && n > 0 {
			if n > 5000 {
				n = 5000
			}
			tailN = n
		}
	}
	offset := 0
	if o := r.URL.Query().Get("offset"); o != "" {
		if n, err := strconv.Atoi(o); err == nil && n >= 0 {
			offset = n
		}
	}

	total := len(allLines)
	lines := allLines
	if offset > 0 && offset < total {
		lines = lines[offset:]
	} else if offset >= total {
		lines = []string{}
	}
	if len(lines) > tailN {
		lines = lines[len(lines)-tailN:]
	}
	for i, j := 0, len(lines)-1; i < j; i, j = i+1, j-1 {
		lines[i], lines[j] = lines[j], lines[i]
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"filename": relPath,
		"total":    total,
		"count":    len(lines),
		"offset":   offset,
		"tail":     tailN,
		"lines":    lines,
	})
}
