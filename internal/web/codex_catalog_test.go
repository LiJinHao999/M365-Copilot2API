package web

import (
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestModelTokenLimitsAreConsistent(t *testing.T) {
	t.Setenv("M365_CONTEXT_WINDOW", "128000")
	t.Setenv("M365_MAX_OUTPUT_TOKENS", "16384")
	l := configuredModelLimits()
	if l.ContextWindow != 128000 || l.MaxOutputTokens != 16384 || l.MaxInputTokens != 111616 {
		t.Fatalf("limits=%+v", l)
	}
}

func TestModelTokenLimitsNormalizeBadOutputLimit(t *testing.T) {
	t.Setenv("M365_CONTEXT_WINDOW", "100")
	t.Setenv("M365_MAX_OUTPUT_TOKENS", "500")
	l := configuredModelLimits()
	if l.MaxInputTokens <= 0 || l.MaxOutputTokens <= 0 || l.MaxInputTokens+l.MaxOutputTokens != l.ContextWindow {
		t.Fatalf("inconsistent limits=%+v", l)
	}
}

func TestModelsAdvertiseContextAndReasoning(t *testing.T) {
	s := &Server{}
	r := httptest.NewRequest("GET", "/v1/models", nil)
	w := httptest.NewRecorder()
	s.openaiModels(w, r)
	var body struct {
		Data   []map[string]any `json:"data"`
		Models []map[string]any `json:"models"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Data) == 0 {
		t.Fatal("empty model catalog")
	}
	if len(body.Models) != len(body.Data) {
		t.Fatalf("models alias length=%d, data length=%d", len(body.Models), len(body.Data))
	}
	for _, m := range body.Data {
		id, _ := m["id"].(string)
		if isContinuousModelAlias(id) || isRawUpstreamToneID(id) {
			t.Fatalf("public catalog must not list continuous aliases or raw tones: %q", id)
		}
		baseInstructions, ok := m["base_instructions"].(string)
		if !ok || baseInstructions == "" {
			t.Fatalf("missing Codex base instructions: %#v", m)
		}
		modelMessages, ok := m["model_messages"].(map[string]any)
		if !ok || modelMessages["instructions_template"] != baseInstructions {
			t.Fatalf("missing or inconsistent Codex model messages: %#v", m)
		}
		variables, ok := modelMessages["instructions_variables"].(map[string]any)
		if !ok || variables["personality_default"] != "" || variables["personality_friendly"] != "" || variables["personality_pragmatic"] != "" {
			t.Fatalf("invalid Codex instruction variables: %#v", modelMessages)
		}
		if modelMessages["approvals"] != nil || modelMessages["auto_review"] != nil {
			t.Fatalf("invalid optional Codex model messages: %#v", modelMessages)
		}
		if m["slug"] != m["id"] {
			t.Fatalf("missing or inconsistent slug: %#v", m)
		}
		if displayName, ok := m["display_name"].(string); !ok || displayName == "" {
			t.Fatalf("missing display_name: %#v", m)
		}
		levels, ok := m["supported_reasoning_levels"].([]any)
		if !ok || len(levels) == 0 {
			t.Fatalf("missing supported reasoning levels: %#v", m)
		}
		for _, level := range levels {
			preset, ok := level.(map[string]any)
			if !ok || preset["effort"] == "" || preset["description"] == "" {
				t.Fatalf("invalid reasoning preset: %#v", level)
			}
		}
		defaultReasoningLevel, ok := m["default_reasoning_level"].(string)
		if effort, err := normalizeReasoningEffort(defaultReasoningLevel); !ok || err != nil || effort == "" || m["description"] == "" {
			t.Fatalf("missing Codex catalog metadata: %#v", m)
		}
		if m["shell_type"] != "shell_command" || m["visibility"] != "list" || m["supported_in_api"] != true || m["priority"] != float64(1) {
			t.Fatalf("missing Codex execution metadata: %#v", m)
		}
		if _, ok := m["additional_speed_tiers"].([]any); !ok {
			t.Fatalf("missing speed tiers: %#v", m)
		}
		if _, ok := m["service_tiers"].([]any); !ok {
			t.Fatalf("missing service tiers: %#v", m)
		}
		if m["apply_patch_tool_type"] != "freeform" || m["web_search_tool_type"] != "text_and_image" || m["tool_mode"] != "code_mode_only" || m["multi_agent_version"] != "v2" {
			t.Fatalf("missing Codex tool metadata: %#v", m)
		}
		if m["max_context_window"] != m["context_window"] || m["effective_context_window_percent"] != float64(95) {
			t.Fatalf("inconsistent Codex context metadata: %#v", m)
		}
		policy, ok := m["truncation_policy"].(map[string]any)
		if !ok || policy["mode"] != "tokens" || policy["limit"] != float64(10000) {
			t.Fatalf("missing truncation policy: %#v", m)
		}
		if _, ok := m["experimental_supported_tools"].([]any); !ok || m["supports_search_tool"] != true || m["use_responses_lite"] != false {
			t.Fatalf("missing Codex capability metadata: %#v", m)
		}
		if m["context_window"].(float64) <= 0 || m["max_input_tokens"].(float64) <= 0 || m["max_output_tokens"].(float64) <= 0 {
			t.Fatalf("missing limits: %#v", m)
		}
		caps, ok := m["capabilities"].(map[string]any)
		if !ok {
			t.Fatalf("missing capabilities: %#v", m)
		}
		if caps["reasoning"] != true {
			t.Fatalf("reasoning not advertised: %#v", m)
		}
		if levels, ok := caps["supported_reasoning_levels"].([]any); !ok || len(levels) == 0 {
			t.Fatalf("capabilities missing supported reasoning levels: %#v", m)
		}
		if efforts, ok := caps["reasoning_efforts"].([]any); !ok || len(efforts) == 0 {
			t.Fatalf("capabilities missing object reasoning efforts: %#v", m)
		} else if _, ok := efforts[0].(map[string]any); !ok {
			t.Fatalf("reasoning efforts must be preset objects: %#v", efforts)
		}
	}
	for i, m := range body.Models {
		if m["slug"] != body.Data[i]["slug"] {
			t.Fatalf("models alias missing slug at %d: %#v", i, m)
		}
		if m["display_name"] != body.Data[i]["display_name"] {
			t.Fatalf("models alias missing display_name at %d: %#v", i, m)
		}
		if m["supported_reasoning_levels"] == nil {
			t.Fatalf("models alias missing supported reasoning levels at %d: %#v", i, m)
		}
		if m["base_instructions"] != body.Data[i]["base_instructions"] || m["model_messages"] == nil {
			t.Fatalf("models alias missing instruction metadata at %d: %#v", i, m)
		}
	}
}

func TestConfiguredModelMappingsDriveCatalogAndRouting(t *testing.T) {
	// Catalog is driven only by the provided mappings (WebUI source of truth).
	mappings := []modelMapping{
		{PublicModel: "gpt-5.6-sol", UpstreamTone: "Gpt_5_6_Reasoning", DisplayName: "GPT-5.6-Sol", DefaultReasoningLevel: "low"},
		{PublicModel: "claude-fable-5", UpstreamTone: "Claude_Fable", DisplayName: "Claude Fable 5", DefaultReasoningLevel: "medium"},
	}
	models := configuredModelSpecs(mappings)
	if len(models) != 2 || models[0].ID != "gpt-5.6-sol" || models[1].ID != "claude-fable-5" {
		t.Fatalf("configured models=%#v", models)
	}
	mapping, ok := configuredModelMapping("GPT-5.6-SOL", mappings)
	if !ok || mapping.UpstreamTone != "Gpt_5_6_Reasoning" {
		t.Fatalf("mapping=%#v ok=%t", mapping, ok)
	}
	if tone, ok := configuredModelTone("gpt-5.6-sol", mappings); !ok || tone != "Gpt_5_6_Reasoning" {
		t.Fatalf("tone=%q ok=%t", tone, ok)
	}
	// Continuous aliases and raw tones must not appear in the public catalog.
	filtered := configuredModelSpecs([]modelMapping{
		{PublicModel: "claude-fable-5-持续", UpstreamTone: "Claude_Fable", DisplayName: "Fable Cont", DefaultReasoningLevel: "medium"},
		{PublicModel: "Claude_Fable", UpstreamTone: "Claude_Fable", DisplayName: "Raw Tone", DefaultReasoningLevel: "medium"},
		{PublicModel: "my-custom-model", UpstreamTone: "Claude_Fable", DisplayName: "Custom", DefaultReasoningLevel: "medium"},
	})
	if len(filtered) != 1 || filtered[0].ID != "my-custom-model" {
		t.Fatalf("filtered=%#v", filtered)
	}
	// Empty input falls back to the product seed list.
	seeded := configuredModelSpecs(nil)
	if len(seeded) != len(defaultModelMappings) {
		t.Fatalf("seed len=%d want=%d", len(seeded), len(defaultModelMappings))
	}
}

func TestConfiguredModelSpecsSkipsEmptyPublicModel(t *testing.T) {
	models := configuredModelSpecs([]modelMapping{
		{PublicModel: "", UpstreamTone: "Claude_Fable", DisplayName: "bad", DefaultReasoningLevel: "medium"},
		{PublicModel: "   ", UpstreamTone: "Claude_Fable", DisplayName: "bad2", DefaultReasoningLevel: "medium"},
		{PublicModel: "claude-fable-custom", UpstreamTone: "Claude_Fable", DisplayName: "Fable Custom", DefaultReasoningLevel: "medium"},
	})
	if len(models) != 1 || models[0].ID != "claude-fable-custom" {
		t.Fatalf("models=%#v", models)
	}
}

func TestModelToneSupportsFableAndProductAliases(t *testing.T) {
	cases := map[string]string{
		"claude-fable-5":                 "Claude_Fable",
		"claude-fable-5-持续":              "Claude_Fable",
		"Claude_Fable":                   "Claude_Fable",
		"claude-sonnet-4-6":              "Claude_Sonnet",
		"claude-sonnet-4-6-持续":           "Claude_Sonnet",
		"claude-sonnet-4-5_Reasoning":    "Claude_Sonnet_Reasoning",
		"claude-sonnet-4-5_Reasoning-持续": "Claude_Sonnet_Reasoning",
		"gpt-5.5_Chat":                   "Gpt_5_5_Chat",
		"gpt-5.5_Chat-持续":                "Gpt_5_5_Chat",
		"gpt-5.6_Reasoning":              "Gpt_5_6_Reasoning",
		"Copilot_自动":                     "Magic",
		"Copilot_自动-持续":                  "Magic",
		"Copilot_快速答复":                   "Chat",
		"Copilot_深度思考":                   "Reasoning",
		"Magic":                          "Magic",
		"Chat":                           "Chat",
		"Reasoning":                      "Reasoning",
	}
	for model, want := range cases {
		if got := modelTone(model); got != want {
			t.Fatalf("modelTone(%q)=%q want %q", model, got, want)
		}
	}
}

func TestOpenAIModelsOmitsEmptyIDs(t *testing.T) {
	// Simulate a corrupted in-memory settings row with a blank public model.
	st := &settingsStore{
		path: filepath.Join(t.TempDir(), "settings.json"),
		v: runtimeSettings{
			MaxToolCallsPerTurn: 1,
			MaxToolRounds:       16,
			ContextWindow:       128000,
			MaxOutputTokens:     16384,
			ChatTimeoutSeconds:  120,
			ImageTimeoutSeconds: 150,
			LogLevel:            "info",
			ModelMappings: []modelMapping{
				{PublicModel: "", UpstreamTone: "Claude_Fable", DisplayName: "blank", DefaultReasoningLevel: "medium"},
				{PublicModel: "claude-fable-ok", UpstreamTone: "Claude_Fable", DisplayName: "Fable OK", DefaultReasoningLevel: "medium"},
			},
			ToolPlanningMode: "router",
		},
	}
	prev := sharedSettings
	sharedSettings = st
	t.Cleanup(func() { sharedSettings = prev })

	s := &Server{settings: st}
	r := httptest.NewRequest("GET", "/v1/models", nil)
	w := httptest.NewRecorder()
	s.openaiModels(w, r)
	var body struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Data) == 0 {
		t.Fatal("empty catalog")
	}
	foundCustom := false
	for _, m := range body.Data {
		id, _ := m["id"].(string)
		if strings.TrimSpace(id) == "" {
			t.Fatalf("empty id in catalog: %#v", m)
		}
		if id == "claude-fable-ok" {
			foundCustom = true
		}
	}
	if !foundCustom {
		t.Fatal("expected claude-fable-ok in catalog")
	}
}

func TestReasoningEffortRouting(t *testing.T) {
	cases := []struct{ model, effort, want string }{
		{"claude-sonnet", "none", "Claude_Sonnet"},
		{"claude-sonnet", "high", "Claude_Sonnet_Reasoning"},
		{"claude-fable-5", "high", "Claude_Fable"},
		{"gpt-5.5", "low", "Gpt_5_5_Chat"},
		{"gpt-5.5", "medium", "Gpt_5_5_Reasoning"},
		{"gpt-5.6-reasoning", "none", "Gpt_5_6_Reasoning"},
		{"Copilot_自动", "high", "Reasoning"},
		{"claude-sonnet-4-6-持续", "none", "Claude_Sonnet"},
	}
	for _, tc := range cases {
		got, err := reasoningTone(tc.model, tc.effort)
		if err != nil || got != tc.want {
			t.Fatalf("%s/%s got=%q err=%v", tc.model, tc.effort, got, err)
		}
	}
	if _, err := reasoningTone("gpt-5.6-reasoning", "extreme"); err == nil {
		t.Fatal("invalid effort accepted")
	}
}

func TestChatRejectsInvalidReasoningBeforeUpstream(t *testing.T) {
	s := &Server{}
	r := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"gpt-5.6-reasoning","reasoning_effort":"extreme","messages":[{"role":"user","content":"hello"}]}`))
	w := httptest.NewRecorder()
	s.openaiChat(w, r)
	if w.Code != 400 || !strings.Contains(w.Body.String(), "unsupported reasoning effort") {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestResponsesReasoningConvertsToOpenAI(t *testing.T) {
	r := responsesRequest{Model: "gpt-5.6-reasoning", Input: "hello", Reasoning: &reasoningConfig{Effort: "high"}}
	o, err := r.openAI()
	if err != nil {
		t.Fatal(err)
	}
	if o.ReasoningEffort != "high" {
		t.Fatalf("effort=%q", o.ReasoningEffort)
	}
}
