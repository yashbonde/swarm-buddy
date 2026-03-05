// MIT License - github.com/yashbonde
// Single file shephard protocol implementation

package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"log/slog"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"
)

// ─── Config types ─────────────────────────────────────────────────────────────

type Config struct {
	Server ServerConfig
	Agents map[string]AgentConfig
}

type ServerConfig struct {
	Port     int    `json:"port"`
	Limit    string `json:"limit"`
	Log      string `json:"log"`
	DB       string `json:"db"`
	Opencode string `json:"opencode"`

	// Prompt
	ShephardSkill string `json:"shephard_skill"`
	ShephardAgent string `json:"shephard_agent"`
	GeneralAgent  string `json:"general_agent"`
}

type AgentConfig struct {
	Prompt     string          `json:"prompt"`
	Folder     string          `json:"folder"`
	Schedule   *ScheduleConfig `json:"schedule"`    // New schedule system
	OutputType string          `json:"output_type"` // optional output type
}

type ScheduleConfig struct {
	Every     string `json:"every"`     // day/week/month
	Weekdays  string `json:"weekdays"`  // mon,tue...
	Monthdays string `json:"monthdays"` // 1,15,28...
	Time      string `json:"time"`      // 12:00
}

// ─── Job / Metrics types ──────────────────────────────────────────────────────

type JobState struct {
	ID         string
	Agent      string // the agent name (key in cfg.Agents)
	Slug       string // the webhook sub-path (everything after /app/{agent}/)
	Status     string // pending|running|done|failed|timedout|interrupted
	Prompt     string
	Output     []string
	PGID       int
	KillAt     time.Time
	CreatedAt  time.Time
	StartedAt  time.Time
	EndedAt    time.Time
	OutputFile string // this is the path to the output generated file
}

func jobDone(job *JobState) bool {
	return job.Status == "done" || job.Status == "failed" || job.Status == "timedout" || job.Status == "interrupted"
}

type MetricSummary struct {
	JobID       string
	SampleCount int
	CPUMean     float64
	CPUStddev   float64
	CPUMax      float64
	CPUCurrent  float64
	MemMean     float64
	MemMax      uint64
	MemCurrent  uint64
	LastSampled time.Time

	// Welford online variance state (not serialized as metrics fields)
	cpuM2  float64
	memSum float64
}

// ─── Globals ──────────────────────────────────────────────────────────────────

type taskMeta struct {
	Slug     string
	Schedule ScheduleConfig
	LastRun  string // format: 2006-01-02-15-04
}

var (
	cfg    Config
	logger *slog.Logger

	registry   = map[string]*JobState{}
	registryMu sync.RWMutex

	metrics   = map[string]*MetricSummary{}
	metricsMu sync.Mutex

	startTime  = time.Now()
	taskMetas  []taskMeta
	configPath string
	serverRoot string

	ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*[mKHJABCDsu]`)
)

// ─── main ─────────────────────────────────────────────────────────────────────

func main() {
	cfgPath := flag.String("cfg", "./swarm_cfg.json", "path to json config file")
	flag.Parse()

	configPath = *cfgPath
	if wd, err := os.Getwd(); err == nil {
		serverRoot = wd
	}

	// Load config into the global cfg
	var err error
	f, err := os.Open(configPath)
	if err != nil {
		log.Fatalf("loadConfig open: %v", err)
	}
	defer f.Close()
	if err := json.NewDecoder(f).Decode(&cfg); err != nil {
		log.Fatalf("loadConfig decode: %v", err)
	}
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 7070
	}
	if cfg.Server.Log == "" {
		cfg.Server.Log = "./swarm_buddy.log"
	}
	if cfg.Server.DB == "" {
		cfg.Server.DB = "./swarm_buddy.db"
	}
	if cfg.Server.Opencode == "" {
		cfg.Server.Opencode = "opencode"
	}
	if cfg.Server.Limit == "" {
		cfg.Server.Limit = "120m"
	}
	// If any prompt field is a path to a .md file, read it from disk.
	loadPromptFile := func(val string) string {
		if strings.HasSuffix(val, ".md") {
			data, err := os.ReadFile(val)
			if err != nil {
				log.Fatalf("loadConfig: reading prompt file %q: %v", val, err)
			}
			return string(data)
		}
		return val
	}
	cfg.Server.ShephardSkill = loadPromptFile(cfg.Server.ShephardSkill)
	cfg.Server.ShephardAgent = loadPromptFile(cfg.Server.ShephardAgent)
	cfg.Server.GeneralAgent = loadPromptFile(cfg.Server.GeneralAgent)

	if _, err := time.ParseDuration(cfg.Server.Limit); err != nil {
		log.Fatalf("loadConfig: invalid server.limit %q: %v", cfg.Server.Limit, err)
	}
	if len(cfg.Agents) == 0 {
		log.Fatal("loadConfig: at least one agent must be defined")
	}
	for agentName, ag := range cfg.Agents {
		if ag.Prompt == "" {
			log.Fatalf("loadConfig: agent %q needs prompt", agentName)
		}
		if ag.Folder == "" {
			ag.Folder = serverRoot + "/agents/" + agentName
		}
		cfg.Agents[agentName] = ag // write the (possibly-modified) copy back into the map
	}

	// Initialise logger
	f, err = os.OpenFile(cfg.Server.Log, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("initLogger: %v", err)
	}
	mw := io.MultiWriter(os.Stdout, f)
	logger = slog.New(slog.NewTextHandler(mw, &slog.HandlerOptions{Level: slog.LevelInfo}))

	// Define DB
	// We use a directory to store JSON files for each key.
	// We use the path specified in the config, but treat it as a directory.
	dbDir := cfg.Server.DB
	if dbDir == "" {
		dbDir = ".shephard/swb_db"
	}
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		log.Fatalf("initDB: failed to create data directory: %v", err)
	}
	cfg.Server.DB = dbDir

	// merge the things from DB and config into the registry
	bootFreshRegistry()

	// start scheduler
	for agentName, ag := range cfg.Agents {
		if ag.Schedule != nil {
			taskMetas = append(taskMetas, taskMeta{
				Slug:     agentName,
				Schedule: *ag.Schedule,
			})
		}
	}

	go startScheduler()
	go startMetricsSampler()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /app/", handleSpawn)
	mux.HandleFunc("POST /app/{slug}/", handleSpawn)
	mux.HandleFunc("GET /status/{id}", handleJobStatus)
	mux.HandleFunc("POST /shephard", handleShephard)
	mux.HandleFunc("POST /kill", handleKill)
	mux.HandleFunc("GET /crons", handleCrons)

	// This route loads any AI with enough information to use the APIs
	mux.HandleFunc("GET /about", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fmt.Sprintf(cfg.Server.ShephardSkill, cfg.Server.Port)))
	})

	// Checklist of things
	logger.Info("Total Cron Tasks:", "count", len(taskMetas))
	logger.Info("Agents available:", "count", len(cfg.Agents))
	for agentName, ag := range cfg.Agents {
		logger.Info("Agent", "Name", agentName, "folder", ag.Folder)
	}

	// Start server
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	logger.Info("HTTP server starting", "addr", addr)
	if err := http.ListenAndServe(addr, loggingMiddleware(mux)); err != nil {
		logger.Error("HTTP server error", "err", err)
	}

}

// ─── DB ───────────────────────────────────────────────────────────────────────

func kvSet(key string, val any) {
	b, err := json.Marshal(val)
	if err != nil {
		logger.Error("kvSet marshal", "key", key, "err", err)
		return
	}

	// Sanitize key for filesystem (replace : with _)
	safeKey := strings.ReplaceAll(key, ":", "_")
	path := filepath.Join(cfg.Server.DB, safeKey+".json")

	// Atomic write
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, b, 0644); err != nil {
		logger.Error("kvSet write", "key", key, "err", err)
		return
	}
	if err := os.Rename(tmpPath, path); err != nil {
		logger.Error("kvSet rename", "key", key, "err", err)
	}
}

func kvGet(key string, out any) error {
	safeKey := strings.ReplaceAll(key, ":", "_")
	path := filepath.Join(cfg.Server.DB, safeKey+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}

// ─── ID ───────────────────────────────────────────────────────────────────────

func newID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return time.Now().Format("20060102150405") + "_" + hex.EncodeToString(b)
}

// ─── Job lifecycle ────────────────────────────────────────────────────────────

func streamOutput(job *JobState, pr io.ReadCloser) {
	defer pr.Close()
	buf := make([]byte, 4096)
	var lineBuf strings.Builder
	for {
		n, readErr := pr.Read(buf)
		if n > 0 {
			lineBuf.Write(buf[:n])
			for {
				s := lineBuf.String()
				idx := strings.IndexByte(s, '\n')
				if idx < 0 {
					break
				}
				line := s[:idx]
				lineBuf.Reset()
				lineBuf.WriteString(s[idx+1:])

				filteredLine := ansiRe.ReplaceAllString(line, "")
				filteredLine = strings.TrimSpace(filteredLine)

				if filtered := filteredLine; filtered != "" {
					registryMu.Lock()
					job.Output = append(job.Output, filtered)
					registryMu.Unlock()
				}
			}
		}
		if readErr != nil {
			break
		}
	}
}

func opencodeWorker(
	job *JobState,
	folder string,
	persistPrompt bool,
	taskPrefix string,
	forceDir bool,
	attach_files []string,
) {
	// What does CLI worker do?
	//
	// First we store the generated prompt in the folder MASTER_FOLDER/.prompts/task-{id}.md
	// This file is then fed into the opencode CLI worker. All the generated logs from the stderr
	// stdout is read and parsed to filter relevant information.

	defer func() {
		if r := recover(); r != nil {
			logger.Error("launchJob panic", "id", job.ID, "r", r)
		}
	}()

	// pre-req, create .tmp folder
	if err := os.MkdirAll(filepath.Join(folder, ".tmp"), 0755); err != nil {
		logger.Error("MkdirAll", "id", job.ID, "err", err)
		registryMu.Lock()
		job.Status = "failed"
		job.EndedAt = time.Now()
		registryMu.Unlock()
		kvSet("job:"+job.ID, job)
		return
	}

	var tmpName string
	if persistPrompt {
		promptsDir := filepath.Join(folder, ".prompts")
		if err := os.MkdirAll(promptsDir, 0755); err != nil {
			logger.Error("MkdirAll", "id", job.ID, "err", err)
			registryMu.Lock()
			job.Status = "failed"
			job.EndedAt = time.Now()
			registryMu.Unlock()
			kvSet("job:"+job.ID, job)
			return
		}

		tmpName = filepath.Join(promptsDir, fmt.Sprintf("%s-%s.md", taskPrefix, job.ID))
		if err := os.WriteFile(tmpName, []byte(job.Prompt), 0644); err != nil {
			logger.Error("WriteFile", "id", job.ID, "err", err)
			registryMu.Lock()
			job.Status = "failed"
			job.EndedAt = time.Now()
			registryMu.Unlock()
			kvSet("job:"+job.ID, job)
			return
		}
	} else {
		tmpName = job.Prompt
	}

	var cmd *exec.Cmd
	commands := []string{
		cfg.Server.Opencode, "run", tmpName,
		"--title", fmt.Sprintf("swarm-shephard-%s", job.ID),
		"--model", "google/gemini-3-flash-preview",
		"--variant", "low",
	}
	if forceDir {
		commands = append(commands, "--dir", folder)
	}
	if len(attach_files) > 0 {
		commands = append(commands, "--file")
		for _, f := range attach_files {
			commands = append(commands, f)
		}
	}
	logger.Info("opencodeWorker", "commands", commands)
	if !job.KillAt.IsZero() {
		ctx, cancel := context.WithDeadline(context.Background(), job.KillAt)
		defer cancel()
		cmd = exec.CommandContext(ctx, commands[0], commands[1:]...)
	} else {
		cmd = exec.Command(commands[0], commands[1:]...)
	}
	cmd.Dir = folder
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	pr, pw, err := os.Pipe()
	if err != nil {
		logger.Error("os.Pipe", "id", job.ID, "err", err)
		registryMu.Lock()
		job.Status = "failed"
		job.EndedAt = time.Now()
		registryMu.Unlock()
		kvSet("job:"+job.ID, job)
		return
	}
	cmd.Stdout = pw
	cmd.Stderr = pw

	if err := cmd.Start(); err != nil {
		pw.Close()
		pr.Close()
		logger.Error("cmd.Start", "id", job.ID, "err", err)
		registryMu.Lock()
		job.Status = "failed"
		job.EndedAt = time.Now()
		registryMu.Unlock()
		kvSet("job:"+job.ID, job)
		return
	}
	pw.Close()

	registryMu.Lock()
	job.Status = "running"
	job.StartedAt = time.Now()
	job.PGID = cmd.Process.Pid
	registryMu.Unlock()
	kvSet("job:"+job.ID, job)

	streamOutput(job, pr)

	err = cmd.Wait()
	registryMu.Lock()
	job.EndedAt = time.Now()
	if err != nil {
		if strings.Contains(err.Error(), "signal: killed") || strings.Contains(err.Error(), "context deadline exceeded") {
			job.Status = "timedout"
		} else {
			job.Status = "failed"
		}
	} else {
		job.Status = "done"
	}
	registryMu.Unlock()
	kvSet("job:"+job.ID, job)
}

func bashWorker(job *JobState, folder string, command string) {
	defer func() {
		if r := recover(); r != nil {
			logger.Error("bashWorker panic", "id", job.ID, "r", r)
		}
	}()

	ctx, cancel := context.WithDeadline(context.Background(), job.KillAt)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	cmd.Dir = folder
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	pr, pw, err := os.Pipe()
	if err != nil {
		logger.Error("os.Pipe", "id", job.ID, "err", err)
		registryMu.Lock()
		job.Status = "failed"
		job.EndedAt = time.Now()
		registryMu.Unlock()
		kvSet("job:"+job.ID, job)
		return
	}
	cmd.Stdout = pw
	cmd.Stderr = pw

	if err := cmd.Start(); err != nil {
		pw.Close()
		pr.Close()
		logger.Error("cmd.Start", "id", job.ID, "err", err)
		registryMu.Lock()
		job.Status = "failed"
		job.EndedAt = time.Now()
		registryMu.Unlock()
		kvSet("job:"+job.ID, job)
		return
	}
	pw.Close()

	registryMu.Lock()
	job.Status = "running"
	job.StartedAt = time.Now()
	job.PGID = cmd.Process.Pid
	registryMu.Unlock()
	kvSet("job:"+job.ID, job)

	streamOutput(job, pr)

	err = cmd.Wait()
	registryMu.Lock()
	job.EndedAt = time.Now()
	if err != nil {
		if strings.Contains(err.Error(), "signal: killed") || strings.Contains(err.Error(), "context deadline exceeded") {
			job.Status = "timedout"
		} else {
			job.Status = "failed"
		}
	} else {
		job.Status = "done"
	}
	registryMu.Unlock()
	kvSet("job:"+job.ID, job)
}

func spawnJob(
	agentName string,
	slug string,
	ag AgentConfig,
	queryParams map[string]string,
	body map[string]any,
	id string,
	outputType string,
) string {
	if ag.Folder == "" {
		ag.Folder = filepath.Join(serverRoot, "agents", slug)
	}
	limit, _ := time.ParseDuration(cfg.Server.Limit)
	outType := outputType
	if outType == "" {
		outType = "ignore"
	}
	outputFilePath := ag.Folder + "/.tmp/OUTPUT-" + id + ".json"
	job := &JobState{
		ID:    id,
		Agent: agentName,
		Slug:  slug,
		Prompt: fmt.Sprintf(cfg.Server.GeneralAgent,
			time.Now().Format("2006-01-02-15-04"),
			ag.Folder,
			ag.Prompt,
			slug,
			queryParams,
			body,
			outputFilePath,
		),
		Status:     "pending",
		CreatedAt:  time.Now(),
		KillAt:     time.Now().Add(limit),
		OutputFile: outputFilePath,
	}
	registryMu.Lock()
	registry[job.ID] = job
	registryMu.Unlock()
	kvSet("job:"+job.ID, job)

	logger.Info("spawnJob", "JobID", job.ID, "Slug", slug)

	go opencodeWorker(job, ag.Folder, true, "tasks", true, []string{})
	return id
}

func killJob(id string) {
	registryMu.Lock()
	job, ok := registry[id]
	registryMu.Unlock()
	if !ok {
		return
	}
	if job.PGID > 0 {
		_ = syscall.Kill(-job.PGID, syscall.SIGKILL)
	}
	registryMu.Lock()
	job.Status = "interrupted"
	job.EndedAt = time.Now()
	registryMu.Unlock()
	kvSet("job:"+id, job)
}

func bootFreshRegistry() {
	// Scan the data directory for all job files
	files, err := filepath.Glob(filepath.Join(cfg.Server.DB, "job_*.json"))
	if err != nil {
		logger.Error("bootCleanup glob", "err", err)
		return
	}

	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var job JobState
		if err := json.Unmarshal(data, &job); err != nil {
			continue
		}

		if job.Status == "running" {
			if job.PGID > 0 {
				_ = syscall.Kill(-job.PGID, syscall.SIGKILL)
			}
			if !job.KillAt.IsZero() && time.Now().After(job.KillAt) {
				job.Status = "timedout"
			} else {
				job.Status = "interrupted"
			}
			job.EndedAt = time.Now()
			kvSet("job:"+job.ID, job)
		}
		jCopy := job
		registryMu.Lock()
		registry[job.ID] = &jCopy
		registryMu.Unlock()
	}
}

// ─── Cron ─────────────────────────────────────────────────────────────────────

func startScheduler() {
	ticker := time.NewTicker(1 * time.Minute)
	for {
		now := time.Now()
		minuteKey := now.Format("2006-01-02-15-04")

		for i := range taskMetas {
			meta := &taskMetas[i]
			if meta.LastRun == minuteKey {
				continue
			}

			if shouldRunNow(meta.Schedule, now) {
				ag := cfg.Agents[meta.Slug]
				id := spawnJob(meta.Slug, "", ag, nil, nil, newID(), ag.OutputType)
				logger.Info("scheduled job spawned", "slug", meta.Slug, "id", id)
				meta.LastRun = minuteKey
			}
		}
		<-ticker.C
	}
}

func shouldRunNow(s ScheduleConfig, now time.Time) bool {
	// 1. Check time (HH:MM)
	if now.Format("15:04") != s.Time {
		return false
	}

	// 2. Check frequency
	switch s.Every {
	case "day":
		return true
	case "week":
		// Weekdays: "mon,sat"
		currentDay := strings.ToLower(now.Weekday().String()[:3]) // Mon, Tue... -> mon, tue...
		return strings.Contains(strings.ToLower(s.Weekdays), currentDay)
	case "month":
		// Monthdays: "1,15,28"
		currentDayStr := strconv.Itoa(now.Day())
		days := strings.Split(s.Monthdays, ",")
		for _, d := range days {
			if strings.TrimSpace(d) == currentDayStr {
				return true
			}
		}
	}
	return false
}

func handleCrons(w http.ResponseWriter, r *http.Request) {
	// Return list of tasks.
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"schedules": taskMetas})
}

// ─── Metrics ──────────────────────────────────────────────────────────────────

func startMetricsSampler() {
	defer func() {
		if r := recover(); r != nil {
			logger.Error("metrics sampler panic", "r", r)
		}
	}()
	sampleTick := time.NewTicker(10 * time.Second)
	flushTick := time.NewTicker(1 * time.Minute)
	for {
		select {
		case <-sampleTick.C:
			sampleProcesses()
		case <-flushTick.C:
			flushMetrics()
		}
	}
}

func sampleProcesses() {
	registryMu.RLock()
	type snap struct {
		id   string
		pgid int
	}
	var snaps []snap
	for id, j := range registry {
		if j.Status == "running" && j.PGID > 0 {
			snaps = append(snaps, snap{id, j.PGID})
		}
	}
	registryMu.RUnlock()

	for _, s := range snaps {
		// Get metrics using 'ps'. Columns: %cpu, rss (in KB)
		// We use -p <pid> to target the specific process.
		out, err := exec.Command("ps", "-p", strconv.Itoa(s.pgid), "-o", "%cpu,rss").Output()
		if err != nil {
			continue
		}

		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		if len(lines) < 2 {
			continue
		}

		// Header: %CPU   RSS
		// Data:   0.0   1234
		fields := strings.Fields(lines[1])
		if len(fields) < 2 {
			continue
		}

		cpu, _ := strconv.ParseFloat(fields[0], 64)
		rssKB, _ := strconv.ParseUint(fields[1], 10, 64)
		rssBytes := rssKB * 1024

		metricsMu.Lock()
		ms, ok := metrics[s.id]
		if !ok {
			ms = &MetricSummary{JobID: s.id}
			metrics[s.id] = ms
		}
		updateSummary(ms, cpu, rssBytes)
		metricsMu.Unlock()
	}
}

func updateSummary(ms *MetricSummary, cpu float64, memRSS uint64) {
	ms.SampleCount++
	n := float64(ms.SampleCount)

	// Welford online mean/variance for CPU
	delta := cpu - ms.CPUMean
	ms.CPUMean += delta / n
	delta2 := cpu - ms.CPUMean
	ms.cpuM2 += delta * delta2
	if n > 1 {
		ms.CPUStddev = math.Sqrt(ms.cpuM2 / (n - 1))
	}
	if cpu > ms.CPUMax {
		ms.CPUMax = cpu
	}
	ms.CPUCurrent = cpu

	// Simple mean for mem
	ms.memSum += float64(memRSS)
	ms.MemMean = ms.memSum / n
	if memRSS > ms.MemMax {
		ms.MemMax = memRSS
	}
	ms.MemCurrent = memRSS
	ms.LastSampled = time.Now()
}

func flushMetrics() {
	metricsMu.Lock()
	defer metricsMu.Unlock()
	for id, ms := range metrics {
		kvSet("metrics:"+id, ms)
	}
}

// ─── HTTP ─────────────────────────────────────────────────────────────────────

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		logger.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"latency", time.Since(start).String(),
			"ip", r.RemoteAddr,
		)
	})
}

const IMAGES_SPECIAL_KEY = "x-swarm-images-key"

func handleSpawn(w http.ResponseWriter, r *http.Request) {
	// /app/{slug}/any/sub/path — split off the agent name
	parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/app/"), "/", 2)
	agentName := parts[0]
	if agentName == "swarm_buddy" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "swarm_buddy is a reserved slug"})
		return
	}
	logger.Info("spawn", "agent", agentName)
	logger.Info("spawn", "agents", cfg.Agents)
	ag, ok := cfg.Agents[agentName]
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "unknown agent"})
		return
	}

	// check special protocol inputs
	imagesKey := ""
	for k, v := range r.Header {
		if strings.EqualFold(k, IMAGES_SPECIAL_KEY) {
			imagesKey = v[0]
		}
	}

	queryParams := make(map[string]string)
	for k, v := range r.URL.Query() {
		if len(v) > 0 {
			queryParams[k] = v[0]
		}
	}

	var body map[string]any
	data, err := io.ReadAll(io.LimitReader(r.Body, 10*1024*1024))
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "error reading body"})
		return
	}

	if len(data) > 0 {
		if !utf8.Valid(data) || bytes.IndexByte(data, 0) != -1 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "unsupported binary content (must be plaintext/JSON)"})
			return
		}

		ct := r.Header.Get("Content-Type")
		if strings.Contains(ct, "application/json") || bytes.HasPrefix(bytes.TrimSpace(data), []byte("{")) {
			if err := json.Unmarshal(data, &body); err != nil {
				if strings.Contains(ct, "application/json") {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusBadRequest)
					json.NewEncoder(w).Encode(map[string]string{"error": "bad JSON body"})
					return
				}
				body = map[string]any{"content": string(data)}
			}
		} else {
			body = map[string]any{"content": string(data)}
		}
	}

	id := newID()
	// pop the images b64 from the body and write it to a png file
	if imagesKey != "" {
		if imgB64s, ok := body[imagesKey]; ok {
			folder := ag.Folder
			if folder == "" {
				folder = filepath.Join(serverRoot, "agents", agentName)
			}
			os.MkdirAll(folder, 0755)

			var list []string
			if sl, ok := imgB64s.([]any); ok {
				for _, v := range sl {
					if s, ok := v.(string); ok {
						list = append(list, s)
					}
				}
			} else if sl, ok := imgB64s.([]string); ok {
				list = sl
			}

			for i, imgB64 := range list {
				raw := imgB64
				if idx := strings.Index(imgB64, ","); idx != -1 {
					raw = imgB64[idx+1:]
				}
				data, err := base64.StdEncoding.DecodeString(raw)
				if err != nil {
					logger.Error("failed to decode image", "id", id, "index", i, "err", err)
					continue
				}
				filename := fmt.Sprintf("%s_img_%d.png", id, i)
				path := filepath.Join(folder, filename)
				if err := os.WriteFile(path, data, 0644); err != nil {
					logger.Error("failed to write image", "id", id, "path", path, "err", err)
				}
			}

			// delete the images from the body
			delete(body, imagesKey)
		}
	}

	spawnJob(agentName, strings.Join(parts[1:], "/"), ag, queryParams, body, id, ag.OutputType)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{
		"id":        id,
		"next_step": "check status at /status/" + id + ". Be kind.",
	})
}

func handleJobStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	registryMu.RLock()
	job, ok := registry[id]
	if ok {
		jcopy := *job
		registryMu.RUnlock()
		metricsMu.Lock()
		ms := metrics[id]
		metricsMu.Unlock()
		w.Header().Set("Content-Type", "application/json")

		// when output type is given, read the fixed output file written by the agent
		ag := cfg.Agents[jcopy.Agent]
		stdout := ""
		logger.Info("handleJobStatus", "ag.OutputType", ag.OutputType)
		if jcopy.OutputFile != "" {
			data, err := os.ReadFile(jcopy.OutputFile)
			if err == nil {
				stdout = string(data)
			}
		} else {
			stdout = strings.Join(jcopy.Output, "\n")
		}
		json.NewEncoder(w).Encode(map[string]any{
			"job":     jcopy,
			"metrics": ms,
			"stdout":  stdout,
			"done":    jobDone(&jcopy),
		})
		return
	}
	registryMu.RUnlock()

	var stored JobState
	if err := kvGet("job:"+id, &stored); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
		return
	}
	var ms MetricSummary
	_ = kvGet("metrics:"+id, &ms)
	w.Header().Set("Content-Type", "application/json")

	// when output type is given, read the fixed output file written by the agent
	stdout := ""
	if stored.OutputFile != "" {
		data, err := os.ReadFile(stored.OutputFile)
		if err == nil {
			stdout = string(data)
		}
	} else {
		stdout = strings.Join(stored.Output, "\n")
	}
	json.NewEncoder(w).Encode(map[string]any{
		"job":     stored,
		"metrics": ms,
		"stdout":  stdout,
		"done":    jobDone(&stored),
	})
}

func handleKill(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "bad request, should be like {\"id\": \"<id>\"}"})
		return
	}
	if req.ID == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "id required"})
		return
	}
	killJob(req.ID)
	w.WriteHeader(http.StatusNoContent)
}

// ─── Shephard ─────────────────────────────────────────────────────────────────

func handleShephard(w http.ResponseWriter, r *http.Request) {
	var message struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&message); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "bad request, should be like {\"message\": \"<message>\"}"})
		return
	}
	shephardTasksDir := filepath.Join(".", ".shephard")
	if err := os.MkdirAll(shephardTasksDir, 0755); err != nil {
		logger.Error("MkdirAll", "err", err)
		return
	}
	prompt := strings.TrimSpace(message.Message)
	if prompt == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "message body required"})
		return
	}

	id := newID()
	limit, _ := time.ParseDuration(cfg.Server.Limit)
	job := &JobState{
		ID:        id,
		Slug:      "shephard",
		Status:    "pending",
		CreatedAt: time.Now(),
		KillAt:    time.Now().Add(limit),
	}
	logger.Info("message", "prompt", prompt)

	// this message contains a shell command just run that
	if prompt[0] == '!' {
		job.Prompt = prompt
		registryMu.Lock()
		registry[job.ID] = job
		registryMu.Unlock()
		kvSet("job:"+job.ID, job)

		logger.Info("spawnJob", "JobID (bash)", job.ID)

		go bashWorker(job, shephardTasksDir, prompt[1:])

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]string{"id": id})
	} else {
		job.Prompt = fmt.Sprintf(cfg.Server.ShephardAgent,
			fmt.Sprintf(cfg.Server.ShephardSkill, cfg.Server.Port),
			prompt,
		)
		registryMu.Lock()
		registry[job.ID] = job
		registryMu.Unlock()
		kvSet("job:"+job.ID, job)

		logger.Info("spawnJob", "JobID (opencode)", job.ID)

		go opencodeWorker(job, shephardTasksDir, true, "shephard", false, []string{})

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]string{"id": id})
	}
}
