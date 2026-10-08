package reviewer

import (
	"context"
	"encoding/json"
	"errors"
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
