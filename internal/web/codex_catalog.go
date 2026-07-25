// Codex model catalog compatibility lives here. It is intentionally kept in
// package web because route handlers share unexported request and settings types.
package web

import (
	"fmt"
	"os"
	"strconv"
	"m365-native/internal/chathub"
	"strings"
)

type modelLimits struct{ ContextWindow, MaxInputTokens, MaxOutputTokens int }
type reasoningConfig struct {
	Effort  string `json:"effort,omitempty"`
	Summary string `json:"summary,omitempty"`
}

type modelSpec struct {
	ID, Owner, DisplayName, DefaultReasoningLevel string
	Tools                                         bool
}

type reasoningEffortPreset struct {
	Effort      string `json:"effort"`
	Description string `json:"description"`
}

var advertisedReasoningEfforts = []reasoningEffortPreset{
	{Effort: "none", Description: "Disable additional reasoning."},
	{Effort: "minimal", Description: "Fast responses with minimal reasoning."},
	{Effort: "low", Description: "Fast responses with lighter reasoning."},
	{Effort: "medium", Description: "Balances speed and reasoning depth for everyday tasks."},
	{Effort: "high", Description: "Greater reasoning depth for complex problems."},
	{Effort: "xhigh", Description: "Extra high reasoning depth for complex problems."},
}

// gatewayCodexBaseInstructions is returned only in the Codex model catalog.
// Codex uses it to build its own request instructions; it is not interpreted
// or forwarded directly by the gateway's ChatHub adapter.
const gatewayCodexBaseInstructions = `You are Codex, a coding agent collaborating with the user in their workspace. Follow the user's request, inspect the repository before making changes, preserve unrelated work, and verify changes proportionately. Keep responses clear, concise, and grounded in observed evidence.`

func codexModelMessages() map[string]any {
	return map[string]any{
		"instructions_template": gatewayCodexBaseInstructions,
		"instructions_variables": map[string]string{
			"personality_default":   "",
			"personality_friendly":  "",
			"personality_pragmatic": "",
		},
		"approvals":   nil,
		"auto_review": nil,
	}
}

// gatewayModels is retained only as a derived view of defaultModelMappings for
// tests that still reference the symbol. The live /v1/models catalog is built
// exclusively from settings.ModelMappings (WebUI-editable, hot-reloaded).
var gatewayModels = func() []modelSpec {
	out := make([]modelSpec, 0, len(defaultModelMappings))
	for _, m := range defaultModelMappings {
		out = append(out, modelSpec{
			ID: m.PublicModel, Owner: modelOwnerForID(m.PublicModel),
			DisplayName: m.DisplayName, DefaultReasoningLevel: m.DefaultReasoningLevel, Tools: true,
		})
	}
	return out
}()

func modelOwnerForID(id string) string {
	lower := strings.ToLower(strings.TrimSpace(id))
	if strings.Contains(lower, "claude") || strings.Contains(lower, "fable") {
		return "anthropic-via-microsoft-365"
	}
	return "microsoft-365"
}

func validUpstreamTone(tone string) bool {
	tone = strings.TrimSpace(tone)
	for _, known := range knownUpstreamTones() {
		if tone == known {
			return true
		}
	}
	// Allow free-form custom tones (new ChatHub modes) so admins can map them
	// from the WebUI without shipping a code change.
	return chathub.ValidCustomToneID(tone)
}

func knownUpstreamTones() []string {
	return []string{
		"Magic", "Chat", "Reasoning",
		"Gpt_5_2_Chat", "Gpt_5_2_Reasoning",
		"Gpt_5_3_Chat", "Gpt_5_3_Reasoning",
		"Gpt_5_4_Chat", "Gpt_5_4_Reasoning",
		"Gpt_5_5_Chat", "Gpt_5_5_Reasoning",
		"Gpt_5_6_Reasoning",
		"Gpt_Quick", "Gpt_Reasoning",
		"Claude_Sonnet", "Claude_Sonnet_Reasoning", "Claude_Fable",
		// legacy lowercase defaults still accepted from older clients/settings
		"magic",
	}
}

// publicUpstreamTones is the WebUI dropdown list — no legacy lowercase noise.
func publicUpstreamTones() []string {
	out := make([]string, 0, len(knownUpstreamTones()))
	for _, t := range knownUpstreamTones() {
		if t == "magic" {
			continue
		}
		out = append(out, t)
	}
	return out
}

func configuredModelMapping(model string, mappings []modelMapping) (modelMapping, bool) {
	want := strings.ToLower(stripContinuousModelSuffix(strings.TrimSpace(model)))
	for _, mapping := range mappings {
		if strings.EqualFold(strings.TrimSpace(mapping.PublicModel), want) ||
			strings.EqualFold(strings.TrimSpace(mapping.PublicModel), strings.TrimSpace(model)) {
			return mapping, true
		}
	}
	return modelMapping{}, false
}

func configuredModelTone(model string, mappings []modelMapping) (string, bool) {
	mapping, ok := configuredModelMapping(model, mappings)
	if !ok {
		return "", false
	}
	return mapping.UpstreamTone, true
}

// configuredModelSpecs builds the public catalog solely from saved mappings.
// Hardcoded product IDs are no longer merged in — add/remove models via WebUI.
func configuredModelSpecs(mappings []modelMapping) []modelSpec {
	if len(mappings) == 0 {
		mappings = defaultModelMappings
	}
	models := make([]modelSpec, 0, len(mappings))
	seen := map[string]struct{}{}
	for _, mapping := range mappings {
		id := strings.TrimSpace(mapping.PublicModel)
		if id == "" {
			continue
		}
		// Keep /v1/models free of session aliases and raw ChatHub tones.
		if isContinuousModelAlias(id) || isRawUpstreamToneID(id) {
			continue
		}
		key := strings.ToLower(id)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		display := strings.TrimSpace(mapping.DisplayName)
		if display == "" {
			display = id
		}
		level := strings.TrimSpace(mapping.DefaultReasoningLevel)
		if level == "" {
			level = "medium"
		}
		models = append(models, modelSpec{
			ID: id, Owner: modelOwnerForID(id), Tools: true,
			DisplayName: display, DefaultReasoningLevel: level,
		})
	}
	return models
}

func positiveEnvInt(name string, fallback int) int {
	v, err := strconv.Atoi(strings.TrimSpace(os.Getenv(name)))
	if err == nil && v > 0 {
		return v
	}
	return fallback
}
func configuredModelLimits() modelLimits {
	cfg := currentSettings()
	contextWindow := cfg.ContextWindow
	maxOutput := cfg.MaxOutputTokens
	if maxOutput >= contextWindow {
		maxOutput = contextWindow / 8
		if maxOutput < 1 {
			maxOutput = 1
		}
	}
	return modelLimits{ContextWindow: contextWindow, MaxInputTokens: contextWindow - maxOutput, MaxOutputTokens: maxOutput}
}
func normalizeReasoningEffort(e string) (string, error) {
	e = strings.ToLower(strings.TrimSpace(e))
	if e == "" {
		return "", nil
	}
	switch e {
	case "none", "minimal", "low", "medium", "high", "xhigh":
		return e, nil
	}
	return "", fmt.Errorf("unsupported reasoning effort %q; use none, minimal, low, medium, high, or xhigh", e)
}
func reasoningTone(model, effort string) (string, error) {
	e, err := normalizeReasoningEffort(effort)
	if err != nil {
		return "", err
	}
	if tone, ok := configuredModelTone(model, currentSettings().ModelMappings); ok {
		// Saved mappings pin the base tone, but medium+ effort may still lift
		// product auto/fast routes onto deep-think without a separate model id.
		if (tone == "Magic" || tone == "Chat") && e != "" && e != "none" && e != "minimal" && e != "low" {
			return "Reasoning", nil
		}
		return tone, nil
	}
	base := modelTone(model)
	rawModel := strings.TrimSpace(model)
	normalized := strings.ToLower(stripContinuousModelSuffix(rawModel))
	// Explicit reasoning / fable routes are never silently downgraded by a client default.
	if strings.Contains(normalized, "reasoning") ||
		strings.Contains(rawModel, "深度思考") || strings.Contains(normalized, "深度思考") ||
		normalized == "claude-fable" || normalized == "claude-fable-5" ||
		normalized == "claude-fable5" || base == "Claude_Fable" || base == "Reasoning" {
		return base, nil
	}
	if e == "" || e == "none" || e == "minimal" || e == "low" {
		return base, nil
	}
	// Medium+ effort upgrades chat/auto product routes onto the deep-think tone.
	if base == "Magic" || base == "Chat" {
		return "Reasoning", nil
	}
	switch normalized {
	case "claude", "claude-sonnet", "claude-sonnet-4-6", "claude-sonnet-4.6", "claude-sonnet4.6":
		return "Claude_Sonnet_Reasoning", nil
	case "gpt-5.2", "gpt-5.2_chat", "gpt-5.2-chat":
		return "Gpt_5_2_Reasoning", nil
	case "gpt-5.3":
		return "Gpt_5_3_Reasoning", nil
	case "gpt-5.4":
		return "Gpt_5_4_Reasoning", nil
	case "gpt-5.5", "gpt-5.5_chat", "gpt-5.5-chat":
		return "Gpt_5_5_Reasoning", nil
	case "gpt-5.6":
		return "Gpt_5_6_Reasoning", nil
	default:
		return "Gpt_Reasoning", nil
	}
}
func modelCatalog() []map[string]any {
	l := configuredModelLimits()
	models := configuredModelSpecs(currentSettings().ModelMappings)
	out := make([]map[string]any, 0, len(models))
	for _, m := range models {
		id := strings.TrimSpace(m.ID)
		if id == "" {
			continue
		}
		// Keep capability fields both at the top level and under capabilities:
		// different OpenAI-compatible clients inspect different locations.
		features := []string{"tools", "function_calling", "streaming", "reasoning", "vision"}
		modalities := []string{"text", "image"}
		caps := map[string]any{
			"chat_completions": true, "responses": true, "streaming": true,
			"tools": true, "reasoning": true,
			"reasoning_efforts": advertisedReasoningEfforts, "supported_reasoning_levels": advertisedReasoningEfforts,
			"reasoning_mode": "gateway_tone_routing", "supports_tools": true, "tool_calls": true,
			"function_calling": true, "supports_function_calling": true, "supports_vision": true,
			"vision": true, "modalities": modalities, "input_modalities": modalities,
			"output_modalities": []string{"text"}, "supported_features": features,
		}
		displayName := strings.TrimSpace(m.DisplayName)
		if displayName == "" {
			displayName = id
		}
		defaultReasoningLevel := m.DefaultReasoningLevel
		if defaultReasoningLevel == "" {
			defaultReasoningLevel = "medium"
		}
		owner := strings.TrimSpace(m.Owner)
		if owner == "" {
			owner = "microsoft-365"
		}
		out = append(out, map[string]any{
			"id": id, "slug": id, "display_name": displayName, "description": "Microsoft 365 gateway model route.",
			"base_instructions": gatewayCodexBaseInstructions, "model_messages": codexModelMessages(),
			"default_reasoning_level": defaultReasoningLevel, "object": "model", "owned_by": owner,
			"shell_type": "shell_command", "visibility": "list", "supported_in_api": true, "priority": 1,
			"additional_speed_tiers": []string{}, "service_tiers": []any{},
			"availability_nux": nil, "upgrade": nil, "include_skills_usage_instructions": false,
			"supports_reasoning_summaries": true, "default_reasoning_summary": "none",
			"support_verbosity": true, "default_verbosity": "low", "apply_patch_tool_type": "freeform",
			"web_search_tool_type": "text_and_image", "truncation_policy": map[string]any{"mode": "tokens", "limit": 10000},
			"supports_parallel_tool_calls": true, "supports_image_detail_original": true,
			"max_context_window": l.ContextWindow, "effective_context_window_percent": 95,
			"experimental_supported_tools": []any{}, "supports_search_tool": true, "use_responses_lite": false,
			"tool_mode": "code_mode_only", "multi_agent_version": "v2",
			"context_window": l.ContextWindow, "max_input_tokens": l.MaxInputTokens, "max_output_tokens": l.MaxOutputTokens,
			"capabilities": caps, "supports_tools": true, "tool_calls": true,
			"supported_reasoning_levels": advertisedReasoningEfforts,
			"function_calling":           true, "supports_function_calling": true, "supports_vision": true,
			"vision": true, "modalities": modalities, "input_modalities": modalities,
			"output_modalities": []string{"text"}, "supported_features": features,
		})
	}
	return out
}
