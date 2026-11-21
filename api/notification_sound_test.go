package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRejectUnbundledPrinterSound(t *testing.T) {
	h, _ := newTestHandler(t)
	for _, sound := range []string{"../outside.mp3", "https://example.com/alert.mp3", "unknown.mp3"} {
		w := httptest.NewRecorder()
		body := `{"name":"Printer","url":"http://printer.local","api_key":"test","notification_sound":"` + sound + `"}`
		h.addPrinter(w, httptest.NewRequest(http.MethodPost, "/api/printers", strings.NewReader(body)))
		if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "notification sound") {
			t.Fatalf("accepted unbundled sound %q: %d %s", sound, w.Code, w.Body.String())
		}
	}
}
