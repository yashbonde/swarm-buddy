// MIT License - github.com/yashbonde
// swb

package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"sync"

	"swarm_buddy/pkg/toroid"
)

// ─── Config types ─────────────────────────────────────────────────────────────

type Config struct {
	Server ServerConfig           `json:"server"`
	Agents map[string]AgentConfig `json:"agents"`
}

type ServerConfig struct {
	Port        int    `json:"port"`
	Limit       string `json:"limit"`
	Log         string `json:"log"`
	DB          string `json:"db"`
	GeminiModel string `json:"gemini_model"`
	GeminiToken string `json:"gemini_token"`

	// Prompt
	ShephardSkill string `json:"shephard_skill"`
	ShephardAgent string `json:"shephard_agent"`
	GeneralAgent  string `json:"general_agent"`
}

type AgentConfig struct {
	Prompt     string          `json:"prompt"`
	Folder     string          `json:"folder"`
	Schedule   *ScheduleConfig `json:"schedule"`
	OutputType string          `json:"output_type"`
}

type ScheduleConfig struct {
	Every     string `json:"every"`
	Weekdays  string `json:"weekdays"`
	Monthdays string `json:"monthdays"`
	Time      string `json:"time"`
}

// ─── Job / Metrics types ──────────────────────────────────────────────────────

type JobState struct {
	ID         string    `json:"id"`
	Agent      string    `json:"agent"`
	Slug       string    `json:"slug"`
	Status     string    `json:"status"`
	Prompt     string    `json:"prompt"`
	Output     []string  `json:"output"`
	PGID       int       `json:"pgid"`
	KillAt     time.Time `json:"kill_at"`
	CreatedAt  time.Time `json:"created_at"`
	StartedAt  time.Time `json:"started_at"`
	EndedAt    time.Time `json:"ended_at"`
	OutputFile string    `json:"output_file"`
}

func jobDone(job *JobState) bool {
	return job.Status == "done" || job.Status == "failed" || job.Status == "timedout" || job.Status == "interrupted"
}

type MetricSummary struct {
	JobID       string    `json:"job_id"`
	SampleCount int       `json:"sample_count"`
	CPUMean     float64   `json:"cpu_mean"`
	CPUStddev   float64   `json:"cpu_stddev"`
	CPUMax      float64   `json:"cpu_max"`
	CPUCurrent  float64   `json:"cpu_current"`
	MemMean     float64   `json:"mem_mean"`
	MemMax      uint64    `json:"mem_max"`
	MemCurrent  uint64    `json:"mem_current"`
	LastSampled time.Time `json:"last_sampled"`

	cpuM2  float64
	memSum float64
}

type taskMeta struct {
	Slug     string
	Schedule ScheduleConfig
	LastRun  string
}

// ─── Global State ─────────────────────────────────────────────────────────────

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

	reBold       = regexp.MustCompile(`\*\*(.+?)\*\*|__(.+?)__`)
	reItalic     = regexp.MustCompile(`\*(.+?)\*|_(.+?)_`)
	reInlineCode = regexp.MustCompile("`(.+?)`")
)

const (
	ansiReset     = "\033[0m"
	ansiBold      = "\033[1m"
	ansiDim       = "\033[2m"
	ansiItalic    = "\033[3m"
	ansiUnderline = "\033[4m"
	ansiGreen     = "\033[32m"
	ansiYellow    = "\033[33m"
	ansiBlue      = "\033[34m"
	ansiCyan      = "\033[36m"
	ansiRed       = "\033[31m"
	ansiBgGrey    = "\033[48;5;242m" // grey background
)

func usage() {
	fmt.Fprintln(os.Stderr, `Usage: swb <command> [flags]

Commands:
  init      Initialize default config and agents
  server    Start the intelligence orchestrator
  run       Run a one-off prompt or start a REPL
  sessions  List or delete sessions
  swarm     Execute multi-agent orchestration tasks

Use "swb <command> -h" for more information about a command.`)
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "init":
		cmdInit()
	case "server":
		cmdServer(os.Args[2:])
	case "run":
		cmdRunSpec(os.Args[2:])
	case "sessions":
		cmdSessions(os.Args[2:])
	default:
		usage()
		os.Exit(1)
	}
}

// ─── Subcommands ──────────────────────────────────────────────────────────────

func cmdInit() {
	fmt.Println("Initializing Swarm Buddy environment...")
	os.MkdirAll("./.swb/swb_db", 0755)
	os.MkdirAll("./.swb/agents/shephard", 0755)

	// Download agent files
	baseUrl := "https://raw.githubusercontent.com/yashbonde/swarm-buddy/master/agents/shephard/"
	files := []string{"general_agent.md", "agent.md", "about.skill.md"}
	for _, f := range files {
		target := filepath.Join("./.swb/agents/shephard", f)
		if _, err := os.Stat(target); os.IsNotExist(err) {
			fmt.Printf("Downloading %s...\n", f)
			cmd := exec.Command("curl", "-sSL", baseUrl+f, "-o", target)
			if err := cmd.Run(); err != nil {
				fmt.Printf("✘ Failed to download %s: %v\n", f, err)
			}
		}
	}

	defaultCfg := Config{
		Server: ServerConfig{
			Port:          7070,
			Log:           "./swb.log",
			DB:            "./.swb/swb_db",
			GeminiModel:   "gemini-3-flash-preview",
			ShephardAgent: "./.swb/agents/shephard/agent.md",
			ShephardSkill: "./.swb/agents/shephard/about.skill.md",
			GeneralAgent:  "./.swb/agents/shephard/general_agent.md",
			Limit:         "120m",
		},
	}

	cfgData, _ := json.MarshalIndent(defaultCfg, "", "  ")
	os.WriteFile("./.swb/config.json", cfgData, 0644)
	fmt.Println("✔ Initialized environment.")
}

func cmdServer(args []string) {
	fs := flag.NewFlagSet("server", flag.ExitOnError)
	cfgPath := fs.String("config", "./.swb/config.json", "path to config")
	fs.Parse(args)

	configPath = *cfgPath
	if wd, err := os.Getwd(); err == nil {
		serverRoot = wd
	}

	f, err := os.Open(configPath)
	if err != nil {
		log.Fatalf("open config: %v", err)
	}
	defer f.Close()
	json.NewDecoder(f).Decode(&cfg)

	if cfg.Server.Port == 0 {
		cfg.Server.Port = 7070
	}
	if cfg.Server.Log == "" {
		cfg.Server.Log = "./swb.log"
	}
	if cfg.Server.DB == "" {
		cfg.Server.DB = "./.swb/swb_db"
	}
	if cfg.Server.GeminiModel == "" {
		cfg.Server.GeminiModel = "gemini-3-flash-preview"
	}
	if cfg.Server.GeminiToken == "" {
		cfg.Server.GeminiToken = os.Getenv("GEMINI_TOKEN")
	}
	if cfg.Server.Limit == "" {
		cfg.Server.Limit = "120m"
	}

	loadPromptFile := func(val string) string {
		if strings.HasSuffix(val, ".md") {
			data, err := os.ReadFile(val)
			if err == nil {
				return string(data)
			}
		}
		return val
	}
	cfg.Server.ShephardSkill = loadPromptFile(cfg.Server.ShephardSkill)
	cfg.Server.ShephardAgent = loadPromptFile(cfg.Server.ShephardAgent)
	cfg.Server.GeneralAgent = loadPromptFile(cfg.Server.GeneralAgent)

	logFile, _ := os.OpenFile(cfg.Server.Log, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	mw := io.MultiWriter(os.Stdout, logFile)
	logger = slog.New(slog.NewTextHandler(mw, &slog.HandlerOptions{Level: slog.LevelInfo}))

	os.MkdirAll(cfg.Server.DB, 0755)
	bootFreshRegistry()

	for agentName, ag := range cfg.Agents {
		if ag.Schedule != nil {
			taskMetas = append(taskMetas, taskMeta{Slug: agentName, Schedule: *ag.Schedule})
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
	mux.HandleFunc("GET /about", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, cfg.Server.GeneralAgent, cfg.Server.Port)
	})

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	logger.Info("HTTP server starting", "addr", addr)
	http.ListenAndServe(addr, loggingMiddleware(mux))
}

func cmdRunSpec(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	model := fs.String("model", "gemini-3-flash-preview", "Gemini model name")
	apiKey := fs.String("api-key", "", "Gemini API key (default: $GEMINI_TOKEN)")
	sessionID := fs.String("session", "", "Resume an existing session by ID")
	dir := fs.String("dir", "", "Working directory — restricts agent to this path (default: cwd)")
	maxIter := fs.Int("max-iter", 50, "Max tool-call iterations")
	prompt := fs.String("prompt", "", "Run a single prompt then exit (non-interactive)")
	interactive := fs.Bool("i", false, "Start a simple REPL")
	thinking := fs.String("thinking", "none", "Thinking budget: none | low | high")
	jsonMode := fs.Bool("json", false, "Emit all events as JSON lines; text tokens go to stderr")
	fs.Parse(args)

	// support positional prompt: `swb run "my prompt"`
	if *prompt == "" && len(fs.Args()) > 0 {
		p := strings.Join(fs.Args(), " ")
		prompt = &p
	}

	ctx := context.Background()

	// default token from env if not provided
	if *apiKey == "" {
		*apiKey = os.Getenv("GEMINI_TOKEN")
	}

	toroidCfg := toroid.Config{
		Model:     *model,
		APIKey:    *apiKey,
		SessionID: *sessionID,
		Resume:    *sessionID != "",
		WorkDir:   *dir,
		MaxIter:   *maxIter,
		Thinking:  toroid.Thinking(*thinking),
	}
	if toroid.Thinking(*thinking) != toroid.ThinkingNone {
		toroidCfg.ThinkingWriter = newThinkingWriter(os.Stderr)
	}

	a, err := toroid.New(ctx, toroidCfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	// print session header
	home, _ := os.UserHomeDir()
	logPath := fmt.Sprintf("%s/.toroid/sessions/%s.jsonl", home, a.SessionID())
	fmt.Fprintf(os.Stderr, "%ssession%s  %s\n", ansiDim, ansiReset, a.SessionID())
	fmt.Fprintf(os.Stderr, "%slogs%s     tail -f %s\n", ansiDim, ansiReset, logPath)

	// set up text writer — gate buffers output until title is ready
	var textWriter io.Writer = newMarkdownWriter(os.Stdout)
	var gate *titleGate
	if !*jsonMode {
		gate = newTitleGate(textWriter)
		textWriter = gate
	}

	// print title as part of the header metadata, then release buffered output
	a.On(toroid.EventTitle, func(_ context.Context, e toroid.Event) error {
		p := e.Payload.(*toroid.TitlePayload)
		hr := ansiDim + strings.Repeat("─", 48) + ansiReset
		fmt.Fprintf(os.Stderr, "\n%s\n%s%s%s\n%s\n\n", hr, ansiBold, p.Title, ansiReset, hr)
		if gate != nil {
			gate.Release()
		}
		return nil
	})
	// safety: if session ends before title arrives, release gate so output isn't lost
	a.On(toroid.EventSessionEnd, func(_ context.Context, _ toroid.Event) error {
		if gate != nil {
			fmt.Fprintln(os.Stderr)
			gate.Release()
		}
		return nil
	})

	// tool call display
	if !*jsonMode {
		a.On(toroid.EventPreToolUse, func(_ context.Context, e toroid.Event) error {
			p := e.Payload.(*toroid.ToolUsePayload)
			s := renderPreToolUse(p.Name, p.Args)
			if s != "" {
				fmt.Fprintf(os.Stderr, "\n%s\n", s)
			}
			return nil
		})
		a.On(toroid.EventPostToolUse, func(_ context.Context, e toroid.Event) error {
			p := e.Payload.(*toroid.ToolUsePayload)
			s := renderPostToolUse(p.Name, p.Result)
			if s != "" {
				fmt.Fprintf(os.Stderr, "%s\n", s)
			}
			return nil
		})
		a.On(toroid.EventPostToolUseFailure, func(_ context.Context, e toroid.Event) error {
			p := e.Payload.(*toroid.ToolUsePayload)
			fmt.Fprintf(os.Stderr, "  %s✗ %s%s\n", ansiRed, p.Error, ansiReset)
			return nil
		})
	}

	if *jsonMode {
		textWriter = os.Stderr
		enc := json.NewEncoder(os.Stdout)
		enc.SetEscapeHTML(false)
		allEvents := []toroid.EventKind{
			toroid.EventSessionStart, toroid.EventSessionEnd,
			toroid.EventUserPromptSubmit, toroid.EventToken,
			toroid.EventPreToolUse, toroid.EventPostToolUse,
			toroid.EventPostToolUseFailure, toroid.EventPermissionRequest,
			toroid.EventStop, toroid.EventSubagentStart,
			toroid.EventSubagentStop, toroid.EventMasterIdle,
			toroid.EventTaskCompleted, toroid.EventNotification,
			toroid.EventPreCompact,
		}
		for _, kind := range allEvents {
			k := kind
			a.On(k, func(_ context.Context, e toroid.Event) error {
				return enc.Encode(e)
			})
		}
	}

	run := func(p string) {
		if g, ok := textWriter.(*titleGate); ok {
			defer g.Flush()
		} else if mw, ok := textWriter.(*markdownWriter); ok {
			defer mw.Flush()
		}
		if err := a.Stream(ctx, p, textWriter); err != nil {
			fmt.Fprintln(os.Stderr, ansiRed+"error:"+ansiReset, err)
		}
		fmt.Fprintln(os.Stdout)
		fmt.Fprintln(os.Stderr)
	}

	if *prompt != "" {
		if *interactive {
			fmt.Fprintln(os.Stderr, "\n"+renderBoxedPrompt(*prompt))
		}
		run(*prompt)
		if !*interactive {
			return
		}
	}

	// interactive REPL
	sc := bufio.NewScanner(os.Stdin)
	fmt.Fprint(os.Stderr, "\n"+ansiBold+"> "+ansiReset)
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			fmt.Fprint(os.Stderr, ansiBold+"> "+ansiReset)
			continue
		}
		if *interactive {
			fmt.Fprintln(os.Stderr, "\n"+renderBoxedPrompt(line))
		}
		run(line)
		fmt.Fprint(os.Stderr, "\n"+ansiBold+"> "+ansiReset)
	}
}

// ─── Logic ────────────────────────────────────────────────────────────────────

func kvSet(key string, val any) {
	b, _ := json.Marshal(val)
	path := filepath.Join(cfg.Server.DB, strings.ReplaceAll(key, ":", "_")+".json")
	os.WriteFile(path, b, 0644)
}

func kvGet(key string, out any) error {
	path := filepath.Join(cfg.Server.DB, strings.ReplaceAll(key, ":", "_")+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}

func newID() string {
	b := make([]byte, 4)
	rand.Read(b)
	return time.Now().Format("20060102150405") + "_" + hex.EncodeToString(b)
}

func bootFreshRegistry() {
	files, _ := filepath.Glob(filepath.Join(cfg.Server.DB, "job_*.json"))
	for _, path := range files {
		data, _ := os.ReadFile(path)
		var job JobState
		if err := json.Unmarshal(data, &job); err == nil {
			if job.Status == "running" {
				if job.PGID > 0 {
					syscall.Kill(-job.PGID, syscall.SIGKILL)
				}
				job.Status = "interrupted"
				job.EndedAt = time.Now()
				kvSet("job:"+job.ID, job)
			}
			registryMu.Lock()
			registry[job.ID] = &job
			registryMu.Unlock()
		}
	}
}

func streamOutput(job *JobState, pr io.ReadCloser) {
	defer pr.Close()
	scanner := bufio.NewScanner(pr)
	for scanner.Scan() {
		line := ansiRe.ReplaceAllString(scanner.Text(), "")
		if line = strings.TrimSpace(line); line != "" {
			registryMu.Lock()
			job.Output = append(job.Output, line)
			registryMu.Unlock()
		}
	}
}

func toroidWorker(job *JobState, folder string, persistPrompt bool, taskPrefix string) {
	os.MkdirAll(filepath.Join(folder, ".tmp"), 0755)
	registryMu.Lock()
	job.Status = "running"
	job.StartedAt = time.Now()
	registryMu.Unlock()
	kvSet("job:"+job.ID, job)

	ctx, cancel := context.WithDeadline(context.Background(), job.KillAt)
	defer cancel()

	agent, _ := toroid.New(ctx, toroid.Config{
		Model:     cfg.Server.GeminiModel,
		APIKey:    cfg.Server.GeminiToken,
		SessionID: job.ID,
		WorkDir:   folder,
	})

	pr, pw, _ := os.Pipe()
	done := make(chan struct{})
	go func() { streamOutput(job, pr); close(done) }()
	runErr := agent.Stream(ctx, job.Prompt, pw)
	pw.Close()
	<-done

	registryMu.Lock()
	job.EndedAt = time.Now()
	if runErr != nil {
		job.Status = "failed"
	} else {
		job.Status = "done"
	}
	registryMu.Unlock()
	kvSet("job:"+job.ID, job)
}

func bashWorker(job *JobState, folder string, command string) {
	ctx, cancel := context.WithDeadline(context.Background(), job.KillAt)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	cmd.Dir = folder
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	pr, pw, _ := os.Pipe()
	cmd.Stdout, cmd.Stderr = pw, pw
	cmd.Start()
	pw.Close()
	registryMu.Lock()
	job.Status = "running"
	job.StartedAt = time.Now()
	job.PGID = cmd.Process.Pid
	registryMu.Unlock()
	kvSet("job:"+job.ID, job)
	streamOutput(job, pr)
	cmd.Wait()
	registryMu.Lock()
	job.EndedAt = time.Now()
	job.Status = "done"
	registryMu.Unlock()
	kvSet("job:"+job.ID, job)
}

func spawnJob(agentName string, slug string, ag AgentConfig, queryParams map[string]string, body map[string]any, id string, outputType string) string {
	limit, _ := time.ParseDuration(cfg.Server.Limit)
	outputFilePath := ag.Folder + "/.tmp/OUTPUT-" + id + ".json"
	job := &JobState{
		ID: id, Agent: agentName, Slug: slug,
		Prompt: fmt.Sprintf(cfg.Server.GeneralAgent, time.Now().Format("2006-01-02-15-04"), ag.Folder, ag.Prompt, slug, queryParams, body, outputFilePath),
		Status: "pending", CreatedAt: time.Now(), KillAt: time.Now().Add(limit), OutputFile: outputFilePath,
	}
	registryMu.Lock()
	registry[job.ID] = job
	registryMu.Unlock()
	kvSet("job:"+job.ID, job)
	go toroidWorker(job, ag.Folder, true, "tasks")
	return id
}

func killJob(id string) {
	registryMu.Lock()
	job, ok := registry[id]
	registryMu.Unlock()
	if ok && job.PGID > 0 {
		syscall.Kill(-job.PGID, syscall.SIGKILL)
	}
}

func startScheduler() {
	ticker := time.NewTicker(1 * time.Minute)
	for range ticker.C {
		now := time.Now()
		for i := range taskMetas {
			meta := &taskMetas[i]
			if meta.LastRun != now.Format("2006-01-02-15-04") && shouldRunNow(meta.Schedule, now) {
				spawnJob(meta.Slug, "", cfg.Agents[meta.Slug], nil, nil, newID(), "")
				meta.LastRun = now.Format("2006-01-02-15-04")
			}
		}
	}
}

func shouldRunNow(s ScheduleConfig, now time.Time) bool {
	if now.Format("15:04") != s.Time {
		return false
	}
	switch s.Every {
	case "day":
		return true
	case "week":
		return strings.Contains(strings.ToLower(s.Weekdays), strings.ToLower(now.Weekday().String()[:3]))
	case "month":
		return strings.Contains(s.Monthdays, strconv.Itoa(now.Day()))
	}
	return false
}

func handleCrons(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]any{"schedules": taskMetas})
}

func startMetricsSampler() {
	ticker := time.NewTicker(10 * time.Second)
	for range ticker.C {
		registryMu.RLock()
		for id, j := range registry {
			if j.Status == "running" && j.PGID > 0 {
				out, _ := exec.Command("ps", "-p", strconv.Itoa(j.PGID), "-o", "%cpu,rss").Output()
				fields := strings.Fields(string(out))
				if len(fields) >= 4 {
					cpu, _ := strconv.ParseFloat(fields[2], 64)
					rss, _ := strconv.ParseUint(fields[3], 10, 64)
					metricsMu.Lock()
					ms, ok := metrics[id]
					if !ok {
						ms = &MetricSummary{JobID: id}
						metrics[id] = ms
					}
					ms.SampleCount++
					n := float64(ms.SampleCount)
					delta := cpu - ms.CPUMean
					ms.CPUMean += delta / n
					if cpu > ms.CPUMax {
						ms.CPUMax = cpu
					}
					ms.CPUCurrent = cpu
					ms.memSum += float64(rss * 1024)
					ms.MemMean = ms.memSum / n
					if rss*1024 > ms.MemMax {
						ms.MemMax = rss * 1024
					}
					ms.MemCurrent = rss * 1024
					ms.LastSampled = time.Now()
					metricsMu.Unlock()
				}
			}
		}
		registryMu.RUnlock()
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) { r.status = code; r.ResponseWriter.WriteHeader(code) }
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{w, 200}
		next.ServeHTTP(rec, r)
		logger.Info("request", "method", r.Method, "path", r.URL.Path, "status", rec.status, "latency", time.Since(start).String())
	})
}

func handleSpawn(w http.ResponseWriter, r *http.Request) {
	parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/app/"), "/", 2)
	ag, ok := cfg.Agents[parts[0]]
	if !ok {
		http.Error(w, "unknown agent", 404)
		return
	}
	var body map[string]any
	json.NewDecoder(r.Body).Decode(&body)
	id := spawnJob(parts[0], strings.Join(parts[1:], "/"), ag, nil, body, newID(), "")
	json.NewEncoder(w).Encode(map[string]string{"id": id})
}

func handleJobStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	registryMu.RLock()
	job, ok := registry[id]
	registryMu.RUnlock()
	if ok {
		json.NewEncoder(w).Encode(map[string]any{"job": job, "done": jobDone(job)})
		return
	}
	http.Error(w, "not found", 404)
}

func handleKill(w http.ResponseWriter, r *http.Request) {
	var req struct{ ID string }
	json.NewDecoder(r.Body).Decode(&req)
	killJob(req.ID)
}

func handleShephard(w http.ResponseWriter, r *http.Request) {
	var m struct{ Message string }
	json.NewDecoder(r.Body).Decode(&m)
	shephardDir := ".shephard"
	os.MkdirAll(shephardDir, 0755)
	id := newID()
	job := &JobState{ID: id, Slug: "shephard", Status: "pending", CreatedAt: time.Now(), KillAt: time.Now().Add(120 * time.Minute)}
	if m.Message[0] == '!' {
		go bashWorker(job, shephardDir, m.Message[1:])
	} else {
		job.Prompt = fmt.Sprintf(cfg.Server.ShephardAgent, fmt.Sprintf(cfg.Server.ShephardSkill, cfg.Server.Port), m.Message)
		go toroidWorker(job, shephardDir, true, "shephard")
	}
	registryMu.Lock()
	registry[id] = job
	registryMu.Unlock()
	kvSet("job:"+id, job)
	json.NewEncoder(w).Encode(map[string]string{"id": id})
}

// ── sessions ──────────────────────────────────────────────────────────────────

func cmdSessions(args []string) {
	fs := flag.NewFlagSet("sessions", flag.ExitOnError)
	deleteID := fs.String("delete", "", "Delete session by ID")
	fs.Parse(args)

	if *deleteID != "" {
		if err := toroid.DeleteSession(*deleteID); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "deleted session %s\n", *deleteID)
		return
	}

	sessions, err := toroid.ListSessions()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	if len(sessions) == 0 {
		fmt.Fprintln(os.Stderr, "no sessions found")
		return
	}

	fmt.Fprintf(os.Stderr, "%s%-24s  %-14s  %s%s\n", ansiBold, "SESSION ID", "WHEN", "TITLE", ansiReset)
	fmt.Fprintln(os.Stderr, ansiDim+fmt.Sprintf("%-24s  %-14s  %s", "──────────────────────", "────────────", "──────────────────────────────────────────────")+ansiReset)
	for _, s := range sessions {
		when := relativeTime(s.ID)
		fmt.Fprintf(os.Stderr, "%-24s  %s%-14s%s  %s\n", s.ID, ansiDim, when, ansiReset, truncate(s.Title, 60))
	}
}

func relativeTime(sessionID string) string {
	ns, err := strconv.ParseInt(sessionID, 10, 64)
	if err != nil {
		return ""
	}
	d := time.Since(time.Unix(0, ns))
	switch {
	case d < time.Hour:
		m := int(d.Minutes())
		if m <= 1 {
			return "just now"
		}
		return fmt.Sprintf("%d minutes ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		return fmt.Sprintf("%d hours ago", h)
	case d < 7*24*time.Hour:
		days := int(d.Hours() / 24)
		return fmt.Sprintf("%d days ago", days)
	case d < 30*24*time.Hour:
		weeks := int(d.Hours() / 24 / 7)
		return fmt.Sprintf("%d weeks ago", weeks)
	default:
		months := int(d.Hours() / 24 / 30)
		return fmt.Sprintf("%d months ago", months)
	}
}

// ── titleGate ─────────────────────────────────────────────────────────────────

type titleGate struct {
	mu   sync.Mutex
	buf  bytes.Buffer
	w    io.Writer
	open bool
}

func newTitleGate(w io.Writer) *titleGate { return &titleGate{w: w} }

func (g *titleGate) Write(p []byte) (int, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.open {
		return g.w.Write(p)
	}
	return g.buf.Write(p)
}

func (g *titleGate) Release() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.open {
		return
	}
	g.open = true
	if g.buf.Len() > 0 {
		g.w.Write(g.buf.Bytes())
		g.buf.Reset()
	}
}

func (g *titleGate) Flush() {
	if mw, ok := g.w.(*markdownWriter); ok {
		mw.Flush()
	}
}

// ── Markdown writer ───────────────────────────────────────────────────────────

type markdownWriter struct {
	w      io.Writer
	buf    strings.Builder
	inCode bool
}

func newMarkdownWriter(w io.Writer) *markdownWriter { return &markdownWriter{w: w} }

func (m *markdownWriter) Write(p []byte) (int, error) {
	m.buf.Write(p)
	for {
		s := m.buf.String()
		idx := strings.Index(s, "\n")
		if idx == -1 {
			break
		}
		line := s[:idx]
		m.buf.Reset()
		m.buf.WriteString(s[idx+1:])
		fmt.Fprintln(m.w, m.renderLine(line))
	}
	return len(p), nil
}

func (m *markdownWriter) Flush() {
	if s := m.buf.String(); s != "" {
		fmt.Fprint(m.w, m.renderLine(s))
		m.buf.Reset()
	}
}

func (m *markdownWriter) renderLine(line string) string {
	if strings.HasPrefix(line, "```") {
		m.inCode = !m.inCode
		lang := strings.TrimPrefix(line, "```")
		if m.inCode {
			label := ""
			if lang != "" {
				label = " " + lang
			}
			return ansiCyan + "┌─" + label + ansiReset
		}
		return ansiCyan + "└─" + ansiReset
	}
	if m.inCode {
		return ansiDim + "│ " + line + ansiReset
	}
	if strings.HasPrefix(line, "### ") {
		return ansiBold + ansiBlue + strings.TrimPrefix(line, "### ") + ansiReset
	}
	if strings.HasPrefix(line, "## ") {
		return ansiBold + ansiUnderline + ansiBlue + strings.TrimPrefix(line, "## ") + ansiReset
	}
	if strings.HasPrefix(line, "# ") {
		return ansiBold + ansiUnderline + ansiCyan + strings.TrimPrefix(line, "# ") + ansiReset
	}
	if strings.HasPrefix(line, "> ") {
		return ansiDim + ansiItalic + "▎ " + renderInline(strings.TrimPrefix(line, "> ")) + ansiReset
	}
	for _, prefix := range []string{"- ", "* ", "+ "} {
		if strings.HasPrefix(line, prefix) {
			return "  • " + renderInline(strings.TrimPrefix(line, prefix))
		}
	}
	if line == "---" || line == "***" || line == "___" {
		return ansiDim + strings.Repeat("─", 60) + ansiReset
	}
	return renderInline(line)
}

func renderInline(s string) string {
	s = reBold.ReplaceAllStringFunc(s, func(m string) string {
		inner := reBold.FindStringSubmatch(m)
		text := inner[1]
		if text == "" {
			text = inner[2]
		}
		return ansiBold + text + ansiReset
	})
	s = reItalic.ReplaceAllStringFunc(s, func(m string) string {
		inner := reItalic.FindStringSubmatch(m)
		text := inner[1]
		if text == "" {
			text = inner[2]
		}
		return ansiItalic + text + ansiReset
	})
	s = reInlineCode.ReplaceAllStringFunc(s, func(m string) string {
		inner := reInlineCode.FindStringSubmatch(m)
		return ansiCyan + inner[1] + ansiReset
	})
	return s
}

// ── Tool call renderer ────────────────────────────────────────────────────────

func renderPreToolUse(name string, args map[string]any) string {
	var b strings.Builder
	w := &b

	switch name {
	case "bash":
		fmt.Fprintf(w, "%s$ %s%s", ansiGreen, ansiReset, strMapGet(args, "command"))

	case "read_file":
		path := strMapGet(args, "path")
		start, hasStart := args["start_line"]
		end, hasEnd := args["end_line"]
		if hasStart && hasEnd {
			fmt.Fprintf(w, "> %scat%s %s:%v-%v", ansiCyan, ansiReset, path, start, end)
		} else {
			fmt.Fprintf(w, "> %scat%s %s", ansiCyan, ansiReset, path)
		}

	case "write_file":
		path := strMapGet(args, "path")
		lines := strings.Count(strMapGet(args, "content"), "\n") + 1
		fmt.Fprintf(w, "> Writing %s%d lines%s to %s%s%s",
			ansiBold, lines, ansiReset, ansiItalic, path, ansiReset)

	case "edit_file":
		path := strMapGet(args, "path")
		old := truncate(strMapGet(args, "old_str"), 40)
		fmt.Fprintf(w, "> %s~%s %s  %s%q%s", ansiYellow, ansiReset, path, ansiDim, old, ansiReset)

	case "todo_write":
		tasks, _ := args["tasks"].([]any)
		label := "Tasks"
		for _, t := range tasks {
			if tm, ok := t.(map[string]any); ok {
				s := strMapGet(tm, "status")
				if s == "done" || s == "in_progress" {
					label = "Update Tasks"
					break
				}
			}
		}
		fmt.Fprintf(w, "> %s%s%s\n", ansiBold, label, ansiReset)
		renderTaskTree(w, tasks)

	case "todo_read":
		fmt.Fprintf(w, "> %sTasks%s", ansiBold, ansiReset)

	case "subagent":
		fmt.Fprintf(w, "> %s>_%s %s", ansiBlue, ansiReset, truncate(strMapGet(args, "prompt"), 80))

	case "notify":
		fmt.Fprintf(w, "> %snotify%s %q  %s%s%s",
			ansiYellow, ansiReset, strMapGet(args, "title"),
			ansiDim, strMapGet(args, "message"), ansiReset)

	default:
		fmt.Fprintf(w, "> %s%s%s", ansiYellow, name, ansiReset)
	}

	return b.String()
}

func renderPostToolUse(name string, result any) string {
	var b strings.Builder
	w := &b

	switch name {
	case "bash":
		m, ok := result.(map[string]any)
		if !ok {
			break
		}
		out := strings.TrimSpace(fmt.Sprint(m["output"]))
		if out == "" {
			out = strings.TrimSpace(fmt.Sprint(m["stdout"]))
		}
		if out == "" {
			out = strings.TrimSpace(fmt.Sprint(m["stderr"]))
		}
		code := fmt.Sprint(m["exit_code"])
		if code != "0" {
			fmt.Fprintf(w, "%s[exit %s]%s %s", ansiRed, code, ansiReset, truncate(out, 200))
		} else {
			fmt.Fprintf(w, "%s[ok]%s %s", ansiGreen, ansiReset, truncate(out, 200))
		}

	case "write_file", "edit_file", "todo_write":
		// info already shown in pre

	case "todo_read":
		if tasks := toTaskList(result); len(tasks) > 0 {
			renderTaskTree(w, tasks)
		}

	case "read_file":
		s := strings.TrimSpace(fmt.Sprint(result))
		if s != "" {
			renderOutputLines(w, s, 4)
		}

	default:
		fmt.Fprintf(w, "%s%s%s", ansiDim, truncate(fmt.Sprint(result), 120), ansiReset)
	}

	return b.String()
}

func renderTaskTree(w *strings.Builder, tasks []any) {
	for i, t := range tasks {
		tm, ok := t.(map[string]any)
		if !ok {
			continue
		}
		conn := "├──"
		if i == len(tasks)-1 {
			conn = "└──"
		}
		status := strMapGet(tm, "status")
		title := strMapGet(tm, "title")
		if title == "" {
			title = strMapGet(tm, "id")
		}
		var box, color string
		switch status {
		case "done", "completed":
			box, color = "✓", ansiGreen
		case "in_progress":
			box, color = "*", ansiYellow
		case "failed", "error":
			box, color = "!", ansiRed
		default:
			box, color = " ", ansiDim
		}
		fmt.Fprintf(w, "  %s %s[%s]%s %s\n", conn, color, box, ansiReset, title)
	}
}

func renderOutputLines(w *strings.Builder, output string, maxLines int) {
	lines := strings.Split(output, "\n")
	shown := lines
	extra := 0
	if len(lines) > maxLines {
		shown = lines[:maxLines]
		extra = len(lines) - maxLines
	}
	for _, l := range shown {
		fmt.Fprintf(w, "  %s%s%s\n", ansiDim, l, ansiReset)
	}
	if extra > 0 {
		fmt.Fprintf(w, "  %s… %d more lines%s\n", ansiDim, extra, ansiReset)
	}
}

func toTaskList(result any) []any {
	switch v := result.(type) {
	case []any:
		return v
	case []map[string]any:
		out := make([]any, len(v))
		for i, m := range v {
			out[i] = m
		}
		return out
	}
	return nil
}

func strMapGet(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		return fmt.Sprint(v)
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func renderBoxedPrompt(prompt string) string {
	lines := strings.Split(strings.TrimSpace(prompt), "\n")
	var maxLen int
	for _, l := range lines {
		if len(l) > maxLen {
			maxLen = len(l)
		}
	}
	var b strings.Builder
	b.WriteString(ansiBgGrey + strings.Repeat(" ", maxLen+4) + ansiReset + "\n")
	for _, l := range lines {
		padding := maxLen - len(l)
		b.WriteString(ansiBgGrey + "  " + l + strings.Repeat(" ", padding) + "  " + ansiReset + "\n")
	}
	b.WriteString(ansiBgGrey + strings.Repeat(" ", maxLen+4) + ansiReset)
	return b.String()
}

// ── thinking writer ───────────────────────────────────────────────────────────

type thinkingWriter struct {
	w       io.Writer
	started bool
}

func newThinkingWriter(w io.Writer) *thinkingWriter { return &thinkingWriter{w: w} }

func (t *thinkingWriter) Write(p []byte) (int, error) {
	if !t.started {
		t.started = true
		fmt.Fprint(t.w, ansiDim+"[thinking]\n")
	}
	return t.w.Write(p)
}

func (t *thinkingWriter) Reset() {
	if t.started {
		fmt.Fprint(t.w, ansiReset+"\n")
		t.started = false
	}
}
