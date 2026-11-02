package api

import (
	"net/http"

	"github.com/jbat66/PrintWatcher/camera"
)

func (h *Handler) serveRTSP(w http.ResponseWriter, r *http.Request, raw string, snapshot bool) {
	if snapshot {
		data, err := camera.Snapshot(r.Context(), raw)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Cache-Control", "no-cache, no-store")
		w.Write(data)
		return
	}
	s, err := camera.Start(r.Context(), raw, false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer s.Close()
	w.Header().Set("Content-Type", "multipart/x-mixed-replace; boundary=printwatcher")
	w.Header().Set("Cache-Control", "no-cache, no-store")
	flusher, _ := w.(http.Flusher)
	buf := make([]byte, 32*1024)
	for {
		n, err := s.Read(buf)
		if n > 0 {
			if _, writeErr := w.Write(buf[:n]); writeErr != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if err != nil {
			return
		}
	}
}
