package api

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"
)

func TestCameraAcceptsRTSPAndRejectsOtherSchemes(t *testing.T) {
	h, d := newTestHandler(t)
	for _, raw := range []string{"rtsp://user:password@192.168.1.20:554/live", "http://192.168.1.21"} {
		body, _ := json.Marshal(map[string]string{"url": raw, "name": "Camera"})
		rec := do(h.handleCameras, "POST", "/api/cameras", string(body))
		if rec.Code != 200 {
			t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
		}
	}
	cams, _ := d.ListAllCameras()
	if len(cams) != 2 {
		t.Fatalf("camera count: %d", len(cams))
	}
	for _, method := range []string{"POST", "PUT"} {
		handler, path := h.handleCameras, "/api/cameras"
		if method == "PUT" {
			handler, path = h.handleCameraByID, fmt.Sprintf("/api/cameras/%d", cams[0].ID)
		}
		rec := do(handler, method, path, `{"url":"file:///etc/passwd"}`)
		if rec.Code != 400 {
			t.Fatalf("%s allowed invalid URL: %d", method, rec.Code)
		}
	}
}

func TestRTSPCameraSettingsAreNotProxied(t *testing.T) {
	h, d := newTestHandler(t)
	id, err := d.CreateCamera("rtsp://192.168.1.20/live", "Camera", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{"GET", "PUT"} {
		rec := httptest.NewRecorder()
		h.handleCameraSettings(rec, httptest.NewRequest(method, "/", nil), id)
		if rec.Code != 501 {
			t.Fatalf("%s: %d", method, rec.Code)
		}
	}
}
