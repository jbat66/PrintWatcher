package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jbat66/PrintWatcher/models"
)

func TestPrinterUploadStorageAndExtensions(t *testing.T) {
	for _, tc := range []struct{ kind, name, storage string }{
		{"prusalink", "part.gcode", "usb"}, {"prusalink", "part.BGCODE", "usb"},
		{"snapmaker-u1", "part.GCODE", "gcodes"}, {"snapmaker-u1", "part.bgcode", ""},
		{"prusalink", "part.txt", ""}, {"octoprint", "part.gcode", ""},
		{"snapmaker-u1", "../part.gcode", ""}, {"prusalink", `folder\part.gcode`, ""},
	} {
		t.Run(tc.kind+"/"+tc.name, func(t *testing.T) {
			storage, err := printerUploadStorage(tc.kind, tc.name)
			if storage != tc.storage || (err != nil) != (tc.storage == "") {
				t.Fatalf("storage=%q err=%v", storage, err)
			}
		})
	}
}

func TestKlipperUploadRejectsBGCODEBeforeTransfer(t *testing.T) {
	h, d := newTestHandler(t)
	p := models.PrinterConfig{Name: "U1", Type: "snapmaker-u1"}
	if err := d.CreatePrinter(&p); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	h.uploadFile(w, httptest.NewRequest(http.MethodPost, "/upload?filename=part.bgcode", strings.NewReader("not sent")), p.ID)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "gcode files only") {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}
