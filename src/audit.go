package main

import (
	"encoding/json"
	"os"
	"sync"
	"time"
)

// Audit 以 JSONL 记录每次意图、工具调用与确认结果，便于事后追查。
type Audit struct {
	mu   sync.Mutex
	path string
}

func newAudit(path string) *Audit {
	return &Audit{path: path}
}

func (a *Audit) log(entry map[string]any) {
	if a == nil || a.path == "" {
		return
	}
	now := time.Now()
	entry["ts"] = now.Format(time.RFC3339)
	entry["tsMs"] = now.UnixMilli()
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	f, err := os.OpenFile(a.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(data, '\n'))
}
