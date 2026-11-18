package api

import (
	"context"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLiveSnapshotStreamRefreshesFrames(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var frame int
		streamJPEGFrames(r.Context(), w, func(context.Context) ([]byte, error) {
			frame++
			return []byte{0xff, 0xd8, byte(frame), 0xff, 0xd9}, nil
		}, time.Millisecond)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	mediaType, params, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/x-mixed-replace" {
		t.Fatalf("unexpected stream content type: %q, %v", mediaType, err)
	}
	reader := multipart.NewReader(resp.Body, params["boundary"])
	for want := byte(1); want <= 2; want++ {
		part, err := reader.NextPart()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(part)
		if err != nil || len(data) != 5 || data[2] != want || part.Header.Get("Content-Type") != "image/jpeg" {
			t.Fatalf("unexpected frame %d: %v, %v", want, data, err)
		}
	}
}
