// Package camera converts RTSP video to browser-compatible JPEG images.
package camera

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

const MaxImageBytes = 10 << 20

// Bound concurrent decoders, including notification snapshots.
var decoders = make(chan struct{}, 8)

func IsRTSP(raw string) bool { return strings.HasPrefix(strings.ToLower(raw), "rtsp:") }

func ValidateURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.Fragment != "" {
		return errors.New("a valid HTTP, HTTPS or RTSP camera URL is required")
	}
	if u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "rtsp" {
		return errors.New("camera URL must use http://, https:// or rtsp://")
	}
	if p := u.Port(); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 || n > 65535 {
			return errors.New("invalid camera port")
		}
	}
	return nil
}

// Resolve once and pin the address so FFmpeg cannot resolve a different address
// after validation. LAN addresses are allowed, matching the HTTP camera policy.
func resolve(ctx context.Context, raw string) (string, error) {
	if err := ValidateURL(raw); err != nil {
		return "", err
	}
	u, _ := url.Parse(raw)
	if u.Scheme != "rtsp" {
		return "", errors.New("RTSP URL required")
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, u.Hostname())
	if err != nil || len(ips) == 0 {
		return "", errors.New("cannot resolve RTSP camera")
	}
	for _, a := range ips {
		ip := a.IP
		if ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || a.Zone != "" {
			return "", errors.New("blocked RTSP camera address")
		}
	}
	port := u.Port()
	if port == "" {
		port = "554"
	}
	u.Host = net.JoinHostPort(ips[0].IP.String(), port)
	return u.String(), nil
}

// Start returns an MJPEG stream or one JPEG. Closing it stops and reaps FFmpeg.
// Error messages intentionally exclude camera credentials and FFmpeg stderr.
func Start(ctx context.Context, raw string, snapshot bool) (io.ReadCloser, error) {
	select {
	case decoders <- struct{}{}:
	default:
		return nil, errors.New("all RTSP decoders are busy")
	}
	release := func() { <-decoders }
	ctx, cancel := context.WithCancel(ctx)
	address, err := resolve(ctx, raw)
	if err != nil {
		cancel()
		release()
		return nil, err
	}
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-rtsp_transport", "tcp", "-timeout", "10000000", "-protocol_whitelist", "tcp,rtp", "-analyzeduration", "1000000", "-probesize", "1000000", "-threads", "1", "-i", address, "-map", "0:v:0", "-an", "-threads", "1", "-vf", "scale='min(1280,iw)':-2", "-c:v", "mjpeg", "-q:v", "5"}
	if snapshot {
		args = append(args, "-frames:v", "1", "-f", "image2pipe")
	} else {
		args = append(args, "-r", "5", "-f", "mpjpeg", "-boundary_tag", "printwatcher")
	}
	args = append(args, "pipe:1")
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	pipe, err := cmd.StdoutPipe()
	if err == nil {
		err = cmd.Start()
	}
	if err != nil {
		cancel()
		release()
		return nil, errors.New("RTSP requires FFmpeg installed on the server and available on PATH")
	}
	s := &stream{cmd: cmd, pipe: pipe, cancel: cancel, release: release}
	// A camera that connects but never supplies video must not hang the request.
	timer := time.AfterFunc(20*time.Second, cancel)
	reader := bufio.NewReader(pipe)
	_, err = reader.Peek(1)
	timer.Stop()
	if err != nil {
		s.Close()
		return nil, errors.New("cannot decode RTSP camera; check its URL, credentials and connectivity")
	}
	s.reader = reader
	return s, nil
}

type stream struct {
	cmd     *exec.Cmd
	pipe    io.ReadCloser
	reader  *bufio.Reader
	cancel  context.CancelFunc
	release func()
	once    sync.Once
	err     error
}

func (s *stream) Read(p []byte) (int, error) { return s.reader.Read(p) }
func (s *stream) Close() error {
	s.once.Do(func() {
		s.cancel()
		s.pipe.Close()
		s.err = s.cmd.Wait()
		s.release()
	})
	return s.err
}

func Snapshot(ctx context.Context, raw string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	s, err := Start(ctx, raw, true)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	data, err := io.ReadAll(io.LimitReader(s, MaxImageBytes+1))
	if err != nil {
		return nil, errors.New("cannot read RTSP snapshot")
	}
	if len(data) > MaxImageBytes {
		return nil, fmt.Errorf("RTSP snapshot exceeds %d bytes", MaxImageBytes)
	}
	if len(data) < 4 || !bytes.HasPrefix(data, []byte{0xff, 0xd8}) || !bytes.HasSuffix(data, []byte{0xff, 0xd9}) {
		return nil, errors.New("RTSP camera did not supply a complete JPEG")
	}
	return data, nil
}
