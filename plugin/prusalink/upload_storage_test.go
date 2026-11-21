package prusalink

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jbat66/PrintWatcher/models"
)

func TestUploadUsesUSBWithOptionalPrint(t *testing.T) {
	for _, ext := range []string{"gcode", "bgcode"} {
		for _, printNow := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%t", ext, printNow), func(t *testing.T) {
				s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method != http.MethodPut || r.URL.Path != "/api/v1/files/usb/part."+ext {
						t.Errorf("unexpected destination: %s %s", r.Method, r.URL.Path)
					}
					if (r.Header.Get("Print-After-Upload") == "?1") != printNow {
						t.Error("incorrect Print-After-Upload")
					}
					fmt.Fprint(w, `{"name":"part.gcode","m_timestamp":123}`)
				}))
				defer s.Close()
				p := New(models.PrinterConfig{URL: s.URL})
				p.uploadClient = s.Client()
				if _, _, err := p.UploadFile(context.Background(), "usb", "part."+ext, []byte("G1 X1\n"), printNow); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}
