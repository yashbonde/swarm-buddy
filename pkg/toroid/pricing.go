package toroid

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"charm.land/fantasy"
)

// INRPerUSD is the exchange rate used to convert USD costs to Indian Rupees.
const INRPerUSD = 94.0

// ModelPricing defines the cost per token for an LLM.
type ModelPricing struct {
	Prompt     float64 `json:"Prompt"`
	Completion float64 `json:"Completion"`
	Reasoning  float64 `json:"Reasoning"`
	CacheRead  float64 `json:"CacheRead"`
	CacheWrite float64 `json:"CacheWrite"`
}

// Pricing handles loading and retrieving model pricing information.
type Pricing struct {
	mu    sync.RWMutex
	table map[string]ModelPricing
}

var (
	defaultPricing *Pricing
	once           sync.Once
)

// GetDefaultPricing returns the singleton pricing instance.
func GetDefaultPricing() *Pricing {
	once.Do(func() {
		defaultPricing = &Pricing{
			table: make(map[string]ModelPricing),
		}
		// Attempt to load from default location
		_ = defaultPricing.Load("assets/pricing.json")
	})
	return defaultPricing
}

// Load reads pricing data from a JSON file.
func (p *Pricing) Load(path string) error {
	if !filepath.IsAbs(path) {
		// Try to find relative to current working directory
		path = filepath.Clean(path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var newTable map[string]ModelPricing
	if err := json.Unmarshal(data, &newTable); err != nil {
		return err
	}

	p.mu.Lock()
	p.table = newTable
	p.mu.Unlock()

	return nil
}

// Get returns the pricing for a given model ID.
func (p *Pricing) Get(modelID string) ModelPricing {
	id := strings.ToLower(modelID)
	if strings.HasPrefix(id, "models/") {
		id = id[7:]
	}

	p.mu.RLock()
	defer p.mu.RUnlock()

	// Direct match
	if pricing, ok := p.table[id]; ok {
		return pricing
	}

	// Match with google/ prefix
	if !strings.Contains(id, "/") {
		if pricing, ok := p.table["google/"+id]; ok {
			return pricing
		}
	}

	// Reverse match (google/gemini-2.0-flash-001 -> google/gemini-2.0-flash)
	for k, pricing := range p.table {
		if strings.HasPrefix(id, k) || strings.HasPrefix(k, id) {
			return pricing
		}
	}

	return ModelPricing{}
}

// CalculateCost computes the total cost for a usage breakdown using default pricing.
func CalculateCost(modelID string, usage Usage) float64 {
	p := GetDefaultPricing().Get(modelID)

	// In most modern APIs (Gemini, OpenAI O1/O3), OutputTokens include ReasoningTokens.
	// We subtract reasoning to avoid double-charging.
	contentTokens := float64(usage.Output - usage.Reasoning)
	if contentTokens < 0 {
		contentTokens = 0
	}

	return float64(usage.Input)*p.Prompt +
		contentTokens*p.Completion +
		float64(usage.Reasoning)*p.Reasoning +
		float64(usage.CacheRead)*p.CacheRead +
		float64(usage.CacheWrite)*p.CacheWrite
}

// GetPricing is a legacy helper for GetDefaultPricing().Get().
func GetPricing(modelID string) ModelPricing {
	return GetDefaultPricing().Get(modelID)
}

// Usage tracker

type Usage struct {
	Output     int64
	Input      int64
	Reasoning  int64
	CacheRead  int64
	CacheWrite int64
	Cost       float64
}

func (u *Usage) FromFantasyUsage(usage fantasy.Usage, model string) {
	u.Output = usage.OutputTokens
	u.Input = usage.InputTokens
	u.Reasoning = usage.ReasoningTokens
	u.CacheRead = usage.CacheReadTokens
	u.CacheWrite = usage.CacheCreationTokens
	u.Cost = CalculateCost(model, *u)
}
