package ui

import (
	"context"
	"os"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/Mateooo93/cortex-cli/internal/config"
	"github.com/Mateooo93/cortex-cli/internal/cortexconfig"
	llmprovider "github.com/Mateooo93/cortex-cli/internal/provider"
)

type settingsInputMode int

const (
	settingsInputNone settingsInputMode = iota
	settingsInputAPIKey
	settingsInputBaseURL
	settingsInputCustomProviderName
	settingsInputCustomProviderBaseURL
	settingsInputCustomProviderAPIKey
)

// settingsWizardField identifies the focused field inside a provider edit
// wizard. Field order is also the navigation order with up/down arrows.
type settingsWizardField int

const (
	wizardFieldName settingsWizardField = iota
	wizardFieldBaseURL
	wizardFieldAPIKey
)

// settingsWizard holds the state of the full-section provider edit wizard.
// The wizard opens when the user selects a provider in the Providers
// section. It replaces the provider list and presents three editable
// fields (Name, Base URL, API Key). Arrows move between fields; Enter
// commits the current field to disk; Esc commits any in-flight changes
// and closes the wizard.
type settingsWizard struct {
	active   bool
	provider string
	isCustom bool
	field    settingsWizardField

	// Working values. The active field is bound to the text input;
	// the other two are kept in sync with the input as the user types.
	name    string
	baseURL string
	apiKey  string

	// The text input is always focused; arrows switch fields by
	// loading/saving its value into the working-value slots above.
	input textinput.Model
}

type modelsFetchedMsg struct {
	provider string
	baseURL  string
	models   []string
	err      error
}
// refreshSettingsKeys rebuilds the Settings provider/API rows from Cortex config.
func (m *Model) refreshSettingsKeys() {
	if m.cortexCfg != nil {
		m.cortexCfg.EnsureProviderPresets()
	}
	m.settingsKeys = ProviderSettingsRows(m.cortexCfg)
	if m.settingsKeySel < 0 {
		m.settingsKeySel = 0
	}
	if len(m.settingsKeys) == 0 {
		m.settingsKeySel = 0
		return
	}
	if m.settingsKeySel >= len(m.settingsKeys) {
		m.settingsKeySel = len(m.settingsKeys) - 1
	}
}

func (m *Model) settingsProviders() []ProviderInfo {
	return ProvidersFromConfig(m.cortexCfg)
}

func (m *Model) selectedSettingsProviderName() string {
	providers := m.settingsProviders()
	if len(providers) == 0 {
		return ""
	}
	if m.settingsProviderSel < 0 {
		m.settingsProviderSel = 0
	}
	if m.settingsProviderSel >= len(providers) {
		m.settingsProviderSel = len(providers) - 1
	}
	return providers[m.settingsProviderSel].Name
}

func (m *Model) selectedSettingsModels() []ModelInfo {
	return ModelsForProviderFromConfig(m.selectedSettingsProviderName(), m.cortexCfg)
}

func (m *Model) currentSettingsModel() string {
	if m.cortexCfg != nil && m.cortexCfg.DefaultModel != "" {
		return m.cortexCfg.DefaultModel
	}
	if m.cfg != nil && m.cfg.Model != "" {
		return m.cfg.Model
	}
	if sess := m.currentSession(); sess != nil && sess.modelName != "" {
		return sess.modelName
	}
	return ""
}

func (m *Model) displayNameForModelSpec(spec string) string {
	if spec == "" {
		return ""
	}
	if m.cortexCfg != nil {
		if key, mc, err := m.cortexCfg.GetModel(spec); err == nil && mc != nil {
			if key == spec {
				return cortexconfig.ModelSpec(mc.Provider, mc.Model)
			}
			return key
		}
	}
	return spec
}

func (m *Model) setActiveModelSpec(spec string) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return
	}
	if m.cfg != nil {
		m.cfg.Model = spec
	}
	if m.cortexCfg != nil {
		m.cortexCfg.DefaultModel = spec
	}
	if sess := m.currentSession(); sess != nil {
		sess.modelName = spec
	}
}

func (m *Model) activeModelForNewSession() string {
	if m.cortexCfg != nil && m.cortexCfg.DefaultModel != "" {
		return m.cortexCfg.DefaultModel
	}
	if m.cfg != nil && m.cfg.Model != "" {
		return m.cfg.Model
	}
	return m.currentSettingsModel()
}

func (m *Model) canonicalSettingsModel(spec string) string {
	return normalizeSettingsModelSpec(spec, m.cortexCfg)
}

func (m *Model) openSettingsTab() {
	m.activeTab = TabKindSettings
	m.refreshSettingsKeys()
	m.settingsInKeyInput = false
	m.settingsInputMode = settingsInputNone
	m.settingsModelPending = ""
	m.settingsKeyInputLabel = ""
	m.settingsCustomProvider = ""
	m.settingsCustomBaseURL = ""
	m.settingsInspectProvider = ""
	m.settingsWizard = settingsWizard{}
	providerSel, modelSel := locateActiveModelFromConfig(m.currentSettingsModel(), m.cortexCfg)
	m.settingsProviderSel = providerSel
	m.settingsModelSel = modelSel
	if m.settingsModelColumn != 1 {
		m.settingsModelColumn = 1
	}
}

func (m *Model) findSettingsKeyIndex(provider string) int {
	for i, pk := range m.settingsKeys {
		if pk.Provider == provider {
			return i
		}
	}
	return -1
}

func (m *Model) settingsInspectView() SettingsInspectView {
	if m.settingsInspectProvider == "" {
		return SettingsInspectView{}
	}
	idx := m.findSettingsKeyIndex(m.settingsInspectProvider)
	if idx < 0 {
		return SettingsInspectView{}
	}
	pk := m.settingsKeys[idx]
	return SettingsInspectView{
		Provider:    pk.Provider,
		DisplayName: pk.DisplayName,
		BaseURL:     pk.BaseURL,
		HasAPIKey:   pk.KeyPrefix != "",
		KeyPrefix:   pk.KeyPrefix,
		Field:       m.settingsInspectField,
		NeedsAPIKey: pk.NeedsAPIKey,
		EnvVar:      pk.EnvVar,
		AuthKind:    pk.AuthKind,
		HelpURL:     pk.HelpURL,
	}
}

// settingsWizardView projects the live wizard state into a view-model
// that the renderer can consume without touching the text input or
// cortexCfg. The API key is masked unless the field is currently
// focused (so the user can see what they have typed).
func (m *Model) settingsWizardView() SettingsWizardView {
	w := m.settingsWizard
	if !w.active {
		return SettingsWizardView{}
	}
	maskKey := func(v string) string {
		if v == "" {
			return ""
		}
		if len(v) <= 4 {
			return strings.Repeat("•", len(v))
		}
		return strings.Repeat("•", len(v)-4) + v[len(v)-4:]
	}
	makeField := func(field settingsWizardField, value string) SettingsWizardFieldView {
		fv := SettingsWizardFieldView{
			Label:       w.fieldLabel(field),
			Placeholder: w.fieldPlaceholder(field),
			Focused:     w.field == field,
			Editable:    w.isFieldEditable(field),
		}
		if field == wizardFieldAPIKey {
			if w.field == field {
				fv.Value = value
			} else {
				fv.Value = maskKey(value)
			}
		} else {
			fv.Value = value
		}
		if !fv.Editable {
			fv.Hint = "preset — read-only"
			// Customise the hint per-field-type so the user
			// understands why the field is locked:
			//   - Name: the preset's display name is fixed
			//   - BaseURL: read from the preset
			//   - APIKey: hidden for local servers (Ollama, LM
			//     Studio, vLLM) and env-var providers (Bedrock)
			if field == wizardFieldAPIKey {
				auth := cortexconfig.ProviderAuthKind(w.provider)
				switch auth {
				case "none":
					fv.Hint = "no key — local server"
				case "env":
					if env := cortexconfig.ProviderEnvVar(w.provider); env != "" {
						fv.Hint = "read from $" + env
					} else {
						fv.Hint = "read from env"
					}
				}
			}
		}
		return fv
	}
	view := SettingsWizardView{
		Provider:    w.provider,
		DisplayName: w.name,
		IsCustom:    w.isCustom,
		Field:       w.field,
		InputView:   w.input.View(),
		Fields: []SettingsWizardFieldView{
			makeField(wizardFieldName, w.name),
			makeField(wizardFieldBaseURL, w.baseURL),
			makeField(wizardFieldAPIKey, w.apiKey),
		},
	}
	return view
}

func (m *Model) openSettingsTextInput(mode settingsInputMode, provider, label, placeholder, value string, password bool) {
	ti := textinput.New()
	ti.Placeholder = placeholder
	if password {
		ti.EchoMode = textinput.EchoPassword
	}
	ti.SetValue(value)
	ti.Focus()
	m.settingsKeyInput = ti
	m.settingsKeyInputProvider = provider
	m.settingsKeyInputLabel = label
	m.settingsInputMode = mode
	m.settingsInKeyInput = true
}

// openSettingsWizard opens the full-section provider edit wizard for the
// given provider. The wizard replaces the provider list inside the
// Providers section; the rest of the Settings tab is hidden. The wizard
// initialises its three working values from the current cortexCfg (or
// the provider's defaults when unset) and binds the first editable
// field to the text input.
func (m *Model) openSettingsWizard(providerName string) {
	providerName = cortexconfig.NormalizeProviderName(providerName)
	if providerName == "" {
		return
	}
	// OAuth providers should never enter the wizard — the caller must
	// short-circuit to startCodexLoginCmd (and the analogous flows
	// for claude-sub / copilot). This guard is defense-in-depth in
	// case a future refactor adds another openSettingsWizard call
	// site.
	if cortexconfig.ProviderAuthKind(providerName) == "oauth" {
		return
	}
	w := settingsWizard{
		active:   true,
		provider: providerName,
	}
	if m.cortexCfg != nil {
		if pc, ok := m.cortexCfg.ProviderConfig(providerName); ok {
			w.name = pc.Provider
			if pc.Provider == "" {
				w.name = providerName
			}
			w.isCustom = cortexconfig.IsCustomProvider(providerName)
		}
	}
	if w.name == "" {
		w.name = providerName
	}
	w.baseURL = m.providerBaseURL(providerName)
	w.apiKey = m.providerAPIKey(providerName)
	// Pre-select the most useful field for a typical edit: the API key
	// when the provider needs one and none is set, otherwise base URL.
	if w.apiKey == "" && cortexconfig.ProviderNeedsAPIKey(providerName) {
		w.field = wizardFieldAPIKey
	} else {
		w.field = wizardFieldBaseURL
	}
	w.input = m.newWizardFieldInput(w.field)
	m.settingsWizard = w
	m.settingsInspectProvider = ""
	m.settingsInKeyInput = false
	m.settingsInputMode = settingsInputNone
}

// closeSettingsWizard commits any in-flight changes, persists the
// provider config, refreshes derived UI state, and closes the wizard.
func (m *Model) closeSettingsWizard() tea.Cmd {
	w := m.settingsWizard
	if !w.active {
		return nil
	}
	m.captureWizardFieldValue()
	m.settingsWizard = settingsWizard{}
	m.refreshSettingsKeys()
	providerName := w.provider
	if m.cortexCfg != nil {
		if pc, ok := m.cortexCfg.ProviderConfig(providerName); ok {
			if w.isCustom && w.name != "" && cortexconfig.NormalizeProviderName(w.name) != providerName {
				// Custom-provider rename: rotate the config key.
				newName := cortexconfig.NormalizeProviderName(w.name)
				if newName != "" {
					updated := pc
					updated.Provider = newName
					delete(m.cortexCfg.Models, providerName)
					m.cortexCfg.Models[newName] = updated
					_ = cortexconfig.Save(m.cortexCfg)
				}
			}
		}
	}
	return m.fetchModelsForProvider(providerName)
}

// wizardFieldInput creates a fresh text input bound to the given wizard
// field. The input carries the current working value of the field and
// password-echoes the API key.
func (m *Model) newWizardFieldInput(field settingsWizardField) textinput.Model {
	w := &m.settingsWizard
	ti := textinput.New()
	ti.SetValue(w.fieldValue(field))
	if field == wizardFieldAPIKey {
		ti.EchoMode = textinput.EchoPassword
		ti.Placeholder = "Paste API key (leave empty to clear)"
	} else {
		ti.Placeholder = w.fieldPlaceholder(field)
	}
	ti.Focus()
	return ti
}

// fieldValue returns the working value of a wizard field.
func (w *settingsWizard) fieldValue(field settingsWizardField) string {
	switch field {
	case wizardFieldName:
		return w.name
	case wizardFieldBaseURL:
		return w.baseURL
	case wizardFieldAPIKey:
		return w.apiKey
	}
	return ""
}

// setFieldValue updates the working value of a wizard field.
func (w *settingsWizard) setFieldValue(field settingsWizardField, value string) {
	switch field {
	case wizardFieldName:
		w.name = value
	case wizardFieldBaseURL:
		w.baseURL = value
	case wizardFieldAPIKey:
		w.apiKey = value
	}
}

// fieldPlaceholder returns the placeholder text for a wizard field.
func (w *settingsWizard) fieldPlaceholder(field settingsWizardField) string {
	switch field {
	case wizardFieldName:
		if w.isCustom {
			return "e.g. groq, together, local-ai"
		}
		return "Provider name (preset — read-only)"
	case wizardFieldBaseURL:
		return "https://example.com/v1"
	}
	return ""
}

// fieldLabel returns the human-readable label for a wizard field.
func (w *settingsWizard) fieldLabel(field settingsWizardField) string {
	switch field {
	case wizardFieldName:
		return "Name"
	case wizardFieldBaseURL:
		return "Base URL"
	case wizardFieldAPIKey:
		return "API key"
	}
	return ""
}

// isFieldEditable reports whether the wizard field accepts edits.
func (w *settingsWizard) isFieldEditable(field settingsWizardField) bool {
	if field == wizardFieldName {
		return w.isCustom
	}
	// API-key field is hidden for local / env-var auth kinds:
	//   "none" — no key (Ollama, LM Studio, vLLM, Cortex)
	//   "env"  — key lives in an env var the user can set in their
	//            shell (Bedrock reads AWS_BEARER_TOKEN_BEDROCK)
	if field == wizardFieldAPIKey {
		auth := cortexconfig.ProviderAuthKind(w.provider)
		if auth == "none" || auth == "env" {
			return false
		}
	}
	return true
}

// captureWizardFieldValue writes the current text input value back into
// the wizard's working-value slot for the active field.
func (m *Model) captureWizardFieldValue() {
	w := &m.settingsWizard
	if !w.active {
		return
	}
	w.setFieldValue(w.field, w.input.Value())
}

// wizardMoveField switches the wizard's active field by delta (typically
// +1 for down, -1 for up). The current text input's value is captured
// into the outgoing field, then the input is rebound to the new field's
// working value.
func (m *Model) wizardMoveField(delta int) tea.Cmd {
	w := &m.settingsWizard
	if !w.active {
		return nil
	}
	m.captureWizardFieldValue()
	fields := []settingsWizardField{wizardFieldName, wizardFieldBaseURL, wizardFieldAPIKey}
	idx := 0
	for i, f := range fields {
		if f == w.field {
			idx = i
			break
		}
	}
	idx = (idx + delta + len(fields)) % len(fields)
	w.field = fields[idx]
	w.input = m.newWizardFieldInput(w.field)
	return nil
}

// wizardCommitCurrent writes the active field's value to cortexCfg and
// persists it to disk. The wizard stays open and the field stays
// selected so the user can keep editing.
func (m *Model) wizardCommitCurrent() tea.Cmd {
	w := &m.settingsWizard
	if !w.active {
		return nil
	}
	m.captureWizardFieldValue()
	providerName := w.provider
	if m.cortexCfg == nil {
		return nil
	}
	var status string
	switch w.field {
	case wizardFieldName:
		if w.isCustom {
			newName := cortexconfig.NormalizeProviderName(w.name)
			if newName == "" {
				return m.emitStatusMsg("Provider name cannot be empty", StatusMsgError)
			}
			if newName != providerName {
				if pc, ok := m.cortexCfg.Models[providerName]; ok {
					pc.Provider = newName
					delete(m.cortexCfg.Models, providerName)
					m.cortexCfg.Models[newName] = pc
					_ = cortexconfig.Save(m.cortexCfg)
					w.provider = newName
					w.isCustom = true
					status = "Provider renamed to " + newName
				}
			}
		}
	case wizardFieldBaseURL:
		val := strings.TrimSpace(w.baseURL)
		if val == "" {
			return m.emitStatusMsg("Base URL cannot be empty", StatusMsgError)
		}
		m.cortexCfg.SetProviderBaseURL(providerName, val)
		_ = cortexconfig.Save(m.cortexCfg)
		status = "Base URL saved for " + providerName
	case wizardFieldAPIKey:
		m.cortexCfg.SetProviderAPIKey(providerName, strings.TrimSpace(w.apiKey))
		_ = cortexconfig.Save(m.cortexCfg)
		if w.apiKey == "" {
			status = "API key cleared for " + providerName
		} else {
			status = "API key saved for " + providerName
		}
	}
	m.refreshSettingsKeys()
	if w.field == wizardFieldAPIKey || w.field == wizardFieldBaseURL {
		return tea.Batch(m.emitStatusMsg(status, StatusMsgInfo), m.fetchModelsForProvider(providerName))
	}
	return m.emitStatusMsg(status, StatusMsgInfo)
}

func (m *Model) settingsProviderHasKey(providerName string) bool {
	return m.providerConfigured(providerName)
}

// providerConfigured reports whether the provider already has usable
// credentials: OAuth token for subscription providers, or an API key
// from Settings (cortexCfg), env, or keychain.
func (m *Model) providerConfigured(providerName string) bool {
	providerName = cortexconfig.NormalizeProviderName(providerName)
	authKind := cortexconfig.ProviderAuthKind(providerName)
	switch authKind {
	case "oauth":
		return config.OAuthProviderSignedIn(providerName)
	case "none", "env":
		return true
	default:
		if m.providerAPIKey(providerName) != "" {
			return true
		}
		key, _ := config.ResolveProviderKey(providerName, false)
		return key != ""
	}
}

// switchToModelSpec activates spec, persists it, and notifies the session.
func (m *Model) switchToModelSpec(spec string) tea.Cmd {
	if m.cortexCfg != nil {
		provider, model, _ := cortexconfig.SplitModelSpec(spec)
		if ensured := m.cortexCfg.EnsureProviderModel(provider, model); ensured != "" {
			spec = ensured
		}
	}
	m.setActiveModelSpec(spec)
	if m.cortexCfg != nil {
		m.cortexCfg.DefaultModel = spec
		_ = cortexconfig.Save(m.cortexCfg)
	}
	sess := m.currentSession()
	if sess != nil && sess.client != nil {
		_ = sess.client.SendSetModel(spec)
	}
	return m.emitStatusMsg("Switched to "+spec, StatusMsgInfo)
}

func (m *Model) providerAPIKey(providerName string) string {
	if m.cortexCfg != nil {
		if pc, ok := m.cortexCfg.ProviderConfig(providerName); ok && pc.APIKey != "" {
			return pc.APIKey
		}
	}
	if envVar := cortexconfig.ProviderEnvVar(providerName); envVar != "" {
		return os.Getenv(envVar)
	}
	return ""
}

func (m *Model) providerBaseURL(providerName string) string {
	if m.cortexCfg != nil {
		if pc, ok := m.cortexCfg.ProviderConfig(providerName); ok && pc.BaseURL != "" {
			return pc.BaseURL
		}
	}
	return cortexconfig.DefaultBaseURL(providerName)
}

func (m *Model) selectSettingsModel(mod ModelInfo) tea.Cmd {
	spec := normalizeSettingsModelSpec(mod.Spec, m.cortexCfg)
	if spec == "" {
		return m.emitStatusMsg("Model selection is empty", StatusMsgError)
	}
	modelID := strings.TrimSpace(mod.DisplayName)
	if _, parsedModel, ok := cortexconfig.SplitModelSpec(spec); ok {
		modelID = parsedModel
	}
	if m.cortexCfg != nil {
		if _, _, err := m.cortexCfg.GetModel(spec); err != nil {
			if ensured := m.cortexCfg.EnsureProviderModel(mod.Provider, modelID); ensured != "" {
				spec = ensured
			}
		}
		m.cortexCfg.DefaultModel = spec
		_ = cortexconfig.Save(m.cortexCfg)
	}
	m.setActiveModelSpec(spec)
	if settSess := m.currentSession(); settSess != nil && settSess.client != nil {
		_ = settSess.client.SendSetModel(spec)
	}
	m.refreshSettingsKeys()
	m.settingsProviderSel, m.settingsModelSel = locateActiveModelFromConfig(spec, m.cortexCfg)
	return m.emitStatusMsg("Model set to "+mod.DisplayName, StatusMsgInfo)
}

func (m *Model) fetchModelsForProvider(providerName string) tea.Cmd {
	providerName = cortexconfig.NormalizeProviderName(providerName)
	// SuperGrok subscription exposes a fixed pair of models (see
	// `grok models`), not the full console.x.ai catalogue.
	if providerName == "xai-sub" {
		return func() tea.Msg {
			var ids []string
			for _, mod := range ModelsForProvider(providerName) {
				if _, model, ok := cortexconfig.SplitModelSpec(mod.Spec); ok && model != "" {
					ids = append(ids, model)
				}
			}
			return modelsFetchedMsg{provider: providerName, models: ids}
		}
	}
	baseURL := m.providerBaseURL(providerName)
	apiKey := m.providerAPIKey(providerName)
	candidates := m.modelRefreshBaseURLCandidates(providerName, baseURL)
	return func() tea.Msg {
		models, usedBaseURL, err := llmprovider.FetchModelsFromCandidates(context.Background(), apiKey, candidates...)
		return modelsFetchedMsg{provider: providerName, baseURL: usedBaseURL, models: models, err: err}
	}
}

func (m *Model) modelRefreshBaseURLCandidates(providerName, baseURL string) []string {
	trimmedBaseURL := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if trimmedBaseURL == "" {
		return nil
	}

	var candidates []string
	seen := map[string]bool{}
	appendCandidate := func(candidate string) {
		candidate = strings.TrimRight(strings.TrimSpace(candidate), "/")
		if candidate == "" {
			return
		}
		key := strings.ToLower(candidate)
		if seen[key] {
			return
		}
		seen[key] = true
		candidates = append(candidates, candidate)
	}
	appendScopedCandidate := func(scope string) {
		scope = strings.ToLower(strings.TrimSpace(scope))
		if scope == "" {
			return
		}
		if strings.HasSuffix(strings.ToLower(trimmedBaseURL), "/"+scope) {
			return
		}
		appendCandidate(trimmedBaseURL + "/" + scope)
	}

	appendCandidate(trimmedBaseURL)
	for _, scope := range m.providerModelScopes(providerName) {
		appendScopedCandidate(scope)
	}
	if providerName == "opengateway" || strings.Contains(strings.ToLower(trimmedBaseURL), "opengateway") {
		for _, scope := range []string{"xiaomi", "google", "minimax", "qwen", "nvidia"} {
			appendScopedCandidate(scope)
		}
	} else if cortexconfig.DefaultBaseURL(providerName) == "" {
		appendScopedCandidate(providerName)
	}
	return candidates
}

func (m *Model) providerModelScopes(providerName string) []string {
	if m.cortexCfg == nil {
		return nil
	}
	providerName = cortexconfig.NormalizeProviderName(providerName)
	seen := map[string]bool{}
	var scopes []string
	for _, mc := range m.cortexCfg.Models {
		if cortexconfig.NormalizeProviderName(mc.Provider) != providerName {
			continue
		}
		prefix, _, ok := cortexconfig.SplitModelSpec(mc.Model)
		if !ok {
			continue
		}
		prefix = strings.ToLower(strings.TrimSpace(prefix))
		if prefix == "" || prefix == providerName || seen[prefix] {
			continue
		}
		seen[prefix] = true
		scopes = append(scopes, prefix)
	}
	return scopes
}

func (m *Model) openAPIKeyInput(providerName string) {
	m.openSettingsTextInput(settingsInputAPIKey, providerName, "Provider: "+providerName+" API key", "Paste your "+providerName+" API key...", "", true)
}

func normalizedTheme(theme string) string {
	switch strings.ToLower(strings.TrimSpace(theme)) {
	case "dark", "light", "auto":
		return strings.ToLower(strings.TrimSpace(theme))
	default:
		return "auto"
	}
}

func nextTheme(theme string) string {
	switch normalizedTheme(theme) {
	case "auto":
		return "dark"
	case "dark":
		return "light"
	default:
		return "auto"
	}
}

func normalizedReasoningEffort(effort string) string {
	switch strings.ToLower(strings.TrimSpace(effort)) {
	case "auto", "low", "medium", "high", "xhigh", "minimal", "none":
		return strings.ToLower(strings.TrimSpace(effort))
	default:
		return "auto"
	}
}

func nextReasoningEffort(effort string) string {
	switch normalizedReasoningEffort(effort) {
	case "auto":
		return "low"
	case "low":
		return "medium"
	case "medium":
		return "high"
	case "high":
		return "xhigh"
	case "xhigh":
		return "minimal"
	case "minimal":
		return "none"
	default:
		return "auto"
	}
}

func (m *Model) configuredTheme() string {
	if m.cortexCfg == nil {
		return "auto"
	}
	return normalizedTheme(m.cortexCfg.Theme)
}

func (m *Model) applyConfiguredTheme() {
	theme := m.configuredTheme()
	switch theme {
	case "dark":
		m.hasDarkBG = true
	case "light":
		m.hasDarkBG = false
	}
	m.styles = NewStyles(m.hasDarkBG)
	width := 80
	if m.mdRenderer != nil {
		width = m.mdRenderer.width
	}
	m.mdRenderer = NewMarkdownRenderer(width, m.hasDarkBG, m.styles.CodeBoxBorderStyle)
}

func (m *Model) currentReasoningEffort() string {
	if m.cortexCfg == nil {
		return "auto"
	}
	_, mc, err := m.cortexCfg.GetModel(m.currentSettingsModel())
	if err != nil || mc == nil {
		return "auto"
	}
	return normalizedReasoningEffort(mc.ReasoningEffort)
}

func (m *Model) setActiveReasoningEffort(effort string) {
	if m.cortexCfg == nil {
		return
	}
	key, mc, err := m.cortexCfg.GetModel(m.currentSettingsModel())
	if err != nil || mc == nil || key == "" {
		return
	}
	updated := *mc
	updated.ReasoningEffort = normalizedReasoningEffort(effort)
	m.cortexCfg.Models[key] = updated
	_ = cortexconfig.Save(m.cortexCfg)
}

func (m *Model) configuredShowUsage() bool {
	if m.cortexCfg == nil {
		return true
	}
	return m.cortexCfg.ShowUsage
}

// configuredAutoCompact reports whether the user has enabled
// auto-compact in Settings → Other Settings. The default is true
// so brand-new users get the safety net; power users can turn it
// off in the Settings tab.
func (m *Model) configuredAutoCompact() bool {
	if m.cortexCfg == nil {
		return true
	}
	return m.cortexCfg.AutoCompact
}

func (m *Model) setConfiguredAutoCompact(v bool) {
	if m.cortexCfg == nil {
		return
	}
	m.cortexCfg.AutoCompact = v
	_ = cortexconfig.Save(m.cortexCfg)
}

func (m *Model) setConfiguredTheme(theme string) {
	if m.cortexCfg == nil {
		return
	}
	m.cortexCfg.Theme = normalizedTheme(theme)
	_ = cortexconfig.Save(m.cortexCfg)
	m.applyConfiguredTheme()
}

func (m *Model) setConfiguredShowUsage(v bool) {
	if m.cortexCfg == nil {
		return
	}
	m.cortexCfg.ShowUsage = v
	_ = cortexconfig.Save(m.cortexCfg)
}

// settingsOtherOptionCount matches the row count rendered in renderSettingsView
// for the "Other Settings" section. Keep in sync with tabs.go.
const settingsOtherOptionCount = 5

func (m *Model) setAllSessionsShowThinking(show bool) {
	for _, sess := range m.sessions {
		if sess == nil {
			continue
		}
		sess.showThinking = show
		if show && sess.thinkingBuf != "" {
			sess.thinkingRendered = renderThinkingText(sess.thinkingBuf, m.styles, m.mdRenderer.width+4)
		} else {
			sess.thinkingRendered = ""
		}
	}
	_ = config.SetShowThinking(show)
}
