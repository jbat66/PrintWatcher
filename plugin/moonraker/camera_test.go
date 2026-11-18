package moonraker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jbat66/PrintWatcher/models"
)

func TestCameraRejectsStaleOrIncompleteJPEG(t *testing.T) {
	for _, tc := range []struct {
		name      string
		age       time.Duration
		frame     []byte
		wantError bool
	}{
		{"stale", time.Minute, []byte{0xff, 0xd8, 0xff, 0xd9}, true},
		{"incomplete", 0, []byte{0xff, 0xd8, 0, 0}, true},
		{"fresh", time.Second, []byte{0xff, 0xd8, 0xff, 0xd9}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("X-Api-Key") != "test-key" || r.URL.Query().Get("t") == "" {
					t.Error("camera fetch omitted authentication or cache-busting query")
				}
				printerTime := time.Now().Add(-time.Hour).UTC()
				w.Header().Set("Date", printerTime.Format(http.TimeFormat))
				w.Header().Set("Last-Modified", printerTime.Add(-tc.age).Format(http.TimeFormat))
				w.Write(tc.frame)
			}))
			defer server.Close()
			p := New(models.PrinterConfig{URL: server.URL, APIKey: "test-key"})
			p.client = server.Client()
			frame, err := p.fetchCameraJPEG(context.Background())
			if (err != nil) != tc.wantError || (err == nil && len(frame) != len(tc.frame)) {
				t.Fatalf("unexpected camera response: %v, %v", frame, err)
			}
		})
	}
}
