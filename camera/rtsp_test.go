package camera

import (
	"bytes"
	"context"
	"image/jpeg"
	"io"
	"mime/multipart"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

func TestValidateURL(t *testing.T) {
	for _, raw := range []string{"http://camera.local", "https://192.168.1.2:8443", "rtsp://user:p%40ss@192.168.1.2:554/live?channel=1", "rtsp://[fd00::1]/stream"} {
		if err := ValidateURL(raw); err != nil {
			t.Errorf("%q: %v", raw, err)
		}
	}
	for _, raw := range []string{"", "camera.local", "file:///etc/passwd", "ftp://host/a", "rtsp:///stream", "rtsp://host:99999/stream", "http://host/#fragment"} {
		if err := ValidateURL(raw); err == nil {
			t.Errorf("accepted %q", raw)
		}
	}
}

// Optional integration check against a real RTSP server, using FFmpeg on PATH.
func TestRTSPIntegration(t *testing.T) {
	raw := os.Getenv("PRINTWATCHER_TEST_RTSP_URL")
	if raw == "" {
		t.Skip("set PRINTWATCHER_TEST_RTSP_URL to test a running RTSP camera")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	data, err := Snapshot(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := jpeg.Decode(bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}
	streamCtx, stop := context.WithCancel(ctx)
	s, err := Start(streamCtx, raw, false)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	r := multipart.NewReader(s, "printwatcher")
	for i := 0; i < 2; i++ {
		part, err := r.NextPart()
		if err != nil {
			t.Fatal(err)
		}
		frame, err := io.ReadAll(part)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := jpeg.Decode(bytes.NewReader(frame)); err != nil {
			t.Fatal(err)
		}
	}
	stop()
	done := make(chan struct{})
	go func() { s.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("decoder did not stop on cancellation")
	}
	if len(decoders) != 0 {
		t.Fatal("decoder slot leaked")
	}
}

func TestResolveBlocksLocalAndMetadataAddresses(t *testing.T) {
	for _, host := range []string{"127.0.0.1", "0.0.0.0", "169.254.169.254", "[::1]", "[::]", "[fe80::1]", "224.0.0.1", "[::ffff:127.0.0.1]"} {
		if _, err := resolve(context.Background(), "rtsp://"+host+"/stream"); err == nil {
			t.Errorf("accepted %s", host)
		}
	}
}

func TestResolvePreservesCredentialsPathAndQuery(t *testing.T) {
	raw := "rtsp://user:p%40ss@192.168.1.20:8554/live/main?channel=1&subtype=0"
	got, err := resolve(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(got)
	pass, _ := u.User.Password()
	if u.Host != "192.168.1.20:8554" || pass != "p@ss" || u.Path != "/live/main" || u.RawQuery != "channel=1&subtype=0" {
		t.Fatalf("URL changed: %s", got)
	}
	got, err = resolve(context.Background(), "rtsp://192.168.1.20/live")
	if err != nil || !strings.Contains(got, ":554/live") {
		t.Fatalf("default port: %s, %v", got, err)
	}
}
