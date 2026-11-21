package moonraker

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jbat66/PrintWatcher/models"
)

func TestUploadToGcodesAndOptionalPrint(t *testing.T) {
	for _, printNow := range []bool{false, true} {
		t.Run(fmt.Sprint(printNow), func(t *testing.T) {
			starts := 0
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.Header.Get("X-Api-Key") != "test-key" {
					t.Error("missing upload authentication or POST")
				}
				switch r.URL.Path {
				case "/server/files/upload":
					if err := r.ParseMultipartForm(1 << 20); err != nil {
						t.Error(err)
						return
					}
					defer r.MultipartForm.RemoveAll()
					if r.FormValue("root") != "gcodes" {
						t.Errorf("wrong root: %q", r.FormValue("root"))
					}
					file, header, err := r.FormFile("file")
					if err != nil {
						t.Error(err)
						return
					}
					defer file.Close()
					data, _ := io.ReadAll(file)
					if header.Filename != "part.gcode" || string(data) != "G1 X1\n" {
						t.Error("wrong file contents")
					}
					fmt.Fprint(w, `{"item":{"path":"part.gcode","modified":123}}`)
				case "/printer/print/start":
					starts++
					if filename := r.URL.Query().Get("filename"); filename != "part.gcode" {
						t.Errorf("wrong print path: %q", filename)
					}
					fmt.Fprint(w, `{"result":"ok"}`)
				default:
					t.Errorf("unexpected endpoint %s", r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			defer s.Close()
			p := New(models.PrinterConfig{URL: s.URL, APIKey: "test-key"})
			p.client = s.Client()
			path, stamp, err := p.UploadFile(context.Background(), "gcodes", "part.gcode", []byte("G1 X1\n"), printNow)
			if err != nil || path != "part.gcode" || stamp != 123 {
				t.Fatalf("%q %d %v", path, stamp, err)
			}
			want := 0
			if printNow {
				want = 1
			}
			if starts != want {
				t.Fatalf("print starts=%d, want %d", starts, want)
			}
		})
	}
}
