package moonraker

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/jbat66/PrintWatcher/models"
)

func TestU1StatusAndTerminalJob(t *testing.T) {
	var completed atomic.Bool
	var reportsLayer atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "test-key" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/server/files/roots":
			fmt.Fprint(w, `{"result":[{"name":"gcodes"},{"name":"camera"}]}`)
		case "/server/info":
			fmt.Fprint(w, `{"result":{"moonraker_version":"1.4.0"}}`)
		case "/printer/objects/query":
			state := "printing"
			if completed.Load() {
				state = "complete"
			}
			layer := "null"
			if reportsLayer.Load() {
				layer = "0"
			}
			if r.URL.Query().Get("gcode_move") != "gcode_position" {
				t.Error("missing commanded Z query")
			}
			fmt.Fprintf(w, `{"result":{"status":{"webhooks":{"state":"ready"},"print_stats":{"state":%q,"filename":"folder/test.gcode","print_duration":90,"info":{"current_layer":%s}},"gcode_move":{"gcode_position":[0,0,40.25,0]},"virtual_sdcard":{"progress":0.5},"toolhead":{"extruder":"extruder2"},"extruder":{"temperature":25},"extruder2":{"temperature":217,"target":220},"heater_bed":{"temperature":59},"temperature_sensor cavity":{"temperature":31}}}}`, state, layer)
		case "/server/files/metadata":
			fmt.Fprint(w, `{"result":{"layer_count":302,"first_layer_height":0.25,"layer_height":0.2,"thumbnails":[{"width":300,"relative_path":".thumbs/test.png"}]}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	p := New(models.PrinterConfig{URL: server.URL, APIKey: "test-key"})
	p.client = server.Client()

	ctx := context.Background()
	if err := p.Connect(ctx); err != nil {
		t.Fatal(err)
	}
	status, err := p.GetStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != models.StatePrinting || status.Temps.HotendActual != 217 || !status.Temps.HasChamber {
		t.Fatalf("unexpected status: %+v", status)
	}
	if status.Job == nil || status.Job.Progress != 50 || status.Job.RemainingSecs != 90 || status.Job.TotalLayers != 302 || status.Job.CurrentLayer != 201 {
		t.Fatalf("unexpected print job: %+v", status.Job)
	}
	if status.ThumbnailURL != server.URL+"/server/files/gcodes/folder/.thumbs/test.png" {
		t.Fatalf("unexpected thumbnail: %q", status.ThumbnailURL)
	}
	if want := server.URL + "/server/files/camera/monitor.jpg"; p.GetSnapshotURL() != want || p.SnapshotLiveURL() != want {
		t.Fatalf("expected built-in camera at %s, got %q", want, p.GetSnapshotURL())
	}
	reportsLayer.Store(true)
	status, err = p.GetStatus(ctx)
	if err != nil || status.Job == nil || status.Job.CurrentLayer != 0 {
		t.Fatalf("expected explicitly reported layer zero, got %+v, %v", status, err)
	}
	completed.Store(true)
	status, err = p.GetStatus(ctx)
	if err != nil || status.State != models.StateIdle || status.Job == nil || status.Job.JobState != "FINISHED" || status.Job.Progress != 100 {
		t.Fatalf("expected a finished job for the completion checkpoint, got %+v, %v", status, err)
	}
}

func TestRemainingTimeSources(t *testing.T) {
	for _, tc := range []struct {
		name, metadata, history string
		remaining               int
	}{
		{"slicer before file bytes", `{"estimated_time":1416}`, `[]`, 336},
		{"completed unchanged file", `{"estimated_time":1416}`, `[{"filename":"test.gcode","exists":true,"status":"completed","print_duration":1605,"total_duration":1724}]`, 525},
		{"ignore cancelled changed and other files", `{"estimated_time":1416}`, `[{"filename":"test.gcode","exists":true,"status":"cancelled","print_duration":2000},{"filename":"test.gcode","exists":false,"status":"completed","print_duration":2000},{"filename":"other.gcode","exists":true,"status":"completed","print_duration":2000}]`, 336},
		{"missing slicer", `{}`, `[]`, 1374},
		{"exceeded slicer", `{"estimated_time":1000}`, `[]`, 1374},
		{"history unavailable", `{"estimated_time":1416}`, ``, 336},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var historyCalls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/printer/objects/query":
					fmt.Fprint(w, `{"result":{"status":{"webhooks":{"state":"ready"},"print_stats":{"state":"printing","filename":"test.gcode","print_duration":1080},"virtual_sdcard":{"progress":0.44}}}}`)
				case "/server/files/metadata":
					fmt.Fprintf(w, `{"result":%s}`, tc.metadata)
				case "/server/history/list":
					historyCalls.Add(1)
					if tc.history == "" {
						http.Error(w, "unsupported", 404)
						return
					}
					fmt.Fprintf(w, `{"result":{"jobs":%s}}`, tc.history)
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			p := New(models.PrinterConfig{URL: server.URL})
			p.client = server.Client()
			for range 2 {
				status, err := p.GetStatus(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				if status.Job.RemainingSecs != tc.remaining || status.Job.EstimatedTotal != 1080+tc.remaining {
					t.Fatalf("unexpected timing: %+v; want remaining %d", status.Job, tc.remaining)
				}
			}
			if historyCalls.Load() != 1 {
				t.Fatalf("history requested %d times", historyCalls.Load())
			}
		})
	}
}
