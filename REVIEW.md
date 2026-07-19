## Review Standards for code-reviewer

- All exported functions and types MUST have doc comments.
- Errors adding context to an existing error MUST wrap with `%w` for proper error chains.
- Never log or expose secrets, tokens, API keys, or auth headers.
- Flag any changes to adversarial content sections in prompts — these are security-critical.
- JSON parsing of model output MUST use `parseReviewJSON()` — never raw `json.Unmarshal`.
- New config fields require updates in ALL of: `Config` struct, `repoConfig` struct, `loadFlags()`, `applyRepoConfig()`, `.code-reviewer.example.yaml`, README flags table.
- Prefer table-driven tests. Mock via interfaces (`ModelReviewer`, `VCSClient`), not concrete types.
- Context must be propagated — flag any use of `context.TODO()` or `context.Background()` in non-init code.
