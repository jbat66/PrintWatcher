package api

import (
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strconv"
	"time"
)

func (h *Handler) serveSnapshotLive(w http.ResponseWriter, r *http.Request, id int64, address string) {
	ctx := r.Context()
	fetch := func(ctx context.Context) ([]byte, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Cache-Control", "no-cache")
		resp, err := h.poller.AuthenticatedDo(id, h.proxy, req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		return readJPEGFrame(resp)
	}
	if err := streamJPEGFrames(ctx, w, fetch, time.Second); err != nil && ctx.Err() == nil {
		http.Error(w, "failed to fetch live camera image", http.StatusBadGateway)
	}
}

func readJPEGFrame(resp *http.Response) ([]byte, error) {
	const maxFrame = 10 << 20
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("camera returned HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxFrame+1))
	if err != nil {
		return nil, err
	}
	n := len(data)
	if n < 4 || n > maxFrame || data[0] != 0xff || data[1] != 0xd8 || data[n-2] != 0xff || data[n-1] != 0xd9 {
		return nil, fmt.Errorf("camera did not return a complete JPEG")
	}
	return data, nil
}

func streamJPEGFrames(ctx context.Context, w http.ResponseWriter, fetch func(context.Context) ([]byte, error), interval time.Duration) error {
	load := func() ([]byte, error) {
		frameCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		return fetch(frameCtx)
	}
	frame, err := load()
	if err != nil {
		return err
	}
	writer := multipart.NewWriter(w)
	w.Header().Set("Content-Type", "multipart/x-mixed-replace; boundary="+writer.Boundary())
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	controller := http.NewResponseController(w)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		// The printer may be replacing the JPEG. Keep showing the previous
		// frame until a complete image becomes available again.
		if err == nil {
			part, writeErr := writer.CreatePart(textproto.MIMEHeader{
				"Content-Type":   {"image/jpeg"},
				"Content-Length": {strconv.Itoa(len(frame))},
			})
			if writeErr != nil {
				return nil
			}
			if _, writeErr = part.Write(frame); writeErr != nil {
				return nil
			}
			if controller.Flush() != nil {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			frame, err = load()
		}
	}
}
