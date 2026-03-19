package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"swarm_buddy/pkg/toroid"

	"charm.land/fantasy/providers/google"
)

// ── event log ────────────────────────────────────────────────────────────────

type row struct {
	session string
	kind    string
	detail  string
}

var log []row

func record(session, kind, detail string) {
	log = append(log, row{session, kind, detail})
}

// ── helpers ──────────────────────────────────────────────────────────────────

func shortID(id string) string {
	// Keep only alphanumeric characters
	var out []rune
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			out = append(out, r)
		}
	}
	s := string(out)
	if len(s) > 12 {
		return s[:12]
	}
	return s
}

func usageLine(tokens map[string]toroid.Usage) string {
	var parts []string
	for sid, u := range tokens {
		parts = append(parts, fmt.Sprintf("%s → in:%d out:%d rs:%d cr:%d cw:%d | $%.6f",
			shortID(sid), u.Input, u.Output, u.Reasoning, u.CacheRead, u.CacheWrite, u.Cost))
	}
	return strings.Join(parts, " | ")
}

// ── hook wiring ──────────────────────────────────────────────────────────────

func logForTable(kernel *toroid.Kernel, label string) {
	kernel.On(toroid.EventSessionStart, func(_ context.Context, e toroid.Event) error {
		record(label, "Start", shortID(e.SessionID))
		return nil
	})

	kernel.On(toroid.EventPreToolUse, func(_ context.Context, e toroid.Event) error {
		if p, ok := e.Payload.(*toroid.ToolUsePayload); ok {
			record(label, "ToolCall", p.Name)
		}
		return nil
	})

	kernel.On(toroid.EventPostToolUse, func(_ context.Context, e toroid.Event) error {
		if p, ok := e.Payload.(*toroid.ToolUsePayload); ok {
			record(label, "ToolDone", p.Name)
		}
		return nil
	})

	kernel.On(toroid.EventReasoning, func(_ context.Context, e toroid.Event) error {
		if p, ok := e.Payload.(*toroid.ReasoningPayload); ok {
			record(label, "Thinking", p.Text)
		}
		return nil
	})

	kernel.On(toroid.EventPreCompact, func(_ context.Context, e toroid.Event) error {
		if p, ok := e.Payload.(*toroid.CompactPayload); ok {
			record(label, "Compact", fmt.Sprintf("%d messages", p.MessageCount))
		}
		return nil
	})

	kernel.On(toroid.EventStop, func(_ context.Context, e toroid.Event) error {
		if p, ok := e.Payload.(*toroid.UsagePayload); ok {
			record(label, "Cost", usageLine(p.Tokens))
		}
		return nil
	})
}

// ── main ─────────────────────────────────────────────────────────────────────

func main() {
	sequence := flag.Bool("sequence", false, "Run kernel in sequence mode")
	block := flag.Bool("block", false, "Run kernel in block mode")
	attachLoggerHooks := flag.Bool("logs", false, "Attach logger hooks")
	showHistory := flag.Bool("show-history", false, "Show history")
	// prompt := flag.String("prompt", "", "Optional prompt to use with block mode")
	flag.Parse()
	ctx := context.Background()

	// Initialize Fantasy Provider
	apiKey := os.Getenv("GEMINI_TOKEN")
	if apiKey == "" {
		fmt.Println("GEMINI_TOKEN environment variable not set")
		os.Exit(1)
	}
	provider, err := google.New(google.WithGeminiAPIKey(apiKey))
	if err != nil {
		panic(err)
	}

	// Initialize Kernel
	k, err := toroid.NewKernel(ctx, toroid.Config{
		Provider:             provider,
		Model:                "models/gemini-3-flash-preview",
		Thinking:             toroid.ThinkingLow,
		AttachLoggerHooks:    attachLoggerHooks,
		TotalContextSize:     20000,
		CompactionBufferSize: 2000,
		ShowHistory:          showHistory,
	})
	if err != nil {
		panic(err)
	}

	if *sequence {
		logForTable(k, "SEQUENCE") // name in table
		runKernelStepByStep(ctx, k)
		printTable()
		log = []row{} // clear log
	}
	if *block {
		logForTable(k, "BLOCK") // name in table
		runKernelBlock(ctx, k)
		printTable()
		log = []row{} // clear log
	}

}

func runKernelStepByStep(ctx context.Context, k *toroid.Kernel) {
	// 3. Run Kernel
	fmt.Println("Running Kernel (steps) ... \n[1/5] Basic")
	resp, _, err := k.Run(ctx, "Give a JSON (indent=2) dump of tool names you have access to. JSON and nothing else.")
	if err != nil {
		panic(err)
	}
	fmt.Printf("\n%s\n", resp)

	// 4. Perform a tool call
	fmt.Printf("\n[2/5] Tool Call\n")
	resp, _, err = k.Run(ctx, "List all the files in the current directory, find the funniest name.")
	if err != nil {
		panic(err)
	}
	fmt.Printf("\n%s\n", resp)

	// 5. Run a subagent
	fmt.Printf("\n[3/5] Subagent\n")
	resp, _, err = k.Run(ctx, "Run a subagent to read the first 10 lines of this file and tell me what you think.")
	if err != nil {
		panic(err)
	}
	fmt.Printf("\n%s\n", resp)

	// 7. Tell conversation
	fmt.Printf("\n[4/5] Recall (there should be compaction here)\n")
	resp, _, err = k.Run(ctx, "What are the things I have asked, only questions?")
	if err != nil {
		panic(err)
	}
	fmt.Printf("\n%s\n", resp)
}

func runKernelBlock(ctx context.Context, k *toroid.Kernel) {
	fmt.Println("Running Kernel (block) ...")
	out, _, err := k.Run(ctx, `Please do the following:
	- First, run a subagent to find if this is a git repo or not. (subagent only here)
	- If it is a git repo, then find the latest commit message.
	- Else, pick nearest file and get me the sha256 of it.
	- Send that data to https://webhook.site/0024a8a9-b4d4-49b9-bf80-b919a8d81cf7
	- List all the files in the current directory and tell me if there is a file with ".md" extension.
	- Run a subagent to read the first 10 lines of this file and tell me what you think.
	`)
	if err != nil {
		panic(err)
	}
	fmt.Println(out)
}

func printTable() {
	// ── Table ────────────────────────────────────────────────────────────────
	const colSession = 9
	const colKind = 14
	const colDetail = 85

	sep := fmt.Sprintf("+%s+%s+%s+",
		strings.Repeat("-", colSession+2),
		strings.Repeat("-", colKind+2),
		strings.Repeat("-", colDetail+2),
	)

	header := fmt.Sprintf("| %-*s | %-*s | %-*s |",
		colSession, "Session",
		colKind, "Event",
		colDetail, "Detail",
	)

	fmt.Println()
	fmt.Println(sep)
	fmt.Println(header)
	fmt.Println(sep)
	for _, r := range log {
		detail := r.detail

		for len(detail) > 0 {
			chunk := detail
			if len(chunk) > colDetail {
				chunk = chunk[:colDetail]
			}
			fmt.Printf("| %-*s | %-*s | %-*s |\n",
				colSession, r.session,
				colKind, r.kind,
				colDetail, chunk,
			)
			detail = detail[len(chunk):]
			// blank out session/kind for continuation lines
			r.session = ""
			r.kind = ""
		}
	}
	fmt.Println(sep)

	// Calculate Grand Total by summing all recorded cost events
	var totalUSD float64
	for _, r := range log {
		if r.kind == "Cost" {
			var turnCost float64
			parts := strings.Split(r.detail, "| $")
			if len(parts) > 1 {
				fmt.Sscanf(parts[1], "%f", &turnCost)
				totalUSD += turnCost
			}
		}
	}

	totalINR := totalUSD * 94.0
	grandTotal := fmt.Sprintf("GRAND TOTAL: $%.6f (₹%.4f)", totalUSD, totalINR)
	fmt.Printf("| %-*s |\n", colSession+colKind+colDetail+6, grandTotal)
	fmt.Println(sep)
}
