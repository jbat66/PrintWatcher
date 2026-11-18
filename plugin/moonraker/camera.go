package moonraker

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
	"github.com/jbat66/PrintWatcher/netguard"
)

func (p *Plugin) wakeCamera(ctx context.Context) error {
	p.cameraMu.Lock()
	defer p.cameraMu.Unlock()
	if time.Since(p.cameraWake) < 5*time.Second {
		return nil
	}
	address := p.base.JoinPath("websocket")
	address.Scheme = "ws"
	if p.base.Scheme == "https" {
		address.Scheme = "wss"
	}
	header := make(http.Header)
	if p.key != "" {
		header.Set("X-Api-Key", p.key)
	}
	dialer := websocket.Dialer{
		NetDialContext:   netguard.Transport().DialContext,
		HandshakeTimeout: 2 * time.Second,
	}
	conn, response, err := dialer.DialContext(ctx, address.String(), header)
	if err != nil {
		if response != nil {
			response.Body.Close()
		}
		return fmt.Errorf("connect to U1 camera monitor: %w", err)
	}
	defer conn.Close()
	conn.SetWriteDeadline(time.Now().Add(time.Second))
	// The printer starts updating monitor.jpg even on firmware that sends
	// no JSON-RPC reply. Confirm capture through the file's modification time.
	if err := conn.WriteJSON(map[string]any{
		"jsonrpc": "2.0",
		"method":  "camera.start_monitor",
		"params":  map[string]any{"domain": "lan", "interval": 0},
		"id":      1,
	}); err != nil {
		return err
	}
	p.cameraWake = time.Now()
	return nil
}

func (p *Plugin) SnapshotFrame(ctx context.Context) ([]byte, error) {
	if p.SnapshotLiveURL() == "" {
		return nil, fmt.Errorf("U1 camera is unavailable")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := p.wakeCamera(ctx); err != nil {
		return nil, err
	}
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		frame, err := p.fetchCameraJPEG(ctx)
		if err == nil {
			return frame, nil
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("waiting for a fresh U1 camera frame: %w", err)
		case <-ticker.C:
		}
	}
}

func (p *Plugin) fetchCameraJPEG(ctx context.Context) ([]byte, error) {
	address := p.base.JoinPath("server/files/camera/monitor.jpg")
	address.RawQuery = url.Values{"t": {strconv.FormatInt(time.Now().UnixNano(), 10)}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Cache-Control", "no-cache")
	resp, err := p.AuthenticatedDo(p.client, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("U1 camera returned HTTP %d", resp.StatusCode)
	}
	// Both headers use the printer's clock, which may differ from ours.
	date, dateErr := http.ParseTime(resp.Header.Get("Date"))
	modified, modifiedErr := http.ParseTime(resp.Header.Get("Last-Modified"))
	if dateErr != nil || modifiedErr != nil || date.Sub(modified) > 2*time.Second {
		return nil, fmt.Errorf("monitor.jpg has not been refreshed")
	}
	return readCameraJPEG(resp.Body)
}

func readCameraJPEG(r io.Reader) ([]byte, error) {
	const maxFrame = 10 << 20
	data, err := io.ReadAll(io.LimitReader(r, maxFrame+1))
	if err != nil {
		return nil, err
	}
	n := len(data)
	if n < 4 || n > maxFrame || data[0] != 0xff || data[1] != 0xd8 || data[n-2] != 0xff || data[n-1] != 0xd9 {
		return nil, fmt.Errorf("camera did not return a complete JPEG")
	}
	return data, nil
}
