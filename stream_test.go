package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type notifyingWriter struct {
	mu      sync.Mutex
	text    strings.Builder
	flushed chan struct{}
	header  http.Header
}

func (w *notifyingWriter) Header() http.Header { return w.header }
func (w *notifyingWriter) WriteHeader(int)     {}
func (w *notifyingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.text.Write(p)
}
func (w *notifyingWriter) Flush() {
	select {
	case w.flushed <- struct{}{}:
	default:
	}
}

func TestHeartbeatKeepsSilentGenerationAliveAndStops(t *testing.T) {
	w := &notifyingWriter{flushed: make(chan struct{}, 5), header: make(http.Header)}
	send, stop := startEventStream(context.Background(), w, 5*time.Millisecond)
	select {
	case <-w.flushed:
	case <-time.After(time.Second):
		t.Fatal("no heartbeat while model silent")
	}
	send("delta", map[string]string{"t": "讲解"})
	stop()
	if !strings.Contains(w.text.String(), "event: ping") || !strings.Contains(w.text.String(), "event: delta") {
		t.Fatal("missing flushed events")
	}
}

func TestUpstreamCompletionAndCancellation(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		wantError  bool
	}{
		{"truncated", "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n", true},
		{"token_limit", "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"length\"}]}\n\n", true},
		{"normal", "data: [DONE]\n\n", false},
		{"stop_then_eof", "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, tc.body) }))
			defer upstream.Close()
			err := streamLLM(context.Background(), &SiliconFlowConfig{BaseURL: upstream.URL}, "test", nil, 100, time.Second, func(string) {})
			if (err != nil) != tc.wantError {
				t.Fatalf("unexpected result: %v", err)
			}
		})
	}
	t.Run("cancel", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := streamLLM(ctx, &SiliconFlowConfig{BaseURL: "http://127.0.0.1:1"}, "test", nil, 100, time.Second, func(string) {})
		if err == nil {
			t.Fatal("canceled request was accepted")
		}
	})
}

func TestTablePreviewShowsRowsBeforeCompletion(t *testing.T) {
	raw := "# 成果\n讲解\n" + tableHeader + givenRows + "| 尚未生成完整"
	preview := previewAnswer(raw)
	if !strings.Contains(preview, "牵引传动系统") || strings.Contains(preview, "尚未生成完整") {
		t.Fatal("complete rows must appear without waiting for final completion")
	}
	preview = previewAnswer(raw[:strings.LastIndex(raw, "| 尚未")] + tableHeader + givenRows + "| 制动[1] | 软件[2] | 舒适[3] |\n")
	if strings.Count(preview, "| 自主建造 |") != 1 || strings.Count(preview, "| 网络控制系统 |") != 1 {
		t.Fatal("preview duplicated table")
	}
}
