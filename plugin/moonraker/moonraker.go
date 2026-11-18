package moonraker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/jbat66/PrintWatcher/models"
	"github.com/jbat66/PrintWatcher/netguard"
	"github.com/jbat66/PrintWatcher/plugin"
)

const printerType = "snapmaker-u1"

func init() {
	plugin.Register(printerType, func(cfg models.PrinterConfig) plugin.PrinterPlugin { return New(cfg) })
}

type Plugin struct {
	base   *url.URL
	key    string
	client *http.Client
}

func New(cfg models.PrinterConfig) *Plugin {
	base, err := url.Parse(strings.TrimRight(cfg.URL, "/"))
	if err != nil || base.Hostname() == "" || base.User != nil || base.RawQuery != "" || base.Fragment != "" || (base.Scheme != "http" && base.Scheme != "https") {
		base = nil
	}
	return &Plugin{base: base, key: cfg.APIKey, client: &http.Client{Transport: netguard.Transport(), Timeout: 90 * time.Second}}
}

func (p *Plugin) Type() string                          { return printerType }
func (p *Plugin) DisplayName() string                   { return "Snapmaker U1" }
func (p *Plugin) GetPrinterName(context.Context) string { return p.DisplayName() }
func (p *Plugin) GetWebcamURL() string                  { return "" }
func (p *Plugin) GetSnapshotURL() string                { return "" }
func (p *Plugin) GetThumbnailURL(ctx context.Context) string {
	status, err := p.GetStatus(ctx)
	if err != nil {
		return ""
	}
	return status.ThumbnailURL
}
func (p *Plugin) SetPowerState(context.Context, string, bool) error {
	return fmt.Errorf("Moonraker printer power requires an assigned smart plug")
}

func (p *Plugin) AuthenticatedDo(client *http.Client, req *http.Request) (*http.Response, error) {
	if p.base == nil || req.URL.Scheme != p.base.Scheme || req.URL.Host != p.base.Host {
		return nil, fmt.Errorf("Moonraker resource must use the configured printer address")
	}
	if p.key != "" {
		req.Header.Set("X-Api-Key", p.key)
	}
	// Do not forward the key, or replay print commands, on redirects.
	local := *client
	local.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return local.Do(req)
}

func (p *Plugin) request(ctx context.Context, method, endpoint string, query url.Values, body io.Reader, contentType string, limit int64) ([]byte, error) {
	if p.base == nil {
		return nil, fmt.Errorf("enter a valid HTTP or HTTPS Moonraker URL")
	}
	address := p.base.JoinPath(endpoint)
	address.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, method, address.String(), body)
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := p.AuthenticatedDo(p.client, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("Moonraker rejected access. Check the API key and printer authorization settings")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("Moonraker returned HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("Moonraker response exceeds %d bytes", limit)
	}
	return data, err
}

func (p *Plugin) query(ctx context.Context, endpoint string, query url.Values, out any) error {
	data, err := p.request(ctx, http.MethodGet, endpoint, query, nil, "", 16<<20)
	if err != nil {
		return err
	}
	var envelope struct{ Result json.RawMessage }
	if err := json.Unmarshal(data, &envelope); err != nil || len(envelope.Result) == 0 {
		return fmt.Errorf("expected a Moonraker API response. Try the printer's Moonraker port (usually 7125)")
	}
	return json.Unmarshal(envelope.Result, out)
}

func (p *Plugin) Connect(ctx context.Context) error {
	var info struct {
		Version string `json:"moonraker_version"`
	}
	if err := p.query(ctx, "server/info", nil, &info); err != nil {
		return err
	}
	if info.Version == "" {
		return fmt.Errorf("Moonraker version missing from server info")
	}
	_, err := p.status(ctx)
	return err
}

type heater struct{ Temperature, Target float64 }

type objectStatus struct {
	Webhooks struct {
		State   string
		Message string `json:"state_message"`
	}
	Print struct {
		State, Filename, Message string
		Duration                 float64 `json:"print_duration"`
		Filament                 float64 `json:"filament_used"`
		Info                     struct {
			Current int `json:"current_layer"`
			Total   int `json:"total_layer"`
		}
	} `json:"print_stats"`
	SD        struct{ Progress float64 } `json:"virtual_sdcard"`
	Toolhead  struct{ Extruder string }
	Extruder  heater
	Extruder1 heater
	Extruder2 heater
	Extruder3 heater
	Bed       heater  `json:"heater_bed"`
	Chamber   *heater `json:"temperature_sensor cavity"`
}

func (p *Plugin) status(ctx context.Context) (objectStatus, error) {
	var result struct{ Status objectStatus }
	query := url.Values{"webhooks": {""}, "print_stats": {""}, "virtual_sdcard": {"progress"}, "toolhead": {"extruder"}}
	for _, name := range []string{"extruder", "extruder1", "extruder2", "extruder3", "heater_bed", "temperature_sensor cavity"} {
		query.Set(name, "temperature,target")
	}
	err := p.query(ctx, "printer/objects/query", query, &result)
	if err == nil && result.Status.Webhooks.State == "" {
		err = fmt.Errorf("Moonraker did not return Klipper status")
	}
	return result.Status, err
}

func (p *Plugin) GetStatus(ctx context.Context) (*models.PrinterStatus, error) {
	s, err := p.status(ctx)
	if err != nil {
		return nil, err
	}
	hotend := s.Extruder
	switch s.Toolhead.Extruder {
	case "extruder1":
		hotend = s.Extruder1
	case "extruder2":
		hotend = s.Extruder2
	case "extruder3":
		hotend = s.Extruder3
	}
	out := &models.PrinterStatus{
		State: models.StateIdle, LastUpdated: time.Now(),
		Temps: models.Temperatures{HotendActual: hotend.Temperature, HotendTarget: hotend.Target, BedActual: s.Bed.Temperature, BedTarget: s.Bed.Target},
	}
	if s.Chamber != nil {
		out.Temps.HasChamber = true
		out.Temps.ChamberActual = s.Chamber.Temperature
	}
	jobState := ""
	switch s.Print.State {
	case "printing":
		out.State = models.StatePrinting
	case "paused":
		out.State = models.StatePaused
	case "complete":
		jobState = "FINISHED"
	case "cancelled":
		jobState = "STOPPED"
	case "error":
		out.State, out.StateMessage, jobState = models.StateError, s.Print.Message, "ERROR"
	}
	if s.Webhooks.State != "ready" {
		out.State, out.StateMessage = models.StateDisconnected, s.Webhooks.Message
		if s.Webhooks.State == "error" || s.Webhooks.State == "shutdown" {
			out.State, jobState = models.StateError, "ERROR"
		}
	}
	if s.Print.Filename == "" || (jobState == "" && out.State != models.StatePrinting && out.State != models.StatePaused) {
		return out, nil
	}
	progress := math.Max(0, math.Min(1, s.SD.Progress))
	if jobState == "FINISHED" {
		progress = 1
	}
	job := &models.JobInfo{
		FileName: path.Base(s.Print.Filename), FilePath: s.Print.Filename, JobState: jobState,
		Progress: progress * 100, ElapsedSecs: int(s.Print.Duration), FilamentUsedMM: s.Print.Filament,
		CurrentLayer: s.Print.Info.Current, TotalLayers: s.Print.Info.Total,
	}
	out.Job = job
	if out.State != models.StatePrinting && out.State != models.StatePaused {
		return out, nil
	}

	if progress > 0 && job.ElapsedSecs > 0 {
		job.EstimatedTotal = int(s.Print.Duration / progress)
		job.RemainingSecs = job.EstimatedTotal - job.ElapsedSecs
	}
	var metadata struct {
		EstimatedTime float64 `json:"estimated_time"`
		LayerCount    int     `json:"layer_count"`
		Thumbnails    []struct {
			Width        int
			RelativePath string `json:"relative_path"`
		}
	}
	metaCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if p.query(metaCtx, "server/files/metadata", url.Values{"filename": {job.FilePath}}, &metadata) == nil {
		if job.TotalLayers == 0 {
			job.TotalLayers = metadata.LayerCount
		}
		if job.EstimatedTotal == 0 && metadata.EstimatedTime > 0 {
			job.EstimatedTotal = int(metadata.EstimatedTime)
			job.RemainingSecs = max(0, job.EstimatedTotal-job.ElapsedSecs)
		}
		width := 0
		for _, thumb := range metadata.Thumbnails {
			if thumb.Width > width && validFile(thumb.RelativePath) {
				fullPath := path.Join(path.Dir(job.FilePath), thumb.RelativePath)
				if validFile(fullPath) {
					out.ThumbnailURL = p.base.JoinPath("server/files/gcodes", fullPath).String()
					width = thumb.Width
				}
			}
		}
	}
	return out, nil
}

func validFile(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.HasPrefix(name, "/") && !strings.HasPrefix(name, "../") && !strings.ContainsAny(name, "\\%\x00") && path.Clean(name) == name
}

func (p *Plugin) GetRecentFiles(ctx context.Context, limit int) ([]models.RecentFile, error) {
	var files []struct {
		Path     string
		Modified float64
		Size     int64
	}
	if err := p.query(ctx, "server/files/list", url.Values{"root": {"gcodes"}}, &files); err != nil {
		return nil, err
	}
	sort.SliceStable(files, func(i, j int) bool { return files[i].Modified > files[j].Modified })
	out := make([]models.RecentFile, 0, len(files))
	for _, file := range files {
		if !validFile(file.Path) || !strings.EqualFold(path.Ext(file.Path), ".gcode") {
			continue
		}
		out = append(out, models.RecentFile{FileName: path.Base(file.Path), Path: file.Path, Origin: "local", UploadedAt: int64(file.Modified), SizeMB: float64(file.Size) / (1 << 20)})
		if limit > 0 && len(out) == limit {
			break
		}
	}
	return out, nil
}

func (p *Plugin) UploadFile(ctx context.Context, _ string, filename string, data []byte, printAfter bool) (string, int64, error) {
	if !validFile(filename) || !strings.EqualFold(path.Ext(filename), ".gcode") {
		return "", 0, fmt.Errorf("Snapmaker U1 requires a .gcode file")
	}
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	if err := form.WriteField("root", "gcodes"); err != nil {
		return "", 0, err
	}
	if dir := path.Dir(filename); dir != "." {
		if err := form.WriteField("path", dir); err != nil {
			return "", 0, err
		}
	}
	part, err := form.CreateFormFile("file", path.Base(filename))
	if err != nil {
		return "", 0, err
	}
	if _, err := part.Write(data); err != nil {
		return "", 0, err
	}
	if err := form.Close(); err != nil {
		return "", 0, err
	}
	response, err := p.request(ctx, http.MethodPost, "server/files/upload", nil, &body, form.FormDataContentType(), 1<<20)
	if err != nil {
		return "", 0, err
	}
	var uploaded struct {
		Item struct {
			Path     string
			Modified float64
		}
	}
	if err := json.Unmarshal(response, &uploaded); err != nil || !validFile(uploaded.Item.Path) {
		return "", 0, fmt.Errorf("Moonraker did not confirm the uploaded file path")
	}
	if printAfter {
		err = p.StartPrint(ctx, "local", uploaded.Item.Path)
	}
	return uploaded.Item.Path, int64(uploaded.Item.Modified), err
}

func (p *Plugin) fileRequest(ctx context.Context, method, filename string) ([]byte, error) {
	if !validFile(filename) {
		return nil, fmt.Errorf("invalid Moonraker file path")
	}
	return p.request(ctx, method, "server/files/gcodes/"+filename, nil, nil, "", 200<<20)
}
func (p *Plugin) DeleteFile(ctx context.Context, _ string, filename string) error {
	_, err := p.fileRequest(ctx, http.MethodDelete, filename)
	return err
}
func (p *Plugin) DownloadFile(ctx context.Context, _ string, filename string) ([]byte, error) {
	return p.fileRequest(ctx, http.MethodGet, filename)
}
func (p *Plugin) command(ctx context.Context, action string, query url.Values) error {
	_, err := p.request(ctx, http.MethodPost, "printer/print/"+action, query, nil, "", 1<<20)
	return err
}
func (p *Plugin) StartPrint(ctx context.Context, _ string, filename string) error {
	if !validFile(filename) {
		return fmt.Errorf("invalid Moonraker file path")
	}
	return p.command(ctx, "start", url.Values{"filename": {filename}})
}
func (p *Plugin) PausePrint(ctx context.Context) error  { return p.command(ctx, "pause", nil) }
func (p *Plugin) ResumePrint(ctx context.Context) error { return p.command(ctx, "resume", nil) }
func (p *Plugin) CancelPrint(ctx context.Context) error { return p.command(ctx, "cancel", nil) }
