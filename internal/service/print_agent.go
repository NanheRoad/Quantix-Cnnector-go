package service

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"quantix-connector-go/internal/config"
)

var defaultBarTenderExecutableCandidates = []string{
	`C:\Program Files\Seagull\BarTender 2022\BarTend.exe`,
	`C:\Program Files\Seagull\BarTender Suite\BarTend.exe`,
	`C:\Program Files (x86)\Seagull\BarTender 2022\BarTend.exe`,
	`C:\Program Files (x86)\Seagull\BarTender Suite\BarTend.exe`,
}

func ListBarTenderExecutableCandidates() []string {
	out := make([]string, 0, len(defaultBarTenderExecutableCandidates))
	seen := map[string]struct{}{}
	for _, candidate := range defaultBarTenderExecutableCandidates {
		if strings.TrimSpace(candidate) == "" {
			continue
		}
		if _, err := os.Stat(candidate); err != nil {
			continue
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		out = append(out, candidate)
	}
	return out
}

type PrintAgentStatus struct {
	Enabled        bool      `json:"enabled"`
	Running        bool      `json:"running"`
	WorkerCount    int       `json:"worker_count"`
	ActiveJobs     int       `json:"active_jobs"`
	ServerURL      string    `json:"server_url"`
	ClientID       string    `json:"client_id"`
	JobType        string    `json:"job_type"`
	LastPollAt     time.Time `json:"last_poll_at"`
	LastSuccessAt  time.Time `json:"last_success_at"`
	LastErrorAt    time.Time `json:"last_error_at"`
	LastError      string    `json:"last_error"`
	CurrentJobCode string    `json:"current_job_code"`
	ClaimedCount   int       `json:"claimed_count"`
	SuccessCount   int       `json:"success_count"`
	FailedCount    int       `json:"failed_count"`
}

type PrintAgentJobRecord struct {
	Time         time.Time      `json:"time"`
	JobID        int            `json:"job_id"`
	JobCode      string         `json:"job_code"`
	TemplateCode string         `json:"template_code"`
	PrinterName  string         `json:"printer_name"`
	Status       string         `json:"status"`
	Message      string         `json:"message"`
	Result       map[string]any `json:"result"`
}

type DirectPrintJob struct {
	JobCode      string         `json:"job_code"`
	JobType      string         `json:"job_type"`
	TemplateCode string         `json:"template_code"`
	PrinterName  string         `json:"printer_name"`
	Copies       int            `json:"copies"`
	Payload      map[string]any `json:"payload"`
}

type PrintAgentService struct {
	mu      sync.Mutex
	cfg     config.PrintAgentSettings
	status  PrintAgentStatus
	history []PrintAgentJobRecord
	cancel  context.CancelFunc
	done    chan struct{}
	client  *http.Client
	execSem chan struct{}
}

func NewPrintAgentService(cfg config.PrintAgentSettings) *PrintAgentService {
	cfg = normalizeAgentCfg(cfg)
	return &PrintAgentService{
		cfg: cfg,
		status: PrintAgentStatus{
			Enabled:     cfg.Enabled,
			ServerURL:   cfg.ServerURL,
			ClientID:    cfg.ClientID,
			JobType:     cfg.JobType,
			WorkerCount: cfg.MaxConcurrentJobs,
		},
		client:  &http.Client{Timeout: 30 * time.Second},
		execSem: make(chan struct{}, cfg.MaxConcurrentJobs),
	}
}

func (s *PrintAgentService) Start(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.startLocked(ctx, s.cfg)
}

func (s *PrintAgentService) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	cancel := s.cancel
	done := s.done
	s.cancel = nil
	s.done = nil
	s.status.Running = false
	s.status.ActiveJobs = 0
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func (s *PrintAgentService) UpdateConfig(ctx context.Context, cfg config.PrintAgentSettings) {
	cfg = normalizeAgentCfg(cfg)
	s.mu.Lock()
	s.cfg = cfg
	s.status.Enabled = cfg.Enabled
	s.status.ServerURL = cfg.ServerURL
	s.status.ClientID = cfg.ClientID
	s.status.JobType = cfg.JobType
	s.status.WorkerCount = cfg.MaxConcurrentJobs
	s.execSem = make(chan struct{}, cfg.MaxConcurrentJobs)
	cancel := s.cancel
	done := s.done
	s.cancel = nil
	s.done = nil
	s.status.Running = false
	s.status.ActiveJobs = 0
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
			return
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.startLocked(ctx, cfg)
}

func (s *PrintAgentService) startLocked(_ context.Context, cfg config.PrintAgentSettings) {
	if !cfg.Enabled {
		return
	}
	workerCount := cfg.MaxConcurrentJobs
	if workerCount <= 0 {
		workerCount = 1
	}
	loopCtx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	s.cancel = cancel
	s.done = done
	s.status.Running = true
	s.status.WorkerCount = workerCount
	go func(localCfg config.PrintAgentSettings, workers int) {
		defer close(done)
		var wg sync.WaitGroup
		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				s.loop(loopCtx, localCfg)
			}()
		}
		wg.Wait()
		s.mu.Lock()
		if s.done == done {
			s.status.Running = false
			s.status.ActiveJobs = 0
		}
		s.mu.Unlock()
	}(cfg, workerCount)
}

func (s *PrintAgentService) CurrentConfig() config.PrintAgentSettings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return clonePrintAgentConfig(s.cfg)
}

func (s *PrintAgentService) Status() PrintAgentStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

func (s *PrintAgentService) Jobs(limit int) []PrintAgentJobRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 || limit > len(s.history) {
		limit = len(s.history)
	}
	out := make([]PrintAgentJobRecord, 0, limit)
	for i := len(s.history) - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, s.history[i])
	}
	return out
}

func (s *PrintAgentService) TriggerPoll() error {
	s.mu.Lock()
	cfg := s.cfg
	s.mu.Unlock()
	if !cfg.Enabled {
		return fmt.Errorf("print agent is disabled")
	}
	_, err := s.pollOnce(context.Background(), cfg)
	if isContextCanceledErr(err) {
		return nil
	}
	return err
}

func (s *PrintAgentService) ExecuteDirectJob(ctx context.Context, req DirectPrintJob) (PrintAgentJobRecord, error) {
	s.mu.Lock()
	cfg := s.cfg
	s.mu.Unlock()
	if strings.TrimSpace(req.TemplateCode) == "" {
		return PrintAgentJobRecord{}, fmt.Errorf("template_code is required")
	}
	jobType := strings.TrimSpace(req.JobType)
	if jobType == "" {
		jobType = cfg.JobType
	}
	if jobType == "" {
		jobType = "bartender"
	}
	if !strings.EqualFold(jobType, "bartender") {
		return PrintAgentJobRecord{}, fmt.Errorf("unsupported job_type: %s", jobType)
	}
	jobCode := strings.TrimSpace(req.JobCode)
	if jobCode == "" {
		jobCode = fmt.Sprintf("DIRECT-%s", time.Now().UTC().Format("20060102-150405.000000"))
	}
	payload := req.Payload
	if payload == nil {
		payload = map[string]any{}
	}
	task := &remotePrintJob{
		JobCode:      jobCode,
		TemplateCode: strings.TrimSpace(req.TemplateCode),
		PrinterName:  strings.TrimSpace(req.PrinterName),
		Copies:       req.Copies,
		Payload:      payload,
	}
	s.mu.Lock()
	s.status.CurrentJobCode = task.JobCode
	s.status.ActiveJobs++
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		if s.status.ActiveJobs > 0 {
			s.status.ActiveJobs--
		}
		if strings.TrimSpace(s.status.CurrentJobCode) == strings.TrimSpace(task.JobCode) {
			s.status.CurrentJobCode = ""
		}
		s.mu.Unlock()
	}()
	select {
	case <-ctx.Done():
		return PrintAgentJobRecord{}, ctx.Err()
	default:
	}
	result, execErr := s.executeTaskLimited(ctx, task, cfg)
	record := PrintAgentJobRecord{
		Time:         time.Now(),
		JobCode:      task.JobCode,
		TemplateCode: task.TemplateCode,
		PrinterName:  task.PrinterName,
		Result:       result,
	}
	if execErr != nil {
		record.Status = "failed"
		record.Message = execErr.Error()
		s.pushHistory(record)
		s.mu.Lock()
		s.status.FailedCount++
		s.status.LastErrorAt = time.Now()
		s.status.LastError = execErr.Error()
		s.mu.Unlock()
		return record, execErr
	}
	record.Status = "success"
	record.Message = "print completed"
	s.pushHistory(record)
	s.mu.Lock()
	s.status.SuccessCount++
	s.status.LastSuccessAt = time.Now()
	s.status.LastError = ""
	s.mu.Unlock()
	return record, nil
}

func (s *PrintAgentService) loop(ctx context.Context, cfg config.PrintAgentSettings) {
	interval := time.Duration(cfg.PollIntervalMS) * time.Millisecond
	if interval < 500*time.Millisecond {
		interval = 500 * time.Millisecond
	}
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		claimed, err := s.pollOnce(ctx, cfg)
		if err != nil {
			if isContextCanceledErr(err) {
				return
			}
			s.setError(err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(interval):
				continue
			}
		}
		if claimed {
			continue
		}
		idleSleep := interval
		if cfg.LongPollMS > 0 && idleSleep > 500*time.Millisecond {
			// Long-poll already blocks on server; avoid stacking another long idle wait.
			idleSleep = 500 * time.Millisecond
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(idleSleep):
		}
	}
}

func (s *PrintAgentService) pollOnce(ctx context.Context, cfg config.PrintAgentSettings) (bool, error) {
	s.mu.Lock()
	s.status.LastPollAt = time.Now()
	s.mu.Unlock()
	task, err := s.claimNextJob(ctx, cfg)
	if err != nil {
		return false, err
	}
	if task == nil {
		return false, nil
	}
	s.mu.Lock()
	s.status.ClaimedCount++
	s.status.CurrentJobCode = task.JobCode
	s.status.ActiveJobs++
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		if s.status.ActiveJobs > 0 {
			s.status.ActiveJobs--
		}
		if strings.TrimSpace(s.status.CurrentJobCode) == strings.TrimSpace(task.JobCode) {
			s.status.CurrentJobCode = ""
		}
		s.mu.Unlock()
	}()
	result, execErr := s.executeTaskLimited(ctx, task, cfg)
	if execErr != nil {
		_ = s.reportFailed(ctx, cfg, task, execErr.Error(), result)
		s.pushHistory(PrintAgentJobRecord{
			Time:         time.Now(),
			JobID:        task.ID,
			JobCode:      task.JobCode,
			TemplateCode: task.TemplateCode,
			PrinterName:  task.PrinterName,
			Status:       "failed",
			Message:      execErr.Error(),
			Result:       result,
		})
		s.mu.Lock()
		s.status.FailedCount++
		s.status.LastErrorAt = time.Now()
		s.status.LastError = execErr.Error()
		s.mu.Unlock()
		return true, nil
	}
	if err := s.reportSuccess(ctx, cfg, task, result); err != nil {
		s.setError(err)
	}
	s.pushHistory(PrintAgentJobRecord{
		Time:         time.Now(),
		JobID:        task.ID,
		JobCode:      task.JobCode,
		TemplateCode: task.TemplateCode,
		PrinterName:  task.PrinterName,
		Status:       "success",
		Message:      "print completed",
		Result:       result,
	})
	s.mu.Lock()
	s.status.SuccessCount++
	s.status.LastSuccessAt = time.Now()
	s.status.LastError = ""
	s.mu.Unlock()
	return true, nil
}

func (s *PrintAgentService) setError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status.LastErrorAt = time.Now()
	s.status.LastError = strings.TrimSpace(err.Error())
}

func (s *PrintAgentService) pushHistory(item PrintAgentJobRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history = append(s.history, item)
	if len(s.history) > 100 {
		s.history = s.history[len(s.history)-100:]
	}
}

type remotePrintJob struct {
	ID           int            `json:"id"`
	JobCode      string         `json:"job_code"`
	TemplateCode string         `json:"template_code"`
	PrinterName  string         `json:"printer_name"`
	Copies       int            `json:"copies"`
	Payload      map[string]any `json:"payload"`
}

func (s *PrintAgentService) claimNextJob(ctx context.Context, cfg config.PrintAgentSettings) (*remotePrintJob, error) {
	body := map[string]any{
		"client_id": cfg.ClientID,
		"job_type":  cfg.JobType,
	}
	if cfg.LongPollMS > 0 {
		body["wait_ms"] = cfg.LongPollMS
	}
	var resp struct {
		Success bool            `json:"success"`
		Message string          `json:"message"`
		Data    *remotePrintJob `json:"data"`
	}
	if err := s.doJSON(ctx, cfg, http.MethodPost, cfg.ServerURL+"/api/print-jobs/next", body, &resp); err != nil {
		return nil, err
	}
	if !resp.Success {
		return nil, fmt.Errorf(resp.Message)
	}
	return resp.Data, nil
}

func (s *PrintAgentService) reportSuccess(ctx context.Context, cfg config.PrintAgentSettings, task *remotePrintJob, result map[string]any) error {
	var resp map[string]any
	return s.doJSON(ctx, cfg, http.MethodPost, fmt.Sprintf("%s/api/print-jobs/%d/success", cfg.ServerURL, task.ID), map[string]any{
		"client_id": cfg.ClientID,
		"result":    result,
	}, &resp)
}

func (s *PrintAgentService) reportFailed(ctx context.Context, cfg config.PrintAgentSettings, task *remotePrintJob, message string, result map[string]any) error {
	var resp map[string]any
	return s.doJSON(ctx, cfg, http.MethodPost, fmt.Sprintf("%s/api/print-jobs/%d/failed", cfg.ServerURL, task.ID), map[string]any{
		"client_id":     cfg.ClientID,
		"error_message": message,
		"result":        result,
	}, &resp)
}

func (s *PrintAgentService) doJSON(ctx context.Context, cfg config.PrintAgentSettings, method, url string, payload any, out any) error {
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Print-Agent-Key", cfg.AgentAPIKey)
	resp, err := s.client.Do(req)
	if err != nil {
		msg := strings.TrimSpace(err.Error())
		if strings.Contains(strings.ToLower(msg), "eof") && !serverURLHasExplicitPort(cfg.ServerURL) {
			return fmt.Errorf("%s (hint: Quantix 服务地址可能缺少端口，请尝试 http://<host>:8050)", msg)
		}
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	if out == nil || len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, out)
}

func serverURLHasExplicitPort(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	host := strings.TrimSpace(u.Host)
	if host == "" {
		return false
	}
	if strings.Contains(host, "]") {
		return strings.Contains(host, "]:")
	}
	parts := strings.Split(host, ":")
	return len(parts) >= 2
}

func isContextCanceledErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return true
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(msg, "context canceled")
}

func (s *PrintAgentService) executeTask(task *remotePrintJob, cfg config.PrintAgentSettings) (map[string]any, error) {
	templatePath := strings.TrimSpace(cfg.TemplateMappings[task.TemplateCode])
	if templatePath == "" {
		return map[string]any{"template_code": task.TemplateCode}, fmt.Errorf("template mapping not found: %s", task.TemplateCode)
	}
	absTemplate, err := filepath.Abs(templatePath)
	if err == nil {
		templatePath = absTemplate
	}
	if _, err := os.Stat(templatePath); err != nil {
		return map[string]any{"template_path": templatePath}, fmt.Errorf("template not found: %s", templatePath)
	}
	exePath, err := resolveBarTenderExecutable(cfg.BartenderExecutable)
	if err != nil {
		return map[string]any{"template_path": templatePath}, err
	}
	copies := task.Copies
	if copies <= 0 {
		copies = 1
	}
	printerName := strings.TrimSpace(task.PrinterName)
	if printerName == "" {
		printerName = cfg.DefaultPrinterName
	}
	xmlContent, err := buildBTXML(templatePath, printerName, copies, task.JobCode, task.Payload)
	if err != nil {
		return map[string]any{"template_path": templatePath}, err
	}
	tempFile, err := os.CreateTemp("", "quantix-bartender-*.btxml")
	if err != nil {
		return map[string]any{"template_path": templatePath}, err
	}
	tempPath := tempFile.Name()
	if _, err := tempFile.WriteString(xmlContent); err != nil {
		_ = tempFile.Close()
		return map[string]any{"template_path": templatePath}, err
	}
	_ = tempFile.Close()
	defer os.Remove(tempPath)

	cmd := exec.Command(exePath, "/XMLScript="+tempPath, "/X")
	output, err := cmd.CombinedOutput()
	result := map[string]any{
		"template_path": templatePath,
		"printer_name":  printerName,
		"copies":        copies,
		"btxml_file":    tempPath,
		"bartender_exe": exePath,
		"output":        strings.TrimSpace(string(output)),
	}
	if err != nil {
		result["exec_error"] = err.Error()
		return result, fmt.Errorf("bartender execute failed: %w", err)
	}
	return result, nil
}

func (s *PrintAgentService) executeTaskLimited(ctx context.Context, task *remotePrintJob, cfg config.PrintAgentSettings) (map[string]any, error) {
	sem := s.currentExecSem(cfg)
	select {
	case sem <- struct{}{}:
		defer func() { <-sem }()
	case <-ctx.Done():
		return map[string]any{"job_code": task.JobCode}, ctx.Err()
	}
	return s.executeTask(task, cfg)
}

func (s *PrintAgentService) currentExecSem(cfg config.PrintAgentSettings) chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.execSem == nil {
		limit := cfg.MaxConcurrentJobs
		if limit <= 0 {
			limit = 1
		}
		s.execSem = make(chan struct{}, limit)
	}
	return s.execSem
}

func normalizeAgentCfg(cfg config.PrintAgentSettings) config.PrintAgentSettings {
	return config.PrintAgentSettings{
		Enabled:             cfg.Enabled,
		ServerURL:           strings.TrimRight(strings.TrimSpace(cfg.ServerURL), "/"),
		AgentAPIKey:         strings.TrimSpace(cfg.AgentAPIKey),
		ClientID:            strings.TrimSpace(cfg.ClientID),
		JobType:             strings.TrimSpace(cfg.JobType),
		DefaultPrinterName:  strings.TrimSpace(cfg.DefaultPrinterName),
		BartenderExecutable: strings.TrimSpace(cfg.BartenderExecutable),
		PollIntervalMS:      cfg.PollIntervalMS,
		LongPollMS:          cfg.LongPollMS,
		MaxConcurrentJobs:   cfg.MaxConcurrentJobs,
		TemplateMappings:    cloneStringMap(cfg.TemplateMappings),
	}
}

func clonePrintAgentConfig(cfg config.PrintAgentSettings) config.PrintAgentSettings {
	return normalizeAgentCfg(cfg)
}

func cloneStringMap(in map[string]string) map[string]string {
	if in == nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func resolveBarTenderExecutable(explicit string) (string, error) {
	candidates := []string{}
	if strings.TrimSpace(explicit) != "" {
		candidates = append(candidates, strings.TrimSpace(explicit))
	}
	candidates = append(candidates, defaultBarTenderExecutableCandidates...)
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("BarTender executable not found")
}

func buildBTXML(templatePath, printerName string, copies int, jobCode string, payload map[string]any) (string, error) {
	var b strings.Builder
	b.WriteString("<?xml version=\"1.0\" encoding=\"utf-8\"?>\n")
	b.WriteString("<XMLScript Version=\"2.0\">\n")
	b.WriteString("  <Command Name=\"")
	b.WriteString(xmlEscape(jobCode))
	b.WriteString("\">\n")
	b.WriteString("    <Print>\n")
	b.WriteString("      <Format>")
	b.WriteString(xmlEscape(templatePath))
	b.WriteString("</Format>\n")
	b.WriteString("      <PrintSetup>\n")
	if strings.TrimSpace(printerName) != "" {
		b.WriteString("        <Printer>")
		b.WriteString(xmlEscape(printerName))
		b.WriteString("</Printer>\n")
	}
	if copies <= 0 {
		copies = 1
	}
	b.WriteString(fmt.Sprintf("        <IdenticalCopiesOfLabel>%d</IdenticalCopiesOfLabel>\n", copies))
	b.WriteString("      </PrintSetup>\n")
	for key, value := range payload {
		b.WriteString("      <NamedSubString Name=\"")
		b.WriteString(xmlEscape(strings.TrimSpace(key)))
		b.WriteString("\">\n")
		b.WriteString("        <Value>")
		b.WriteString(xmlEscape(fmt.Sprintf("%v", value)))
		b.WriteString("</Value>\n")
		b.WriteString("      </NamedSubString>\n")
	}
	b.WriteString("    </Print>\n")
	b.WriteString("  </Command>\n")
	b.WriteString("</XMLScript>\n")
	return b.String(), nil
}

func xmlEscape(v string) string {
	var buf bytes.Buffer
	_ = xml.EscapeText(&buf, []byte(v))
	return buf.String()
}
