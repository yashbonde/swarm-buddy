// MIT License - github.com/yashbonde
// swb

package main

import (
	"fmt"
	"os"
	"regexp"
	"sync"
	"time"

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
	logger interface {
		Info(msg string, args ...any)
	}

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

// keep toroid import used here so other files can reference it freely
var _ = toroid.ThinkingNone

func usage() {
	fmt.Fprintln(os.Stderr, `Usage: swb <command> [flags]

Commands:
  init      Initialize default config and agents
  server    Start the intelligence orchestrator
  run       Run a one-off prompt
  sessions  List or delete sessions

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
