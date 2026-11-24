package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/jbat66/PrintWatcher/models"
)

func TestDiagnosticPingParsing(t *testing.T) {
	for _, tc := range []struct {
		output  string
		err     error
		ms      float64
		message string
	}{
		{"64 bytes from 10.0.0.1: icmp_seq=1 ttl=64 time=18.32 ms", nil, 18.32, ""},
		{"Reply from 10.0.0.1: bytes=32 time<1ms TTL=64", nil, .5, ""},
		{"Destination Host Unreachable", errors.New("exit 1"), 0, "Host unreachable"},
		{"100% packet loss", errors.New("exit 1"), 0, "No ping reply"},
		{"ping: socket: Operation not permitted", errors.New("exit 2"), 0, "Ping permission denied"},
		{"time=18 ms\nDestination Host Unreachable", errors.New("exit 1"), 0, "Host unreachable"},
	} {
		got := parseDiagnosticPing(tc.output, tc.err)
		if got.Error != tc.message || (tc.message == "" && (got.MS == nil || *got.MS != tc.ms)) || (tc.message != "" && got.MS != nil) {
			t.Errorf("parse %q = %+v", tc.output, got)
		}
	}
}

func TestDiagnosticsCameraTargetsAndNoStore(t *testing.T) {
	h, database := newTestHandler(t)
	p := &models.PrinterConfig{Name: "test", Type: "snapmaker-u1", URL: "http://127.0.0.1:7125", PollInterval: 10}
	if err := database.CreatePrinter(p); err != nil {
		t.Fatal(err)
	}
	for _, cameraURL := range []string{"rtsp://127.0.0.1:554/live", "http://169.254.169.254/snapshot"} {
		if _, err := database.CreateCamera(cameraURL, "camera", &p.ID); err != nil {
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		h.getDiagnostics(w, httptest.NewRequest("GET", "/diagnostics", nil), p.ID)
		var result struct {
			Printer diagnosticSample
			Camera  diagnosticSample
		}
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" || result.Printer.Error != "Host address blocked" {
			t.Fatalf("unexpected response: %d %s", w.Code, w.Body.String())
		}
		if cameraURL == "rtsp://127.0.0.1:554/live" {
			if !result.Camera.SameHost {
				t.Fatal("same-host camera should reuse printer ping")
			}
		} else if result.Camera.Error != "Host address blocked" {
			t.Fatalf("camera result: %+v", result.Camera)
		}
	}
}

func TestDiagnosticTargets(t *testing.T) {
	for _, tc := range []struct{ raw, base, want string }{
		{"rtsp://user:password@10.0.0.2:554/live", "", "10.0.0.2"},
		{"/webcam/?action=snapshot", "http://printer:7125", "printer"},
		{"http://[fd00::2]:8080/snapshot", "", "fd00::2"},
	} {
		got, err := diagnosticHost(tc.raw, tc.base)
		if err != nil || got != tc.want {
			t.Fatalf("host = %q, %v", got, err)
		}
	}
	for _, ip := range []string{"127.0.0.1", "169.254.169.254", "::1", "0.0.0.0", "224.0.0.1"} {
		if _, err := diagnosticIP(context.Background(), ip); err == nil {
			t.Errorf("allowed %s", ip)
		}
	}
	if got, err := diagnosticIP(context.Background(), "10.0.0.2"); err != nil || got != "10.0.0.2" {
		t.Fatalf("LAN = %q, %v", got, err)
	}
}

func TestDiagnosticsRejectMethodAndMissingPrinter(t *testing.T) {
	h, _ := newTestHandler(t)
	for _, tc := range []struct {
		method string
		code   int
	}{{"POST", 405}, {"GET", 404}} {
		w := httptest.NewRecorder()
		h.getDiagnostics(w, httptest.NewRequest(tc.method, "/api/printers/999/diagnostics", nil), 999)
		if w.Code != tc.code {
			t.Fatalf("%s: %d", tc.method, w.Code)
		}
	}
}
