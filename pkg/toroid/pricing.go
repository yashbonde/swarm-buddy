package toroid

import "strings"

// ModelPricing defines the cost per token for an LLM.
type ModelPricing struct {
	Prompt     float64
	Completion float64
	Reasoning  float64
	CacheRead  float64
	CacheWrite float64
}

// ModelPricingTable maps model IDs to their pricing information.
// Rates are USD per token.
var ModelPricingTable = map[string]ModelPricing{
	"openai/gpt-5.4-nano": {
		Prompt:     2e-07,
		Completion: 1.25e-06,
		Reasoning:  0.0,
		CacheRead:  2e-08,
		CacheWrite: 0.0,
	},
	"openai/gpt-5.4-mini": {
		Prompt:     7.5e-07,
		Completion: 4.5e-06,
		Reasoning:  0.0,
		CacheRead:  7.5e-08,
		CacheWrite: 0.0,
	},
	"openai/gpt-5.4-pro": {
		Prompt:     3e-05,
		Completion: 0.00018,
		Reasoning:  0.0,
		CacheRead:  0.0,
		CacheWrite: 0.0,
	},
	"openai/gpt-5.4": {
		Prompt:     2.5e-06,
		Completion: 1.5e-05,
		Reasoning:  0.0,
		CacheRead:  2.5e-07,
		CacheWrite: 0.0,
	},
	"openai/gpt-5.3-chat": {
		Prompt:     1.75e-06,
		Completion: 1.4e-05,
		Reasoning:  0.0,
		CacheRead:  1.75e-07,
		CacheWrite: 0.0,
	},
	"google/gemini-3.1-flash-lite-preview": {
		Prompt:     2.5e-07,
		Completion: 1.5e-06,
		Reasoning:  1.5e-06,
		CacheRead:  2.5e-08,
		CacheWrite: 8.333333333333334e-08,
	},
	"google/gemini-3.1-flash-image-preview": {
		Prompt:     5e-07,
		Completion: 3e-06,
		Reasoning:  0.0,
		CacheRead:  0.0,
		CacheWrite: 0.0,
	},
	"google/gemini-3.1-pro-preview-customtools": {
		Prompt:     2e-06,
		Completion: 1.2e-05,
		Reasoning:  1.2e-05,
		CacheRead:  2e-07,
		CacheWrite: 3.75e-07,
	},
	"openai/gpt-5.3-codex": {
		Prompt:     1.75e-06,
		Completion: 1.4e-05,
		Reasoning:  0.0,
		CacheRead:  1.75e-07,
		CacheWrite: 0.0,
	},
	"google/gemini-3.1-pro-preview": {
		Prompt:     2e-06,
		Completion: 1.2e-05,
		Reasoning:  1.2e-05,
		CacheRead:  2e-07,
		CacheWrite: 3.75e-07,
	},
	"anthropic/claude-sonnet-4.6": {
		Prompt:     3e-06,
		Completion: 1.5e-05,
		Reasoning:  0.0,
		CacheRead:  3e-07,
		CacheWrite: 3.75e-06,
	},
	"anthropic/claude-opus-4.6": {
		Prompt:     5e-06,
		Completion: 2.5e-05,
		Reasoning:  0.0,
		CacheRead:  5e-07,
		CacheWrite: 6.25e-06,
	},
	"openai/gpt-audio": {
		Prompt:     2.5e-06,
		Completion: 1e-05,
		Reasoning:  0.0,
		CacheRead:  0.0,
		CacheWrite: 0.0,
	},
	"openai/gpt-audio-mini": {
		Prompt:     6e-07,
		Completion: 2.4e-06,
		Reasoning:  0.0,
		CacheRead:  0.0,
		CacheWrite: 0.0,
	},
	"openai/gpt-5.2-codex": {
		Prompt:     1.75e-06,
		Completion: 1.4e-05,
		Reasoning:  0.0,
		CacheRead:  1.75e-07,
		CacheWrite: 0.0,
	},
	"google/gemini-3-flash-preview": {
		Prompt:     5e-07,
		Completion: 3e-06,
		Reasoning:  3e-06,
		CacheRead:  5e-08,
		CacheWrite: 8.333333333333334e-08,
	},
	"openai/gpt-5.2-chat": {
		Prompt:     1.75e-06,
		Completion: 1.4e-05,
		Reasoning:  0.0,
		CacheRead:  1.75e-07,
		CacheWrite: 0.0,
	},
	"openai/gpt-5.2-pro": {
		Prompt:     2.1e-05,
		Completion: 0.000168,
		Reasoning:  0.0,
		CacheRead:  0.0,
		CacheWrite: 0.0,
	},
	"openai/gpt-5.2": {
		Prompt:     1.75e-06,
		Completion: 1.4e-05,
		Reasoning:  0.0,
		CacheRead:  1.75e-07,
		CacheWrite: 0.0,
	},
	"openai/gpt-5.1-codex-max": {
		Prompt:     1.25e-06,
		Completion: 1e-05,
		Reasoning:  0.0,
		CacheRead:  1.25e-07,
		CacheWrite: 0.0,
	},
	"anthropic/claude-opus-4.5": {
		Prompt:     5e-06,
		Completion: 2.5e-05,
		Reasoning:  0.0,
		CacheRead:  5e-07,
		CacheWrite: 6.25e-06,
	},
	"google/gemini-3-pro-image-preview": {
		Prompt:     2e-06,
		Completion: 1.2e-05,
		Reasoning:  1.2e-05,
		CacheRead:  2e-07,
		CacheWrite: 3.75e-07,
	},
	"google/gemini-3-pro-preview": {
		Prompt:     2e-06,
		Completion: 1.2e-05,
		Reasoning:  1.2e-05,
		CacheRead:  2e-07,
		CacheWrite: 3.75e-07,
	},
	"openai/gpt-5.1": {
		Prompt:     1.25e-06,
		Completion: 1e-05,
		Reasoning:  0.0,
		CacheRead:  1.25e-07,
		CacheWrite: 0.0,
	},
	"openai/gpt-5.1-chat": {
		Prompt:     1.25e-06,
		Completion: 1e-05,
		Reasoning:  0.0,
		CacheRead:  1.25e-07,
		CacheWrite: 0.0,
	},
	"openai/gpt-5.1-codex": {
		Prompt:     1.25e-06,
		Completion: 1e-05,
		Reasoning:  0.0,
		CacheRead:  1.25e-07,
		CacheWrite: 0.0,
	},
	"openai/gpt-5.1-codex-mini": {
		Prompt:     2.5e-07,
		Completion: 2e-06,
		Reasoning:  0.0,
		CacheRead:  2.5e-08,
		CacheWrite: 0.0,
	},
	"openai/gpt-oss-safeguard-20b": {
		Prompt:     7.5e-08,
		Completion: 3e-07,
		Reasoning:  0.0,
		CacheRead:  3.7e-08,
		CacheWrite: 0.0,
	},
	"openai/gpt-5-image-mini": {
		Prompt:     2.5e-06,
		Completion: 2e-06,
		Reasoning:  0.0,
		CacheRead:  2.5e-07,
		CacheWrite: 0.0,
	},
	"anthropic/claude-haiku-4.5": {
		Prompt:     1e-06,
		Completion: 5e-06,
		Reasoning:  0.0,
		CacheRead:  1e-07,
		CacheWrite: 1.25e-06,
	},
	"openai/gpt-5-image": {
		Prompt:     1e-05,
		Completion: 1e-05,
		Reasoning:  0.0,
		CacheRead:  1.25e-06,
		CacheWrite: 0.0,
	},
	"openai/o3-deep-research": {
		Prompt:     1e-05,
		Completion: 4e-05,
		Reasoning:  0.0,
		CacheRead:  2.5e-06,
		CacheWrite: 0.0,
	},
	"openai/o4-mini-deep-research": {
		Prompt:     2e-06,
		Completion: 8e-06,
		Reasoning:  0.0,
		CacheRead:  5e-07,
		CacheWrite: 0.0,
	},
	"google/gemini-2.5-flash-image": {
		Prompt:     3e-07,
		Completion: 2.5e-06,
		Reasoning:  2.5e-06,
		CacheRead:  3e-08,
		CacheWrite: 8.333333333333334e-08,
	},
	"openai/gpt-5-pro": {
		Prompt:     1.5e-05,
		Completion: 0.00012,
		Reasoning:  0.0,
		CacheRead:  0.0,
		CacheWrite: 0.0,
	},
	"anthropic/claude-sonnet-4.5": {
		Prompt:     3e-06,
		Completion: 1.5e-05,
		Reasoning:  0.0,
		CacheRead:  3e-07,
		CacheWrite: 3.75e-06,
	},
	"google/gemini-2.5-flash-lite-preview-09-2025": {
		Prompt:     1e-07,
		Completion: 4e-07,
		Reasoning:  4e-07,
		CacheRead:  1e-08,
		CacheWrite: 8.333333333333334e-08,
	},
	"openai/gpt-5-codex": {
		Prompt:     1.25e-06,
		Completion: 1e-05,
		Reasoning:  0.0,
		CacheRead:  1.25e-07,
		CacheWrite: 0.0,
	},
	"openai/gpt-4o-audio-preview": {
		Prompt:     2.5e-06,
		Completion: 1e-05,
		Reasoning:  0.0,
		CacheRead:  0.0,
		CacheWrite: 0.0,
	},
	"openai/gpt-5-chat": {
		Prompt:     1.25e-06,
		Completion: 1e-05,
		Reasoning:  0.0,
		CacheRead:  1.25e-07,
		CacheWrite: 0.0,
	},
	"openai/gpt-5": {
		Prompt:     1.25e-06,
		Completion: 1e-05,
		Reasoning:  0.0,
		CacheRead:  1.25e-07,
		CacheWrite: 0.0,
	},
	"openai/gpt-5-mini": {
		Prompt:     2.5e-07,
		Completion: 2e-06,
		Reasoning:  0.0,
		CacheRead:  2.5e-08,
		CacheWrite: 0.0,
	},
	"openai/gpt-5-nano": {
		Prompt:     5e-08,
		Completion: 4e-07,
		Reasoning:  0.0,
		CacheRead:  5e-09,
		CacheWrite: 0.0,
	},
	"openai/gpt-oss-120b:free": {
		Prompt:     0.0,
		Completion: 0.0,
		Reasoning:  0.0,
		CacheRead:  0.0,
		CacheWrite: 0.0,
	},
	"openai/gpt-oss-120b": {
		Prompt:     3.9e-08,
		Completion: 1.9e-07,
		Reasoning:  0.0,
		CacheRead:  0.0,
		CacheWrite: 0.0,
	},
	"openai/gpt-oss-20b:free": {
		Prompt:     0.0,
		Completion: 0.0,
		Reasoning:  0.0,
		CacheRead:  0.0,
		CacheWrite: 0.0,
	},
	"openai/gpt-oss-20b": {
		Prompt:     3e-08,
		Completion: 1.4e-07,
		Reasoning:  0.0,
		CacheRead:  0.0,
		CacheWrite: 0.0,
	},
	"anthropic/claude-opus-4.1": {
		Prompt:     1.5e-05,
		Completion: 7.5e-05,
		Reasoning:  0.0,
		CacheRead:  1.5e-06,
		CacheWrite: 1.875e-05,
	},
	"google/gemini-2.5-flash-lite": {
		Prompt:     1e-07,
		Completion: 4e-07,
		Reasoning:  4e-07,
		CacheRead:  1e-08,
		CacheWrite: 8.333333333333334e-08,
	},
	"google/gemma-3n-e2b-it:free": {
		Prompt:     0.0,
		Completion: 0.0,
		Reasoning:  0.0,
		CacheRead:  0.0,
		CacheWrite: 0.0,
	},
	"google/gemini-2.5-flash": {
		Prompt:     3e-07,
		Completion: 2.5e-06,
		Reasoning:  2.5e-06,
		CacheRead:  3e-08,
		CacheWrite: 8.333333333333334e-08,
	},
	"google/gemini-2.5-pro": {
		Prompt:     1.25e-06,
		Completion: 1e-05,
		Reasoning:  1e-05,
		CacheRead:  1.25e-07,
		CacheWrite: 3.75e-07,
	},
	"openai/o3-pro": {
		Prompt:     2e-05,
		Completion: 8e-05,
		Reasoning:  0.0,
		CacheRead:  0.0,
		CacheWrite: 0.0,
	},
	"google/gemini-2.5-pro-preview": {
		Prompt:     1.25e-06,
		Completion: 1e-05,
		Reasoning:  1e-05,
		CacheRead:  1.25e-07,
		CacheWrite: 3.75e-07,
	},
	"anthropic/claude-opus-4": {
		Prompt:     1.5e-05,
		Completion: 7.5e-05,
		Reasoning:  0.0,
		CacheRead:  1.5e-06,
		CacheWrite: 1.875e-05,
	},
	"anthropic/claude-sonnet-4": {
		Prompt:     3e-06,
		Completion: 1.5e-05,
		Reasoning:  0.0,
		CacheRead:  3e-07,
		CacheWrite: 3.75e-06,
	},
	"google/gemma-3n-e4b-it:free": {
		Prompt:     0.0,
		Completion: 0.0,
		Reasoning:  0.0,
		CacheRead:  0.0,
		CacheWrite: 0.0,
	},
	"google/gemma-3n-e4b-it": {
		Prompt:     2e-08,
		Completion: 4e-08,
		Reasoning:  0.0,
		CacheRead:  0.0,
		CacheWrite: 0.0,
	},
	"google/gemini-2.5-pro-preview-05-06": {
		Prompt:     1.25e-06,
		Completion: 1e-05,
		Reasoning:  1e-05,
		CacheRead:  1.25e-07,
		CacheWrite: 3.75e-07,
	},
	"openai/o4-mini-high": {
		Prompt:     1.1e-06,
		Completion: 4.4e-06,
		Reasoning:  0.0,
		CacheRead:  2.75e-07,
		CacheWrite: 0.0,
	},
	"openai/o3": {
		Prompt:     2e-06,
		Completion: 8e-06,
		Reasoning:  0.0,
		CacheRead:  5e-07,
		CacheWrite: 0.0,
	},
	"openai/o4-mini": {
		Prompt:     1.1e-06,
		Completion: 4.4e-06,
		Reasoning:  0.0,
		CacheRead:  2.75e-07,
		CacheWrite: 0.0,
	},
	"openai/gpt-4.1": {
		Prompt:     2e-06,
		Completion: 8e-06,
		Reasoning:  0.0,
		CacheRead:  5e-07,
		CacheWrite: 0.0,
	},
	"openai/gpt-4.1-mini": {
		Prompt:     4e-07,
		Completion: 1.6e-06,
		Reasoning:  0.0,
		CacheRead:  1e-07,
		CacheWrite: 0.0,
	},
	"openai/gpt-4.1-nano": {
		Prompt:     1e-07,
		Completion: 4e-07,
		Reasoning:  0.0,
		CacheRead:  2.5e-08,
		CacheWrite: 0.0,
	},
	"openai/o1-pro": {
		Prompt:     0.00015,
		Completion: 0.0006,
		Reasoning:  0.0,
		CacheRead:  0.0,
		CacheWrite: 0.0,
	},
	"google/gemma-3-4b-it:free": {
		Prompt:     0.0,
		Completion: 0.0,
		Reasoning:  0.0,
		CacheRead:  0.0,
		CacheWrite: 0.0,
	},
	"google/gemma-3-4b-it": {
		Prompt:     4e-08,
		Completion: 8e-08,
		Reasoning:  0.0,
		CacheRead:  0.0,
		CacheWrite: 0.0,
	},
	"google/gemma-3-12b-it:free": {
		Prompt:     0.0,
		Completion: 0.0,
		Reasoning:  0.0,
		CacheRead:  0.0,
		CacheWrite: 0.0,
	},
	"google/gemma-3-12b-it": {
		Prompt:     4e-08,
		Completion: 1.3e-07,
		Reasoning:  0.0,
		CacheRead:  0.0,
		CacheWrite: 0.0,
	},
	"openai/gpt-4o-mini-search-preview": {
		Prompt:     1.5e-07,
		Completion: 6e-07,
		Reasoning:  0.0,
		CacheRead:  0.0,
		CacheWrite: 0.0,
	},
	"openai/gpt-4o-search-preview": {
		Prompt:     2.5e-06,
		Completion: 1e-05,
		Reasoning:  0.0,
		CacheRead:  0.0,
		CacheWrite: 0.0,
	},
	"google/gemma-3-27b-it:free": {
		Prompt:     0.0,
		Completion: 0.0,
		Reasoning:  0.0,
		CacheRead:  0.0,
		CacheWrite: 0.0,
	},
	"google/gemma-3-27b-it": {
		Prompt:     3e-08,
		Completion: 1.1e-07,
		Reasoning:  0.0,
		CacheRead:  1.5e-08,
		CacheWrite: 0.0,
	},
	"google/gemini-2.0-flash-lite-001": {
		Prompt:     7.5e-08,
		Completion: 3e-07,
		Reasoning:  3e-07,
		CacheRead:  0.0,
		CacheWrite: 0.0,
	},
	"anthropic/claude-3.7-sonnet": {
		Prompt:     3e-06,
		Completion: 1.5e-05,
		Reasoning:  0.0,
		CacheRead:  3e-07,
		CacheWrite: 3.75e-06,
	},
	"anthropic/claude-3.7-sonnet:thinking": {
		Prompt:     3e-06,
		Completion: 1.5e-05,
		Reasoning:  0.0,
		CacheRead:  3e-07,
		CacheWrite: 3.75e-06,
	},
	"openai/o3-mini-high": {
		Prompt:     1.1e-06,
		Completion: 4.4e-06,
		Reasoning:  0.0,
		CacheRead:  5.5e-07,
		CacheWrite: 0.0,
	},
	"google/gemini-2.0-flash-001": {
		Prompt:     1e-07,
		Completion: 4e-07,
		Reasoning:  4e-07,
		CacheRead:  2.5e-08,
		CacheWrite: 8.333333333333334e-08,
	},
	"openai/o3-mini": {
		Prompt:     1.1e-06,
		Completion: 4.4e-06,
		Reasoning:  0.0,
		CacheRead:  5.5e-07,
		CacheWrite: 0.0,
	},
	"openai/o1": {
		Prompt:     1.5e-05,
		Completion: 6e-05,
		Reasoning:  0.0,
		CacheRead:  7.5e-06,
		CacheWrite: 0.0,
	},
	"openai/gpt-4o-2024-11-20": {
		Prompt:     2.5e-06,
		Completion: 1e-05,
		Reasoning:  0.0,
		CacheRead:  1.25e-06,
		CacheWrite: 0.0,
	},
	"anthropic/claude-3.5-haiku": {
		Prompt:     8e-07,
		Completion: 4e-06,
		Reasoning:  0.0,
		CacheRead:  8e-08,
		CacheWrite: 1e-06,
	},
	"anthropic/claude-3.5-sonnet": {
		Prompt:     6e-06,
		Completion: 3e-05,
		Reasoning:  0.0,
		CacheRead:  6e-07,
		CacheWrite: 7.5e-06,
	},
	"google/gemini-2.0-flash": {Prompt: 0.0000001, Completion: 0.0000004, Reasoning: 0.0000004},
	"gemini-3-flash-preview": {Prompt: 0.0000001, Completion: 0.0000004, Reasoning: 0.0000004},
	"google/gemini-1.5-flash": {Prompt: 0.000000075, Completion: 0.0000003},
	"google/gemini-1.5-pro":   {Prompt: 0.00000125, Completion: 0.000005},
}

// GetPricing returns the pricing for a given model ID.
func GetPricing(modelID string) ModelPricing {
	id := strings.ToLower(modelID)
	if strings.HasPrefix(id, "models/") {
		id = id[7:]
	}
	
	// Direct match
	if p, ok := ModelPricingTable[id]; ok {
		return p
	}
	
	// Match with google/ prefix
	if !strings.Contains(id, "/") {
		if p, ok := ModelPricingTable["google/"+id]; ok {
			return p
		}
	}

	// Reverse match (google/gemini-2.0-flash-001 -> google/gemini-2.0-flash)
	for k, p := range ModelPricingTable {
		if strings.HasPrefix(id, k) || strings.HasPrefix(k, id) {
			return p
		}
	}
	
	return ModelPricing{}
}

// CalculateCost computes the total cost for a usage breakdown.
func CalculateCost(modelID string, usage Usage) float64 {
	p := GetPricing(modelID)

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
