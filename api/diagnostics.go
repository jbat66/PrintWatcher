package api

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Bound simultaneous processes across all viewers. No diagnostics run without
// a request; aborting the browser request cancels its ping processes.
var diagnosticSlots = make(chan struct{}, 8)
var pingTime = regexp.MustCompile(`(?i)time\s*([=<])\s*([0-9]+(?:\.[0-9]+)?)\s*ms`)

type diagnosticSample struct {
	MS       *float64 `json:"ms,omitempty"`
	Error    string   `json:"error,omitempty"`
	SameHost bool     `json:"same_host,omitempty"`
}

func diagnosticHost(raw, base string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid host")
	}
	if u.Hostname() == "" && base != "" {
		b, err := url.Parse(base)
		if err != nil {
			return "", fmt.Errorf("invalid host")
		}
		u = b.ResolveReference(u)
	}
	if u.Hostname() == "" {
		return "", fmt.Errorf("no host configured")
	}
	return u.Hostname(), nil
}

func diagnosticIP(ctx context.Context, host string) (string, error) {
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil || len(ips) == 0 {
		return "", fmt.Errorf("Host lookup failed")
	}
	for _, addr := range ips {
		ip := addr.IP
		if ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() {
			return "", fmt.Errorf("Host address blocked")
		}
	}
	return ips[0].IP.String(), nil
}

func parseDiagnosticPing(out string, err error) diagnosticSample {
	if err == nil {
		if match := pingTime.FindStringSubmatch(out); match != nil {
			ms, _ := strconv.ParseFloat(match[2], 64)
			if match[1] == "<" {
				ms /= 2
			}
			return diagnosticSample{MS: &ms}
		}
	}
	lower := strings.ToLower(out)
	message := "No ping reply"
	switch {
	case strings.Contains(lower, "unreachable"):
		message = "Host unreachable"
	case strings.Contains(lower, "permitted"), strings.Contains(lower, "permission"), strings.Contains(lower, "cap_net_raw"):
		message = "Ping permission denied"
	case strings.Contains(lower, "not found"):
		message = "Ping unavailable"
	}
	return diagnosticSample{Error: message}
}

func pingDiagnostic(ctx context.Context, host string) diagnosticSample {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	ip, err := diagnosticIP(ctx, host)
	if err != nil {
		return diagnosticSample{Error: err.Error()}
	}
	args := []string{"-n", "-c", "1", "-W", "1", ip}
	if runtime.GOOS == "windows" {
		args = []string{"-n", "1", "-w", "1000", ip}
	}
	cmd := exec.CommandContext(ctx, "ping", args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	out, err := cmd.CombinedOutput()
	if _, ok := err.(*exec.Error); ok {
		return diagnosticSample{Error: "Ping unavailable"}
	}
	return parseDiagnosticPing(string(out), err)
}

func (h *Handler) getDiagnostics(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	p, err := h.db.GetPrinter(id)
	if err != nil {
		jsonError(w, "printer not found", http.StatusNotFound)
		return
	}
	select {
	case diagnosticSlots <- struct{}{}:
		defer func() { <-diagnosticSlots }()
	default:
		jsonError(w, "Diagnostics busy; retrying", http.StatusTooManyRequests)
		return
	}
	printerHost, err := diagnosticHost(p.URL, "")
	if err != nil {
		jsonError(w, "No printer host configured", http.StatusBadRequest)
		return
	}
	cameraURL := ""
	if cam, err := h.db.GetCameraForPrinter(id); err == nil {
		cameraURL = cam.URL
	} else if h.poller != nil {
		cameraURL = h.poller.GetSnapshotURL(id)
		if cameraURL == "" {
			cameraURL = h.poller.GetWebcamURL(id)
		}
	}
	result := struct {
		Printer diagnosticSample  `json:"printer"`
		Camera  *diagnosticSample `json:"camera,omitempty"`
	}{}
	var wg sync.WaitGroup
	wg.Go(func() { result.Printer = pingDiagnostic(r.Context(), printerHost) })
	if cameraURL != "" {
		cameraHost, err := diagnosticHost(cameraURL, p.URL)
		if err != nil {
			result.Camera = &diagnosticSample{Error: "No camera host configured"}
		} else if strings.EqualFold(cameraHost, printerHost) {
			result.Camera = &diagnosticSample{SameHost: true}
		} else {
			wg.Go(func() { sample := pingDiagnostic(r.Context(), cameraHost); result.Camera = &sample })
		}
	}
	wg.Wait()
	w.Header().Set("Cache-Control", "no-store")
	jsonResponse(w, result)
}
