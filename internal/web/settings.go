package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"m365-native/internal/outbound"
)

type modelMapping struct {
	PublicModel           string `json:"publicModel"`
	UpstreamTone          string `json:"upstreamTone"`
	DisplayName           string `json:"displayName"`
	DefaultReasoningLevel string `json:"defaultReasoningLevel"`
}

// defaultModelMappings is the seed / reset catalog shown in WebUI and used when
// settings.json has no modelMappings yet. /v1/models is driven entirely by the
// saved ModelMappings list — edit it in the admin UI, no image rebuild required.
var defaultModelMappings = []modelMapping{
	{PublicModel: "Copilot_自动", UpstreamTone: "Magic", DisplayName: "Copilot 自动", DefaultReasoningLevel: "medium"},
	{PublicModel: "Copilot_快速答复", UpstreamTone: "Chat", DisplayName: "Copilot 快速答复", DefaultReasoningLevel: "low"},
	{PublicModel: "Copilot_深度思考", UpstreamTone: "Reasoning", DisplayName: "Copilot 深度思考", DefaultReasoningLevel: "high"},
	{PublicModel: "claude-sonnet-4-6", UpstreamTone: "Claude_Sonnet", DisplayName: "Claude Sonnet 4.6", DefaultReasoningLevel: "medium"},
	{PublicModel: "claude-sonnet-4-5_Reasoning", UpstreamTone: "Claude_Sonnet_Reasoning", DisplayName: "Claude Sonnet Reasoning", DefaultReasoningLevel: "high"},
	{PublicModel: "claude-fable-5", UpstreamTone: "Claude_Fable", DisplayName: "Claude Fable 5", DefaultReasoningLevel: "medium"},
	{PublicModel: "gpt-5.6_Reasoning", UpstreamTone: "Gpt_5_6_Reasoning", DisplayName: "GPT-5.6 Reasoning", DefaultReasoningLevel: "high"},
	{PublicModel: "gpt-5.5_Chat", UpstreamTone: "Gpt_5_5_Chat", DisplayName: "GPT-5.5 Chat", DefaultReasoningLevel: "low"},
	{PublicModel: "gpt-5.5_Reasoning", UpstreamTone: "Gpt_5_5_Reasoning", DisplayName: "GPT-5.5 Reasoning", DefaultReasoningLevel: "high"},
	{PublicModel: "gpt-5.2_Chat", UpstreamTone: "Gpt_5_2_Chat", DisplayName: "GPT-5.2 Chat", DefaultReasoningLevel: "low"},
	{PublicModel: "gpt-5.2_Reasoning", UpstreamTone: "Gpt_5_2_Reasoning", DisplayName: "GPT-5.2 Reasoning", DefaultReasoningLevel: "high"},
	// Optional Codex-friendly aliases kept as extras (same tones).
	{PublicModel: "gpt-5.6-sol", UpstreamTone: "Gpt_5_6_Reasoning", DisplayName: "GPT-5.6-Sol", DefaultReasoningLevel: "low"},
	{PublicModel: "gpt-5.6-terra", UpstreamTone: "Gpt_5_6_Reasoning", DisplayName: "GPT-5.6-Terra", DefaultReasoningLevel: "medium"},
	{PublicModel: "gpt-5.6-luna", UpstreamTone: "Gpt_5_6_Reasoning", DisplayName: "GPT-5.6-Luna", DefaultReasoningLevel: "medium"},
}

// Allow ASCII plus common CJK product labels (e.g. Copilot_自动) used as public IDs.
var publicModelID = regexp.MustCompile(`^[\p{L}\p{N}._-]{1,128}$`)

// Suggested public model IDs for the WebUI datalist (not a hard allowlist).
var configurableCodexModels = []string{
	"Copilot_自动",
	"Copilot_快速答复",
	"Copilot_深度思考",
	"claude-sonnet-4-6",
	"claude-sonnet-4-5_Reasoning",
	"claude-fable-5",
	"gpt-5.6_Reasoning",
	"gpt-5.5_Chat",
	"gpt-5.5_Reasoning",
	"gpt-5.2_Chat",
	"gpt-5.2_Reasoning",
	"gpt-5.6-sol",
	"gpt-5.6-terra",
	"gpt-5.6-luna",
	"codex-auto-review",
}

type runtimeSettings struct {
	MaxToolCallsPerTurn int            `json:"maxToolCallsPerTurn"`
	MaxToolRounds       int            `json:"maxToolRounds"`
	ContextWindow       int            `json:"contextWindow"`
	MaxOutputTokens     int            `json:"maxOutputTokens"`
	ChatTimeoutSeconds  int            `json:"chatTimeoutSeconds"`
	ImageTimeoutSeconds int            `json:"imageTimeoutSeconds"`
	LogLevel            string         `json:"logLevel"`
	DebugLogPath        string         `json:"debugLogPath"`
	ListenAddress       string         `json:"listenAddress"`
	ConfigPath          string         `json:"configPath"`
	TokenCachePath      string         `json:"tokenCachePath"`
	SessionCachePath    string         `json:"sessionCachePath"`
	OutboundProxy       string         `json:"outboundProxy"`
	ProxyPool           []string       `json:"proxyPool,omitempty"`
	ClientID            string         `json:"clientId"`
	Authority           string         `json:"authority"`
	RedirectURI         string         `json:"redirectUri"`
	Scope               string         `json:"scope"`
	ModelMappings       []modelMapping `json:"modelMappings"`
	ToolPlanningMode    string         `json:"toolPlanningMode"`
}

type settingsStore struct {
	mu   sync.RWMutex
	path string
	v    runtimeSettings
}

func envInt(name string, fallback int) int {
	n, e := strconv.Atoi(strings.TrimSpace(os.Getenv(name)))
	if e == nil && n > 0 {
		return n
	}
	return fallback
}
func defaultRuntimeSettings() runtimeSettings {
	return runtimeSettings{
		MaxToolCallsPerTurn: envInt("M365_MAX_TOOL_CALLS_PER_TURN", 1), MaxToolRounds: envInt("M365_MAX_TOOL_ROUNDS", 16),
		ContextWindow: envInt("M365_CONTEXT_WINDOW", 128000), MaxOutputTokens: envInt("M365_MAX_OUTPUT_TOKENS", 16384),
		ChatTimeoutSeconds: envInt("M365_CHAT_TIMEOUT_SECONDS", 120), ImageTimeoutSeconds: envInt("M365_IMAGE_TIMEOUT_SECONDS", 150), LogLevel: firstNonEmptySetting(os.Getenv("M365_LOG_LEVEL"), "info"),
		DebugLogPath: os.Getenv("M365_DEBUG_LOG"), ListenAddress: os.Getenv("M365_LISTEN"), ConfigPath: os.Getenv("M365_CONFIG"),
		TokenCachePath: os.Getenv("M365_TOKEN_CACHE"), SessionCachePath: os.Getenv("M365_SESSION_CACHE"), OutboundProxy: os.Getenv(outbound.EnvProxy), ClientID: os.Getenv("M365_CLIENT_ID"),
		Authority: os.Getenv("M365_AUTHORITY"), RedirectURI: os.Getenv("M365_REDIRECT_URI"), Scope: os.Getenv("M365_SCOPE"),
		ModelMappings:    append([]modelMapping(nil), defaultModelMappings...),
		ToolPlanningMode: toolPlanningMode(os.Getenv("M365_TOOL_PLANNING_MODE")),
	}
}
func settingsPath() string {
	if dir := strings.TrimSpace(os.Getenv("M365_DATA_DIR")); dir != "" {
		return filepath.Join(dir, "settings.json")
	}
	if p := strings.TrimSpace(os.Getenv("M365_SETTINGS_FILE")); p != "" {
		return p
	}
	h, _ := os.UserHomeDir()
	return filepath.Join(h, ".config", "m365-native", "settings.json")
}

var sharedSettings *settingsStore

func openSettingsStore() *settingsStore {
	if sharedSettings != nil {
		return sharedSettings
	}
	s := &settingsStore{path: settingsPath(), v: defaultRuntimeSettings()}
	loadedFromDisk := false
	if b, e := os.ReadFile(s.path); e == nil {
		_ = json.Unmarshal(b, &s.v)
		loadedFromDisk = true
	}
	s.v.ModelMappings = sanitizeModelMappings(s.v.ModelMappings)
	// Empty catalog (fresh install or wiped rows) falls back to the product seed.
	// Once the admin saves a custom list, that list is authoritative — including
	// deletions — until they click “恢复默认映射” in the WebUI.
	if len(s.v.ModelMappings) == 0 {
		s.v.ModelMappings = append([]modelMapping(nil), defaultModelMappings...)
		if loadedFromDisk {
			_ = s.save(s.v)
		}
	}
	_ = validateSettings(s.v)
	sharedSettings = s
	return s
}

// sanitizeModelMappings drops blank/partial rows that would otherwise surface as
// empty model ids in /v1/models.
func sanitizeModelMappings(in []modelMapping) []modelMapping {
	if len(in) == 0 {
		return in
	}
	out := make([]modelMapping, 0, len(in))
	seen := map[string]struct{}{}
	for _, m := range in {
		id := strings.TrimSpace(m.PublicModel)
		if id == "" {
			continue
		}
		key := strings.ToLower(id)
		if _, ok := seen[key]; ok {
			continue
		}
		if strings.TrimSpace(m.UpstreamTone) == "" || strings.TrimSpace(m.DisplayName) == "" {
			continue
		}
		if _, err := normalizeReasoningEffort(m.DefaultReasoningLevel); err != nil || strings.TrimSpace(m.DefaultReasoningLevel) == "" {
			continue
		}
		if !validUpstreamTone(strings.TrimSpace(m.UpstreamTone)) {
			continue
		}
		m.PublicModel = id
		m.UpstreamTone = strings.TrimSpace(m.UpstreamTone)
		m.DisplayName = strings.TrimSpace(m.DisplayName)
		m.DefaultReasoningLevel = strings.TrimSpace(m.DefaultReasoningLevel)
		seen[key] = struct{}{}
		out = append(out, m)
	}
	return out
}
func firstNonEmptySetting(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func validateSettings(v runtimeSettings) error {
	if v.MaxToolCallsPerTurn < 1 || v.MaxToolCallsPerTurn > 64 {
		return fmt.Errorf("每轮工具调用数必须为 1-64")
	}
	if v.MaxToolRounds < 1 || v.MaxToolRounds > 512 {
		return fmt.Errorf("最大工具轮次必须为 1-512")
	}
	if v.ContextWindow < 1024 {
		return fmt.Errorf("上下文窗口不能小于 1024")
	}
	if v.MaxOutputTokens < 1 || v.MaxOutputTokens >= v.ContextWindow {
		return fmt.Errorf("最大输出必须大于 0 且小于上下文窗口")
	}
	if v.ChatTimeoutSeconds < 5 || v.ChatTimeoutSeconds > 3600 {
		return fmt.Errorf("聊天超时必须为 5-3600 秒")
	}
	if v.ImageTimeoutSeconds < 5 || v.ImageTimeoutSeconds > 3600 {
		return fmt.Errorf("图片超时必须为 5-3600 秒")
	}
	if v.LogLevel != "silent" && v.LogLevel != "error" && v.LogLevel != "warn" && v.LogLevel != "info" && v.LogLevel != "debug" {
		return fmt.Errorf("日志等级必须为 silent、error、warn、info 或 debug")
	}
	if err := outbound.ValidateProxyURL(v.OutboundProxy); err != nil {
		return err
	}
	for _, proxyURL := range v.ProxyPool {
		if err := outbound.ValidateProxyURL(strings.TrimSpace(proxyURL)); err != nil {
			return err
		}
	}
	seen := make(map[string]struct{}, len(v.ModelMappings))
	for _, mapping := range v.ModelMappings {
		model := strings.TrimSpace(mapping.PublicModel)
		if model == "" {
			// Ignore blank mapping rows left over from partial UI edits; they
			// previously leaked into /v1/models as empty ids and crashed clients.
			continue
		}
		if !publicModelID.MatchString(model) {
			return fmt.Errorf("公开模型 ID 只能包含字母、数字、点、下划线、连字符或文字，且长度为 1-128")
		}
		key := strings.ToLower(model)
		if _, exists := seen[key]; exists {
			return fmt.Errorf("公开模型 ID %q 重复", model)
		}
		seen[key] = struct{}{}
		tone := strings.TrimSpace(mapping.UpstreamTone)
		if tone == "" {
			return fmt.Errorf("公开模型 %q 缺少上游 tone", model)
		}
		// Built-in catalog OR free-form custom ChatHub tone ids are accepted.
		if !validUpstreamTone(tone) {
			return fmt.Errorf("上游 tone %q 无效：请使用已知 tone 或自定义标识（字母/数字/_-., 1-128）", tone)
		}
		if strings.TrimSpace(mapping.DisplayName) == "" {
			return fmt.Errorf("公开模型 %q 缺少显示名称", model)
		}
		if _, err := normalizeReasoningEffort(mapping.DefaultReasoningLevel); err != nil || strings.TrimSpace(mapping.DefaultReasoningLevel) == "" {
			return fmt.Errorf("公开模型 %q 的默认推理级别无效", model)
		}
	}
	return nil
}
func (s *settingsStore) get() runtimeSettings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := s.v
	out.ModelMappings = sanitizeModelMappings(append([]modelMapping(nil), s.v.ModelMappings...))
	return out
}
func (s *settingsStore) save(v runtimeSettings) error {
	v.ModelMappings = sanitizeModelMappings(v.ModelMappings)
	if e := validateSettings(v); e != nil {
		return e
	}
	b, _ := json.MarshalIndent(v, "", "  ")
	if e := os.MkdirAll(filepath.Dir(s.path), 0700); e != nil {
		return e
	}
	if e := os.WriteFile(s.path, b, 0600); e != nil {
		return e
	}
	s.mu.Lock()
	s.v = v
	s.mu.Unlock()
	return nil
}
func (s *Server) adminSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		jsonOut(w, map[string]any{
			"settings": s.settings.get(),
			"codexModels": configurableCodexModels,
			"upstreamTones": publicUpstreamTones(),
			"defaultModelMappings": defaultModelMappings,
			"restartRequiredFields": []string{"listenAddress", "configPath", "tokenCachePath", "sessionCachePath", "outboundProxy", "proxyPool", "clientId", "authority", "redirectUri", "scope", "debugLogPath"},
		})
	case http.MethodPut:
		var v runtimeSettings
		if json.NewDecoder(r.Body).Decode(&v) != nil {
			writeOpenAIError(w, 400, "invalid_request_error", "bad json")
			return
		}
		if e := s.settings.save(v); e != nil {
			writeOpenAIError(w, 400, "invalid_request_error", e.Error())
			return
		}
		if e := outbound.ConfigurePool(v.ProxyPool); e != nil {
			writeOpenAIError(w, 400, "invalid_request_error", e.Error())
			return
		}
		// Return the sanitized view so the UI reflects what /v1/models will serve.
		jsonOut(w, map[string]any{"ok": true, "settings": s.settings.get()})
	default:
		writeOpenAIError(w, 405, "invalid_request_error", "method not allowed")
	}
}
func configuredToolCallLimit(s *settingsStore) int {
	if raw, ok := os.LookupEnv("M365_MAX_TOOL_CALLS_PER_TURN"); ok {
		if n, e := strconv.Atoi(strings.TrimSpace(raw)); e == nil && n >= 1 && n <= 64 {
			return n
		}
		return 1
	}
	return s.get().MaxToolCallsPerTurn
}

// adaptiveToolCallLimit permits parallel calls only when every call is a
// read-only, independently addressable operation. Any write, execution,
// mutation, or ambiguous tool is serialized conservatively.
func adaptiveToolCallLimit(c []detectedToolCall, configured int) int {
	if len(c) < 2 || configured < 2 {
		return 1
	}
	for _, call := range c {
		name := strings.ToLower(strings.TrimSpace(call.Name))
		if name == "" || toolLooksMutating(name) || !toolLooksReadOnly(name) {
			return 1
		}
	}
	return configured
}

func toolLooksMutating(name string) bool {
	for _, word := range []string{"exec", "shell", "command", "write", "edit", "update", "delete", "remove", "move", "rename", "create", "patch", "apply", "install", "run"} {
		if strings.Contains(name, word) {
			return true
		}
	}
	return false
}

func toolLooksReadOnly(name string) bool {
	for _, word := range []string{"read", "list", "search", "find", "get", "fetch", "browser", "lookup", "inspect", "stat", "status", "describe", "info"} {
		if strings.Contains(name, word) {
			return true
		}
	}
	return false
}

func limitToolCalls(c []detectedToolCall, n int) []detectedToolCall {
	if n < 1 {
		n = 1
	}
	if len(c) > n {
		return c[:n]
	}
	return c
}

func currentSettings() runtimeSettings { return openSettingsStore().get() }

// ApplyStartupSettingsEnv loads persisted restart-required fields before the
// rest of the application initializes. Explicit process environment variables
// always win over values saved from the web console.
func ApplyStartupSettingsEnv() {
	s := openSettingsStore().get()
	values := map[string]string{"M365_LISTEN": s.ListenAddress, "M365_CONFIG": s.ConfigPath, "M365_TOKEN_CACHE": s.TokenCachePath, "M365_SESSION_CACHE": s.SessionCachePath, outbound.EnvProxy: s.OutboundProxy, "M365_PROXY_POOL": strings.Join(s.ProxyPool, "\n"), "M365_CLIENT_ID": s.ClientID, "M365_AUTHORITY": s.Authority, "M365_REDIRECT_URI": s.RedirectURI, "M365_SCOPE": s.Scope, "M365_DEBUG_LOG": s.DebugLogPath}
	for k, v := range values {
		if _, exists := os.LookupEnv(k); !exists && strings.TrimSpace(v) != "" {
			_ = os.Setenv(k, v)
		}
	}
}
