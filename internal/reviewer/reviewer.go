// Package reviewer orchestrates the code review pipeline:
// input (diffs) → model (AI analysis) → output (terminal or GitLab).
package reviewer

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/OpticDiff/code-reviewer/internal/cache"
	"github.com/OpticDiff/code-reviewer/internal/config"
	ctxpkg "github.com/OpticDiff/code-reviewer/internal/context"
	"github.com/OpticDiff/code-reviewer/internal/diff"
	"github.com/OpticDiff/code-reviewer/internal/model"
	"github.com/OpticDiff/code-reviewer/internal/vcs"
)

// DiffSource provides diffs for review. Extracted for testability.
type DiffSource interface {
	GetDiffs(ctx context.Context) ([]diff.FileDiff, string, string, error)
}

// Reviewer orchestrates the full review pipeline.
type Reviewer struct {
	cfg              *config.Config
	provider         ModelReviewer
	glClient         VCSClient
	diffSource       DiffSource
	contextProvider  ctxpkg.Provider
	mrDraft          bool
	parseFailedFiles []string // files whose diffs failed to parse (unreviewed)
	cache            *cache.Cache
}

// New creates a new Reviewer.
func New(cfg *config.Config, provider ModelReviewer, glClient VCSClient) *Reviewer {
	return &Reviewer{
		cfg:      cfg,
		provider: provider,
		glClient: glClient,
		cache:    initCache(cfg),
	}
}

// NewWithContext creates a Reviewer with a context provider for repo-aware reviews.
func NewWithContext(cfg *config.Config, provider ModelReviewer, glClient VCSClient, cp ctxpkg.Provider) *Reviewer {
	return &Reviewer{
		cfg:             cfg,
		provider:        provider,
		glClient:        glClient,
		contextProvider: cp,
		cache:           initCache(cfg),
	}
}

// NewWithDiffSource creates a Reviewer with a custom DiffSource (useful for testing).
func NewWithDiffSource(cfg *config.Config, provider ModelReviewer, glClient VCSClient, ds DiffSource) *Reviewer {
	return &Reviewer{
		cfg:        cfg,
		provider:   provider,
		glClient:   glClient,
		diffSource: ds,
		cache:      initCache(cfg),
	}
}

// Run executes the full review pipeline and returns the number of findings.
func (r *Reviewer) Run(ctx context.Context) (int, error) {
	var scopeAssessment *ScopeAssessment

	// Hoist variables captured by the audit defer.
	var diffs []diff.FileDiff
	var skippedFiles []string
	var allFindings []model.Finding
	var totalUsage model.TokenUsage
	var dedupedCount int
	var cacheHits int

	start := time.Now()

	// Deferred audit log: writes one record per run regardless of exit path.
	defer func() {
		if r.cfg.AuditLog == "" {
			return
		}
		entry := buildAuditEntry(r.cfg, diffs, skippedFiles, allFindings, dedupedCount, cacheHits, &totalUsage, time.Since(start))
		if err := WriteAuditLog(r.cfg.AuditLog, entry); err != nil {
			slog.Warn("failed to write audit log", "error", err)
		}
	}()

	// Step 1: Get diffs.
	slog.Info("fetching diffs", "mode", r.cfg.Mode())
	var mrTitle, mrDesc string
	var err error
	var cachedVersions []vcs.DiffVersion
	if r.diffSource != nil {
		diffs, mrTitle, mrDesc, err = r.diffSource.GetDiffs(ctx)
	} else {
		diffs, mrTitle, mrDesc, err = r.getDiffs(ctx)
	}
	if err != nil {
		return 0, fmt.Errorf("getting diffs: %w", err)
	}
	slog.Info(fmt.Sprintf("found %d file(s) in diff", len(diffs)))

	// Step 2: Filter excluded files.
	diffs = diff.Filter(diffs, r.cfg.ExcludedPatterns)
	slog.Info(fmt.Sprintf("%d file(s) after filtering", len(diffs)))

	if len(diffs) == 0 {
		slog.Info("no files to review after filtering")
		fmt.Println("✅ No reviewable files in diff.")
		return 0, nil
	}

	// Step 2a: Scope enforcement — warn or fail on oversized MRs.
	if r.cfg.MaxFiles > 0 {
		scopeAssessment = CheckScope(diffs, r.cfg.MaxFiles)
		LogScopeStatus(scopeAssessment)
		if scopeAssessment.IsOversized {
			warning := FormatScopeWarning(scopeAssessment)
			fmt.Fprint(os.Stderr, warning)
			if r.cfg.ScopeAction == "fail" {
				return 0, fmt.Errorf("scope limit exceeded: %d files (max %d)", len(diffs), r.cfg.MaxFiles)
			}
		}
	}

	// Step 2b: Incremental review — filter to only files changed in latest push.
	var incrementalChangedFiles []string // tracks which files changed (for selective cleanup)
	if r.cfg.Incremental && r.cfg.CIMode && r.glClient != nil {
		versions, verr := r.glClient.GetMRVersions(ctx, r.cfg.CIProjectID, r.cfg.CIMergeRequestID)
		if verr == nil {
			cachedVersions = versions
		}
		if verr != nil {
			slog.Warn("failed to get MR versions for incremental review, falling back to full review", "error", verr)
		} else if len(versions) > 1 {
			// Compare previous version's head to current version's head.
			prevHead := versions[1].HeadSHA
			currHead := versions[0].HeadSHA
			changedFiles, cerr := r.glClient.CompareCommits(ctx, r.cfg.CIProjectID, prevHead, currHead)
			if cerr != nil {
				slog.Warn("failed to compare commits for incremental review, falling back to full review", "error", cerr)
			} else {
				before := len(diffs)
				incrementalChangedFiles = changedFiles
				diffs = filterByFiles(diffs, changedFiles)
				slog.Info("incremental review",
					"total_files", before,
					"changed_files", len(changedFiles),
					"reviewing", len(diffs),
					"preserved", before-len(diffs),
				)
			}
		} else {
			slog.Info("first push to MR, performing full review")
		}
	}

	if len(diffs) == 0 {
		slog.Info("no files to review after incremental filtering")
		fmt.Println("✅ No reviewable files changed in latest push.")
		return 0, nil
	}

	// Step 2c: Sort files by review priority (security-sensitive first).
	diff.SortByPriority(diffs)

	// Step 2d: Pre-flight budget check.
	estimate := EstimateCost(diffs)
	if r.cfg.MaxTokens > 0 && estimate.TotalEstimate > r.cfg.MaxTokens {
		diffs, skippedFiles = TrimToBudget(diffs, r.cfg.MaxTokens)
		estimate = EstimateCost(diffs) // Recalculate after trim.
	}
	LogBudgetStatus(estimate, r.cfg.MaxTokens, skippedFiles)

	if len(diffs) == 0 {
		slog.Warn("all files trimmed by token budget")
		fmt.Println("⚠️  Token budget too low to review any files. Increase --max-tokens.")
		return 0, nil
	}

	// Filter custom rules by changed file paths.
	reviewFilePaths := make([]string, len(diffs))
	for i, d := range diffs {
		reviewFilePaths[i] = d.NewPath
	}
	applicableRules := config.FilterRulesByPaths(r.cfg.Rules, reviewFilePaths)
	extraRules := r.cfg.ExtraRules
	formattedRules := config.FormatRulesPrompt(applicableRules)
	if formattedRules != "" {
		if extraRules != "" {
			extraRules += "\n\n"
		}
		extraRules += formattedRules
		slog.Info(fmt.Sprintf("applying %d/%d custom rules", len(applicableRules), len(r.cfg.Rules)))
	}

	// Step 2e: Cache lookup.
	var cachedFindings []model.Finding
	var cacheKeys map[string]string
	if r.cache != nil && len(diffs) > 0 {
		promptHash := cache.PromptHash(r.cfg.CustomPrompt, r.cfg.Focus, r.cfg.ExtraRules, config.FormatRulesPrompt(applicableRules))
		diffs, cachedFindings, cacheHits, cacheKeys = cache.Partition(diffs, r.cache, r.cfg.Model, promptHash)
		if cacheHits > 0 {
			slog.Info("cache", "hits", cacheHits, "cached_findings", len(cachedFindings), "uncached_files", len(diffs))
		}
	}

	if len(diffs) == 0 && len(cachedFindings) == 0 {
		slog.Info("no findings from cache and no files to review")
		return 0, nil
	}

	// Step 3: Check context window / chunk.
	tokenLimit := diff.TokenLimitForModel(r.cfg.Model)
	chunker, err := diff.NewChunkStrategy(string(r.cfg.ChunkStrategy))
	if err != nil {
		return 0, err
	}

	var chunks [][]diff.FileDiff
	if len(diffs) > 0 {
		chunks, err = chunker.Chunk(diffs, tokenLimit)
		if err != nil {
			return 0, err
		}
		slog.Info(fmt.Sprintf("review split into %d chunk(s)", len(chunks)))
	}

	// Step 3b: Discover related code context.
	var contextSnippets []model.ContextSnippet
	if r.contextProvider != nil {
		repoRoot := findRepoRoot()
		if repoRoot != "" {
			raw, cerr := r.contextProvider.FindRelatedCode(ctx, repoRoot, diffs)
			if cerr != nil {
				slog.Warn("context discovery failed, continuing without context", "error", cerr)
			} else if len(raw) > 0 {
				for _, s := range raw {
					contextSnippets = append(contextSnippets, model.ContextSnippet{
						File:    s.File,
						Line:    s.Line,
						Content: s.Content,
						Symbol:  s.Symbol,
					})
				}
				slog.Info(fmt.Sprintf("found %d related code snippet(s)", len(contextSnippets)))
			}
		}
	}

	// Pass 1: Intent inference (if enabled).
	var intentContext string
	var intentSummary *model.SummaryResult
	if r.cfg.IntentReview {
		if sp, ok := r.provider.(model.SummarizeProvider); ok {
			fullDiff := buildNumberedDiff(diffs)
			sysPrompt := model.BuildSummaryPrompt()
			uPrompt := model.BuildSummaryUserPrompt(mrTitle, mrDesc, fullDiff)
			summaryResult, serr := sp.Summarize(ctx, sysPrompt, uPrompt)
			if serr != nil {
				slog.Warn("intent inference failed, falling back to standard review", "error", serr)
			} else {
				intentSummary = summaryResult
				intentContext = model.BuildIntentContext(summaryResult)
				slog.Info("intent inferred",
					"classification", summaryResult.Classification,
					"intent", summaryResult.Intent,
					"risk", summaryResult.RiskLevel,
				)
				if summaryResult.Usage != nil {
					totalUsage.InputTokens += summaryResult.Usage.InputTokens
					totalUsage.OutputTokens += summaryResult.Usage.OutputTokens
					totalUsage.TotalTokens += summaryResult.Usage.TotalTokens
				}
			}
		} else {
			slog.Warn("intent review enabled but provider does not support summarization, skipping pass 1")
		}
	}

	// Step 4: Build prompt and call model for each chunk.
	// In CI mode, source REVIEW.md from the trusted base/target ref so that
	// contributor-controlled branches cannot inject review instructions.
	reviewMD := r.cfg.ReviewMD
	if r.cfg.CIMode && r.cfg.CIDiffBaseSHA != "" {
		reviewMD = readReviewMDFromRef(r.cfg.CIDiffBaseSHA)
	}

	systemPrompt := model.BuildPromptFull(r.cfg.CustomPrompt, reviewMD, r.cfg.Focus, extraRules, intentContext)
	var summary string

	budgetExceeded := false
	anyTruncated := false

	for i, chunk := range chunks {
		slog.Info(fmt.Sprintf("reviewing chunk %d/%d (%d files, ~%d tokens)",
			i+1, len(chunks), len(chunk), diff.EstimateTokens(chunk)))

		numberedDiff := buildNumberedDiff(chunk)
		userPrompt := model.BuildUserPromptWithContext(mrTitle, mrDesc, numberedDiff, contextSnippets)

		result, err := r.provider.Review(ctx, systemPrompt, userPrompt)
		if err != nil {
			return 0, fmt.Errorf("model review (chunk %d): %w", i+1, err)
		}

		if summary == "" {
			summary = result.Summary
		}
		allFindings = append(allFindings, result.Findings...)
		if result.Usage != nil {
			totalUsage.InputTokens += result.Usage.InputTokens
			totalUsage.OutputTokens += result.Usage.OutputTokens
			totalUsage.TotalTokens += result.Usage.TotalTokens
		}
		if result.Truncated {
			anyTruncated = true
		}

		// Runtime budget safety net — stop if actual usage exceeds limit.
		if r.cfg.MaxTokens > 0 && totalUsage.TotalTokens >= int64(r.cfg.MaxTokens) {
			slog.Warn("runtime token budget exceeded, stopping review",
				"used", totalUsage.TotalTokens,
				"limit", r.cfg.MaxTokens,
				"chunks_completed", i+1,
				"chunks_total", len(chunks))
			budgetExceeded = true
			break
		}
	}

	if totalUsage.TotalTokens > 0 {
		slog.Info("token usage",
			"input", totalUsage.InputTokens,
			"output", totalUsage.OutputTokens,
			"total", totalUsage.TotalTokens,
		)
	}

	// Step 5: Validate line references.
	allFindings = ValidateFindings(allFindings, diffs)

	// Step 5b: Deduplicate findings across chunks.
	preDedupCount := len(allFindings)
	allFindings = DeduplicateFindings(allFindings)
	dedupedCount = preDedupCount - len(allFindings)

	// Step 5c: Store fresh findings in cache.
	if r.cache != nil && cacheKeys != nil {
		// Group findings by file.
		byFile := make(map[string][]model.Finding)
		for _, f := range allFindings {
			byFile[f.File] = append(byFile[f.File], f)
		}
		
		for _, d := range diffs { // diffs now holds only uncached
			if key, ok := cacheKeys[d.NewPath]; ok {
				entry := cache.Entry{
					FilePath: d.NewPath,
					DiffHash: cache.DiffHash(d),
					Model:    r.cfg.Model,
					Findings: byFile[d.NewPath],
				}
				if err := r.cache.Store(key, entry); err != nil {
					slog.Debug("failed to cache findings", "file", d.NewPath, "error", err)
				}
			}
		}
	}

	// Merge cached findings.
	allFindings = append(cachedFindings, allFindings...)

	// Step 6: Filter by severity.
	// Preserve raw count for auto-approve safety: approval must consider ALL
	// findings regardless of severity filter, not just the displayed subset.
	rawFindingsCount := len(allFindings)
	allFindings = filterBySeverity(allFindings, r.cfg.MinSeverity)
	slog.Info(fmt.Sprintf("%d finding(s) at or above %s severity", len(allFindings), r.cfg.MinSeverity))

	result := &model.ReviewResult{
		Summary:  summary,
		Findings: allFindings,
	}
	if totalUsage.TotalTokens > 0 {
		result.Usage = &totalUsage
	}

	// Step 7a: Write SARIF if requested (before posting to GitLab so it's not
	// skipped when PostReview fails).
	if r.cfg.SARIFOutput != "" {
		if err := WriteSARIF(r.cfg.SARIFOutput, result, "dev"); err != nil {
			return len(allFindings), fmt.Errorf("writing SARIF: %w", err)
		}
		slog.Info("SARIF output written", "path", r.cfg.SARIFOutput)
	}

	// Step 7b: Output.
	if r.cfg.DryRun || !r.cfg.CIMode {
		if r.cfg.OutputJSON {
			jsonOut, err := json.MarshalIndent(result, "", "  ")
			if err != nil {
				return 0, fmt.Errorf("marshaling JSON output: %w", err)
			}
			fmt.Println(string(jsonOut))
		} else {
			useColor := !r.cfg.NoColor && isTTY()
			fmt.Print(formatIntentOneLiner(intentSummary, useColor))
			fmt.Print(ColorTerminalOutput(result, useColor))
		}
	} else {
		// Post to GitLab.
		var version *vcs.DiffVersion
		if r.cfg.CommentMode == config.CommentModeDiscussions {
			if len(cachedVersions) > 0 {
				version = &cachedVersions[0]
			} else {
				versions, err := r.glClient.GetMRVersions(ctx, r.cfg.CIProjectID, r.cfg.CIMergeRequestID)
				if err != nil {
					slog.Warn("could not fetch MR versions, inline comments may fail", "error", err)
				} else if len(versions) > 0 {
					version = &versions[0]
				}
			}
		}

		// Prepend intent markdown to the summary note if intent was inferred.
		if intentSummary != nil {
			result.Summary = formatIntentMarkdown(intentSummary) + result.Summary
		}

		// Prepend scope markdown if MR is oversized.
		if scopeAssessment != nil && scopeAssessment.IsOversized {
			result.Summary = FormatScopeMarkdown(scopeAssessment) + result.Summary
		}

		if err := PostReview(ctx, r.cfg, r.glClient, result, version, incrementalChangedFiles); err != nil {
			return len(allFindings), fmt.Errorf("posting review: %w", err)
		}

		// Step 7c: Auto-approve if all safety guards pass.
		if r.cfg.AutoApprove {
			// Include parse-failed files as unreviewed — they were never
			// sent to the model, so approval would be incomplete.
			allSkipped := append(skippedFiles, r.parseFailedFiles...)
			decision := EvaluateAutoApprove(r.cfg, len(diffs), rawFindingsCount,
				allSkipped, budgetExceeded,
				scopeAssessment != nil && scopeAssessment.IsOversized,
				anyTruncated, r.mrDraft)
			if decision.Approved {
				if approver, ok := r.glClient.(vcs.VCSApprover); ok {
					// Pin approval to the reviewed HEAD SHA to prevent
					// approving unreviewed pushes that land between
					// diff retrieval and approval.
					// version may be nil when CommentMode is "notes",
					// so fetch versions specifically for the SHA if needed.
					headSHA := ""
					if version != nil {
						headSHA = version.HeadSHA
					} else if len(cachedVersions) > 0 {
						headSHA = cachedVersions[0].HeadSHA
					} else {
						vers, verr := r.glClient.GetMRVersions(ctx, r.cfg.CIProjectID, r.cfg.CIMergeRequestID)
						if verr == nil && len(vers) > 0 {
							headSHA = vers[0].HeadSHA
						}
					}
					if headSHA == "" {
						slog.Warn("auto-approve skipped: could not determine reviewed HEAD SHA")
						fmt.Println("ℹ️  Auto-approve skipped: could not determine reviewed HEAD SHA")
					} else {
						if err := approver.ApproveReview(ctx, r.cfg.CIProjectID, r.cfg.CIMergeRequestID, headSHA); err != nil {
							return len(allFindings), fmt.Errorf("auto-approve failed: %w\n\nEnsure your token has the required permissions:\n  GitHub: 'pull-requests: write'\n  GitLab: 'api' scope", err)
						}
						slog.Info("auto-approved MR/PR", "reason", decision.Reason, "head_sha", headSHA)
						fmt.Printf("✅ %s\n", decision.Reason)
					}
				} else {
					slog.Warn("auto-approve enabled but VCS client does not support approvals")
				}
			} else {
				slog.Info("auto-approve skipped", "reason", decision.Reason)
				fmt.Printf("ℹ️  Auto-approve skipped: %s\n", decision.Reason)
			}
		}
	}

	// Step 8: Apply fixes if requested.
	if r.cfg.Fix && len(allFindings) > 0 {
		repoRoot := findRepoRoot()
		fixes := ApplyFixes(allFindings, repoRoot)
		useColor := !r.cfg.NoColor && isTTY()
		summary := FormatFixSummary(fixes, useColor)
		if r.cfg.OutputJSON {
			// Keep stdout machine-readable; emit fix summary on stderr.
			fmt.Fprint(os.Stderr, summary)
		} else {
			fmt.Print(summary)
		}
	}



	return len(allFindings), nil
}

func (r *Reviewer) getDiffs(ctx context.Context) ([]diff.FileDiff, string, string, error) {
	switch {
	case r.cfg.CIMode:
		return r.getCIDiffs(ctx)
	case r.cfg.DiffMode:
		return r.getLocalDiffs()
	case len(r.cfg.Files) > 0:
		return r.getFileDiffs()
	default:
		return nil, "", "", fmt.Errorf("no input mode specified")
	}
}

func (r *Reviewer) getCIDiffs(ctx context.Context) ([]diff.FileDiff, string, string, error) {
	mr, err := r.glClient.GetMRChanges(ctx, r.cfg.CIProjectID, r.cfg.CIMergeRequestID)
	if err != nil {
		return nil, "", "", err
	}

	r.mrDraft = mr.Draft

	// Check if draft and should skip.
	if r.cfg.SkipDraftMRs && mr.Draft {
		return nil, "", "", fmt.Errorf("skipping draft MR")
	}

	// Parse each file's diff.
	var diffs []diff.FileDiff
	for _, change := range mr.Changes {
		parsed, err := diff.Parse(strings.NewReader("diff --git a/" + change.OldPath + " b/" + change.NewPath + "\n" + change.Diff))
		if err != nil {
			slog.Warn("failed to parse diff for file", "file", change.NewPath, "error", err)
			r.parseFailedFiles = append(r.parseFailedFiles, change.NewPath)
			continue
		}
		diffs = append(diffs, parsed...)
	}

	return diffs, mr.Title, mr.Description, nil
}

func (r *Reviewer) getLocalDiffs() ([]diff.FileDiff, string, string, error) {
	ref := r.cfg.DiffRef
	if ref == "" {
		ref = "origin/HEAD"
	}

	// Prevent command injection: reject refs that look like flags.
	if strings.HasPrefix(ref, "-") {
		return nil, "", "", fmt.Errorf("invalid diff ref %q: must not start with '-'", ref)
	}

	// Use '--' to terminate flag parsing, preventing ref from being interpreted as a git option.
	cmd := exec.Command("git", "diff", "-U5", "--merge-base", ref, "--")
	output, err := cmd.Output()
	if err != nil {
		// Fallback: try without --merge-base.
		cmd = exec.Command("git", "diff", "-U5", ref, "--")
		output, err = cmd.Output()
		if err != nil {
			return nil, "", "", fmt.Errorf("running git diff: %w", err)
		}
	}

	diffs, err := diff.Parse(strings.NewReader(string(output)))
	if err != nil {
		return nil, "", "", fmt.Errorf("parsing git diff: %w", err)
	}

	return diffs, "", "", nil
}

func (r *Reviewer) getFileDiffs() ([]diff.FileDiff, string, string, error) {
	// Validate file paths: reject any that look like flags.
	for _, f := range r.cfg.Files {
		if strings.HasPrefix(f, "-") {
			return nil, "", "", fmt.Errorf("invalid file path %q: must not start with '-'", f)
		}
	}

	args := append([]string{"diff", "-U5", "HEAD", "--"}, r.cfg.Files...)
	cmd := exec.Command("git", args...)
	output, err := cmd.Output()
	if err != nil {
		return nil, "", "", fmt.Errorf("running git diff for files: %w", err)
	}

	diffs, err := diff.Parse(strings.NewReader(string(output)))
	if err != nil {
		return nil, "", "", fmt.Errorf("parsing file diffs: %w", err)
	}

	return diffs, "", "", nil
}

// buildNumberedDiff creates a numbered diff representation for the model prompt.
// Line numbers help the model reference specific lines accurately.
func buildNumberedDiff(diffs []diff.FileDiff) string {
	var sb strings.Builder

	for _, d := range diffs {
		path := d.NewPath
		if path == "" {
			path = d.OldPath
		}
		fmt.Fprintf(&sb, "=== File: %s ===\n", path)

		for _, h := range d.Hunks {
			sb.WriteString(h.Header + "\n")
			for _, l := range h.Lines {
				prefix := " "
				lineNo := l.NewLineNo
				switch l.Type {
				case diff.LineAdded:
					prefix = "+"
					lineNo = l.NewLineNo
				case diff.LineRemoved:
					prefix = "-"
					lineNo = l.OldLineNo
				case diff.LineContext:
					lineNo = l.NewLineNo
				}
				fmt.Fprintf(&sb, "%4d %s %s\n", lineNo, prefix, l.Content)
			}
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func filterBySeverity(findings []model.Finding, minSeverity config.Severity) []model.Finding {
	if minSeverity == config.SeverityLow {
		return findings // No filtering needed.
	}

	var filtered []model.Finding
	for _, f := range findings {
		sev, err := config.ParseSeverity(f.Severity)
		if err != nil {
			// Include findings with unknown severity.
			filtered = append(filtered, f)
			continue
		}
		if sev >= minSeverity {
			filtered = append(filtered, f)
		}
	}
	return filtered
}

// filterByFiles returns only diffs whose file path matches one of the changed files.
func filterByFiles(diffs []diff.FileDiff, changedFiles []string) []diff.FileDiff {
	changed := make(map[string]bool, len(changedFiles))
	for _, f := range changedFiles {
		changed[f] = true
	}
	var filtered []diff.FileDiff
	for _, d := range diffs {
		path := d.NewPath
		if path == "" {
			path = d.OldPath
		}
		if changed[path] {
			filtered = append(filtered, d)
		}
	}
	return filtered
}

// findRepoRoot walks up from cwd to find the git repository root.
func findRepoRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// readReviewMDFromRef reads REVIEW.md from a specific git ref (e.g. base commit SHA).
// Returns empty string if the file doesn't exist at that ref or git fails.
func readReviewMDFromRef(ref string) string {
	// Prevent command injection: reject refs that look like flags.
	if strings.HasPrefix(ref, "-") {
		slog.Warn("invalid git ref for REVIEW.md lookup, skipping", "ref", ref)
		return ""
	}
	cmd := exec.Command("git", "show", ref+":REVIEW.md")
	output, err := cmd.Output()
	if err != nil {
		// File doesn't exist at this ref — this is normal and expected.
		slog.Debug("REVIEW.md not found at base ref", "ref", ref)
		return ""
	}
	content := strings.TrimSpace(string(output))
	if content != "" {
		slog.Info("loaded REVIEW.md from trusted base ref", "ref", ref[:min(len(ref), 12)])
	}
	return content
}

func initCache(cfg *config.Config) *cache.Cache {
	if cfg.NoCache {
		return nil
	}
	c, err := cache.New(cfg.CacheDir, cfg.CacheMaxAge)
	if err != nil {
		slog.Warn("failed to initialize cache", "error", err)
		return nil
	}
	return c
}
