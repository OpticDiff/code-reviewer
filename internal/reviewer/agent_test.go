package reviewer

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/OpticDiff/code-reviewer/pkg/model"
	"google.golang.org/genai"
)

// scriptedChatter replies with a fixed sequence of model texts.
type scriptedChatter struct {
	replies []string
	calls   int
}

func (s *scriptedChatter) Chat(_ context.Context, _ []*genai.Content, _ *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
	text := s.replies[s.calls]
	s.calls++
	return &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{{Content: genai.NewContentFromText(text, genai.RoleModel)}},
	}, nil
}

func TestSplitAgentCandidates(t *testing.T) {
	findings := []model.Finding{
		{File: "a.go", Line: 1, Severity: "CRITICAL"},
		{File: "b.go", Line: 2, Severity: "medium"},
		{File: "c.go", Line: 3, Severity: "High"},
		{File: "d.go", Line: 4, Severity: "LOW"},
	}

	cand, rest := splitAgentCandidates(findings, "high_severity_only")
	if len(cand) != 2 || cand[0].File != "a.go" || cand[1].File != "c.go" {
		t.Errorf("candidates = %+v, want a.go and c.go", cand)
	}
	if len(rest) != 2 {
		t.Errorf("rest = %+v, want 2 findings", rest)
	}

	for _, scope := range []string{"", "all"} {
		cand, rest = splitAgentCandidates(findings, scope)
		if len(cand) != 4 || len(rest) != 0 {
			t.Errorf("scope %q: got %d candidates, %d rest; want 4, 0", scope, len(cand), len(rest))
		}
	}
}

func TestRunAgentLoop_RecordsVerdicts(t *testing.T) {
	initial := []model.Finding{
		{File: "a.go", Line: 1, Severity: "HIGH", Title: "kept"},
		{File: "b.go", Line: 2, Severity: "HIGH", Title: "refuted"},
	}
	refined, _ := json.Marshal(initial[:1])
	chatter := &scriptedChatter{replies: []string{
		`{"action":"findings","findings":` + string(refined) + `}`,
	}}

	res, err := RunAgentLoop(context.Background(), chatter, AgentConfig{RepoRoot: t.TempDir()}, initial)
	if err != nil {
		t.Fatal(err)
	}
	want := []FindingVerdict{
		{File: "a.go", Line: 1, Severity: "HIGH", Title: "kept", Status: VerdictVerified},
		{File: "b.go", Line: 2, Severity: "HIGH", Title: "refuted", Status: VerdictDismissed},
	}
	if len(res.Verdicts) != len(want) {
		t.Fatalf("verdicts = %+v, want %+v", res.Verdicts, want)
	}
	for i := range want {
		if res.Verdicts[i] != want[i] {
			t.Errorf("verdict[%d] = %+v, want %+v", i, res.Verdicts[i], want[i])
		}
	}
}

func TestRunAgentLoop_UnparseableResponseLeavesFindingsUnverified(t *testing.T) {
	initial := []model.Finding{{File: "a.go", Line: 1, Severity: "HIGH", Title: "t"}}
	chatter := &scriptedChatter{replies: []string{"not json"}}

	res, err := RunAgentLoop(context.Background(), chatter, AgentConfig{RepoRoot: t.TempDir()}, initial)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Verdicts) != 1 || res.Verdicts[0].Status != VerdictUnverified {
		t.Errorf("verdicts = %+v, want one unverified", res.Verdicts)
	}
}

func TestBuildAuditEntry_IncludesAgentVerdicts(t *testing.T) {
	verdicts := []FindingVerdict{{File: "a.go", Line: 1, Severity: "HIGH", Title: "t", Status: VerdictDismissed}}
	entry := AuditEntry{AgentVerdicts: verdicts}
	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	got, ok := m["agent_verdicts"].([]any)
	if !ok || len(got) != 1 {
		t.Fatalf("agent_verdicts = %v", m["agent_verdicts"])
	}
	if got[0].(map[string]any)["status"] != "dismissed" {
		t.Errorf("verdict = %v", got[0])
	}
}

func TestRunScopedAgentLoop_HighSeverityOnlyLeavesRestUntouched(t *testing.T) {
	findings := []model.Finding{
		{File: "a.go", Line: 1, Severity: "HIGH", Title: "refuted"},
		{File: "b.go", Line: 2, Severity: "LOW", Title: "minor"},
	}
	chatter := &scriptedChatter{replies: []string{`{"action":"findings","findings":[]}`}}

	merged, res, err := runScopedAgentLoop(context.Background(), chatter, AgentConfig{RepoRoot: t.TempDir()}, findings, AgentScopeHighSeverityOnly)
	if err != nil {
		t.Fatal(err)
	}
	if len(merged) != 1 || merged[0].File != "b.go" {
		t.Errorf("merged = %+v, want only the LOW finding", merged)
	}
	if len(res.Verdicts) != 1 || res.Verdicts[0].Status != VerdictDismissed {
		t.Errorf("verdicts = %+v, want one dismissed (LOW finding is never examined)", res.Verdicts)
	}
}

func TestRunScopedAgentLoop_NoCandidatesSkipsModel(t *testing.T) {
	findings := []model.Finding{{File: "b.go", Line: 2, Severity: "LOW"}}
	chatter := &scriptedChatter{}

	merged, res, err := runScopedAgentLoop(context.Background(), chatter, AgentConfig{RepoRoot: t.TempDir()}, findings, AgentScopeHighSeverityOnly)
	if err != nil {
		t.Fatal(err)
	}
	if chatter.calls != 0 || res != nil || len(merged) != 1 {
		t.Errorf("calls=%d res=%v merged=%v; want no model call and findings unchanged", chatter.calls, res, merged)
	}
}

type failingChatter struct{}

func (failingChatter) Chat(context.Context, []*genai.Content, *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
	return nil, errors.New("backend unavailable")
}

func TestRunScopedAgentLoop_ChatErrorLeavesFindingsUnverified(t *testing.T) {
	findings := []model.Finding{{File: "a.go", Line: 1, Severity: "HIGH", Title: "t"}}

	merged, res, err := runScopedAgentLoop(context.Background(), failingChatter{}, AgentConfig{RepoRoot: t.TempDir()}, findings, AgentScopeHighSeverityOnly)
	if err == nil {
		t.Fatal("want error")
	}
	if len(merged) != 1 {
		t.Errorf("merged = %+v, want original findings", merged)
	}
	if res == nil || len(res.Verdicts) != 1 || res.Verdicts[0].Status != VerdictUnverified {
		t.Fatalf("result = %+v, want one unverified verdict", res)
	}
}

func TestValidateRefinedFindings_RemovedLinesKeyedByOldLine(t *testing.T) {
	initial := []model.Finding{
		{File: "a.go", OldLine: 21, Title: "first"},
		{File: "a.go", Line: 5, Title: "added"},
	}
	refined := []model.Finding{
		{File: "a.go", OldLine: 40, Title: "swapped removed-line finding"},
		{File: "a.go", Line: 5, Title: "added"},
	}
	got := validateRefinedFindings(initial, refined)
	if len(got) != 1 {
		t.Fatalf("expected only the added finding, got %+v", got)
	}
	for _, f := range got {
		if f.OldLine == 40 {
			t.Errorf("refine step must not introduce a new removed-line finding: %+v", f)
		}
	}
}

func TestBuildVerdicts_RemovedLinesAreDistinct(t *testing.T) {
	initial := []model.Finding{
		{File: "a.go", OldLine: 21, Title: "first"},
		{File: "a.go", OldLine: 22, Title: "second"},
	}
	final := []model.Finding{{File: "a.go", OldLine: 22, Title: "second"}}
	got := buildVerdicts(initial, final, true)
	if got[0].Status != VerdictDismissed || got[1].Status != VerdictVerified {
		t.Errorf("verdicts = %+v, want first dismissed and second verified", got)
	}
	if got[0].OldLine != 21 {
		t.Errorf("verdict should carry old_line, got %+v", got[0])
	}
}

// emptyChatter mimics Gemini 3 answering a text-protocol prompt with a native
// function call it cannot complete: no content, finish reason set, tokens spent.
type emptyChatter struct{ reason genai.FinishReason }

func (e emptyChatter) Chat(_ context.Context, _ []*genai.Content, _ *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
	return &genai.GenerateContentResponse{
		Candidates:    []*genai.Candidate{{Content: &genai.Content{Role: genai.RoleModel}, FinishReason: e.reason}},
		UsageMetadata: &genai.GenerateContentResponseUsageMetadata{PromptTokenCount: 420, CandidatesTokenCount: 27, TotalTokenCount: 508},
	}, nil
}

func TestRunAgentLoop_EmptyResponseIsAnErrorNamingTheFinishReason(t *testing.T) {
	initial := []model.Finding{{File: "a.go", Line: 1, Severity: "HIGH", Title: "t"}}

	res, err := RunAgentLoop(context.Background(), emptyChatter{reason: genai.FinishReasonMalformedFunctionCall}, AgentConfig{RepoRoot: t.TempDir()}, initial)
	if err == nil {
		t.Fatal("want an error for an empty model response")
	}
	if !strings.Contains(err.Error(), "MALFORMED_FUNCTION_CALL") {
		t.Errorf("error %q does not name the finish reason", err)
	}
	if res == nil {
		t.Fatal("result must be returned with the error so usage and verdicts are kept")
	}
	if res.StopReason != "error" || res.Iterations != 1 || res.Usage.TotalTokens != 508 {
		t.Errorf("result = %+v, want stop_reason error, 1 iteration, 508 tokens", res)
	}
	if len(res.Verdicts) != 1 || res.Verdicts[0].Status != VerdictUnverified {
		t.Errorf("verdicts = %+v, want one unverified", res.Verdicts)
	}
}

func TestRunScopedAgentLoop_EmptyResponseKeepsFindingsAndReports(t *testing.T) {
	findings := []model.Finding{{File: "a.go", Line: 1, Severity: "HIGH", Title: "t"}}

	merged, res, err := runScopedAgentLoop(context.Background(), emptyChatter{reason: genai.FinishReasonMalformedFunctionCall}, AgentConfig{RepoRoot: t.TempDir()}, findings, AgentScopeAll)
	if err == nil {
		t.Fatal("want error")
	}
	if len(merged) != 1 || res == nil {
		t.Fatalf("merged = %+v, res = %+v; want original findings and a result", merged, res)
	}
}

// captureChatter records the config of the first call and then ends the loop.
type captureChatter struct{ config *genai.GenerateContentConfig }

func (c *captureChatter) Chat(_ context.Context, _ []*genai.Content, cfg *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
	c.config = cfg
	return &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{{Content: genai.NewContentFromText(`{"action":"done"}`, genai.RoleModel)}},
	}, nil
}

func TestRunAgentLoop_RequestsSchemaConstrainedJSON(t *testing.T) {
	chatter := &captureChatter{}
	initial := []model.Finding{{File: "a.go", Line: 1, Severity: "HIGH", Title: "t"}}

	if _, err := RunAgentLoop(context.Background(), chatter, AgentConfig{RepoRoot: t.TempDir()}, initial); err != nil {
		t.Fatal(err)
	}
	cfg := chatter.config
	if cfg.ResponseMIMEType != "application/json" {
		t.Errorf("ResponseMIMEType = %q, want application/json", cfg.ResponseMIMEType)
	}
	if cfg.ResponseSchema == nil {
		t.Error("ResponseSchema is nil: Gemini 3 answers the text protocol with a native function call unless decoding is constrained")
	}
}

// Every shape the loop accepts must be expressible in the schema, and every
// schema branch must be something the loop can execute.
func TestAgentResponseSchema_MatchesLoopProtocol(t *testing.T) {
	schema := agentResponseSchema(true)

	actions := map[string]bool{}
	tools := map[string]bool{}
	var findingProps map[string]*genai.Schema
	var findingRequired []string
	for _, branch := range schema.AnyOf {
		act := branch.Properties["action"]
		if act == nil || len(act.Enum) != 1 {
			t.Fatalf("branch without a single-valued action: %+v", branch)
		}
		actions[act.Enum[0]] = true
		if tool := branch.Properties["tool"]; tool != nil {
			tools[tool.Enum[0]] = true
			if len(branch.Required) == 0 || !contains(branch.Required, "args") {
				t.Errorf("tool branch %v must require args", tool.Enum)
			}
		}
		if f := branch.Properties["findings"]; f != nil {
			findingProps = f.Items.Properties
			findingRequired = f.Items.Required
		}
	}
	for _, want := range []string{"tool", "findings", "done"} {
		if !actions[want] {
			t.Errorf("schema has no %q action", want)
		}
	}

	reg := NewToolRegistry(t.TempDir())
	for tool := range tools {
		if res := reg.Execute(context.Background(), tool, map[string]any{}); strings.HasPrefix(res.Error, "unknown tool") {
			t.Errorf("schema offers tool %q the registry does not implement", tool)
		}
	}
	for _, tool := range []string{"read_file", "find_definition", "list_directory"} {
		if !tools[tool] {
			t.Errorf("schema does not offer tool %q", tool)
		}
	}

	typ := reflect.TypeOf(model.Finding{})
	for i := 0; i < typ.NumField(); i++ {
		name, _, _ := strings.Cut(typ.Field(i).Tag.Get("json"), ",")
		if findingProps[name] == nil {
			t.Errorf("finding field %q is missing from the schema, so a refined finding could not carry it", name)
		}
	}

	// A refined finding replaces the initial one and is matched to it by file, line and
	// old_line, so a schema-valid answer that leaves one out would silently dismiss it.
	for i := 0; i < typ.NumField(); i++ {
		name, opts, _ := strings.Cut(typ.Field(i).Tag.Get("json"), ",")
		if (opts != "omitempty" || name == "old_line") && !contains(findingRequired, name) {
			t.Errorf("finding field %q is not required by the schema", name)
		}
	}
}

func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

func TestNewAgentAudit_RecordsStopReasonAndError(t *testing.T) {
	res := &AgentResult{Iterations: 1, StopReason: "error", ToolCalls: map[string]int{}}
	got := newAgentAudit(res, errors.New("agent iteration 1: empty model response (finish reason MALFORMED_FUNCTION_CALL)"), 3, 3)

	if got.StopReason != "error" || got.Iterations != 1 || got.FindingsBefore != 3 || got.FindingsAfter != 3 {
		t.Errorf("audit = %+v", got)
	}
	if !strings.Contains(got.Error, "MALFORMED_FUNCTION_CALL") {
		t.Errorf("error %q lost the cause", got.Error)
	}

	data, err := json.Marshal(AuditEntry{Agent: got})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"agent":{`) || !strings.Contains(string(data), `"stop_reason":"error"`) {
		t.Errorf("audit JSON = %s", data)
	}
}

func TestNewAgentAudit_NilWhenLoopDidNotRun(t *testing.T) {
	if got := newAgentAudit(nil, nil, 2, 2); got != nil {
		t.Errorf("audit = %+v, want nil when the loop never ran", got)
	}
}

// recordingChatter replays scripted replies and keeps what each call was sent.
type recordingChatter struct {
	replies   []string
	configs   []*genai.GenerateContentConfig
	histories [][]*genai.Content
}

func (r *recordingChatter) Chat(_ context.Context, history []*genai.Content, cfg *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
	r.configs = append(r.configs, cfg)
	r.histories = append(r.histories, append([]*genai.Content(nil), history...))
	text := r.replies[len(r.configs)-1]
	return &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{{Content: genai.NewContentFromText(text, genai.RoleModel)}},
	}, nil
}

func toolActions(cfg *genai.GenerateContentConfig) int {
	n := 0
	for _, b := range cfg.ResponseSchema.AnyOf {
		if b.Properties["action"].Enum[0] == "tool" {
			n++
		}
	}
	return n
}

// A model that spends every turn on tools would exhaust the iterations without
// ruling on a single finding, so the last turn may only answer.
func TestRunAgentLoop_LastTurnCannotCallTools(t *testing.T) {
	initial := []model.Finding{{File: "a.go", Line: 1, Severity: "HIGH", Title: "t"}}
	tool := `{"action":"tool","tool":"list_directory","args":{"path":"."}}`
	chatter := &recordingChatter{replies: []string{tool, tool, `{"action":"findings","findings":[]}`}}

	res, err := RunAgentLoop(context.Background(), chatter, AgentConfig{RepoRoot: t.TempDir(), MaxIterations: 3}, initial)
	if err != nil {
		t.Fatal(err)
	}
	if res.StopReason != "done" || res.Verdicts[0].Status != VerdictDismissed {
		t.Errorf("result = %+v, want the final answer applied", res)
	}
	for i, want := range []int{3, 3, 0} {
		if got := toolActions(chatter.configs[i]); got != want {
			t.Errorf("call %d offers %d tool branches, want %d", i+1, got, want)
		}
	}
	last := chatter.histories[2]
	notice := last[len(last)-1].Parts[0].Text
	if !strings.Contains(notice, "last turn") {
		t.Errorf("final request does not tell the model it is the last turn: %q", notice)
	}
	if strings.Contains(chatter.histories[1][len(chatter.histories[1])-1].Parts[0].Text, "last turn") {
		t.Error("the notice must not be sent before the final turn")
	}
}
