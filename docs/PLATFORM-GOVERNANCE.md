# Platform Governance & Multi-File Configuration

In enterprise platforms (such as healthcare, financial services, or multitenant cloud environments), code review governance requires a strict separation of concerns:

- **Platform Engineering & Security Teams**: Enforce non-negotiable security floors, regulatory compliance (e.g. HIPAA PHI masking), zero-trust architecture, and tenant isolation across all repositories.
- **Service Teams**: Define service-level domain guidelines, style preferences, and repository-specific custom rules.

OpticDiff `code-reviewer` natively composes platform mandates with repository guidelines in a **single evaluation pass**, eliminating duplicate CI runs and duplicate LLM token costs.

---

## Dual-Review CI Architecture (Platform Gate vs. Product Quality Review)

### Overview
The `--profile platform|product|all` flag enables running two independent, isolated reviews per PR/MR:
- **Platform review** (`--profile platform`): Enforces enterprise compliance, security invariants, and platform architecture rules. Ignores repo-level `.code-reviewer.yaml` and `REVIEW.md`. Cannot be influenced by product teams.
- **Product review** (`--profile product`): Focuses on code quality, bugs, readability, and domain conventions. Loads repo-level config. Does not load platform governance.
- **Unified review** (`--profile all`, default): Existing composite behavior for backward compatibility.

### Exit Code Contract
| Exit Code | Meaning |
|---|---|
| 0 | Review completed, no blocking findings |
| 1 | Review completed, blocking findings present |
| 2 | Configuration error (invalid profile, missing platform config) |
| 3 | Infrastructure error (LLM unreachable, VCS API failure) |

### Fail-Closed Guarantee
When `--profile platform` cannot load its governance rules (missing config, empty rules), it fails with exit code 2 rather than silently passing with zero findings.

### GitHub Actions Example
```yaml
jobs:
  platform-review:
    runs-on: ubuntu-latest
    permissions:
      pull-requests: write
      security-events: write
    steps:
      - uses: actions/checkout@v4
      - uses: OpticDiff/code-reviewer-action@v1
        with:
          profile: platform
          platform-config: 'path/to/platform-rules/*.yaml'
          platform-review-md: 'path/to/platform-guidelines/*.md'
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          GOOGLE_CLOUD_PROJECT: ${{ vars.GCP_PROJECT }}

  product-review:
    runs-on: ubuntu-latest
    permissions:
      pull-requests: write
    steps:
      - uses: actions/checkout@v4
      - uses: OpticDiff/code-reviewer-action@v1
        with:
          profile: product
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          GOOGLE_CLOUD_PROJECT: ${{ vars.GCP_PROJECT }}
```

### GitLab CI Example (Pipeline Execution Policy)
```yaml
# Platform review — injected centrally via Pipeline Execution Policy
platform-review:
  image: ghcr.io/opticdiff/code-reviewer:latest
  script:
    - code-reviewer --ci --profile platform
  variables:
    CODE_REVIEW_PLATFORM_CONFIG: "$CI_PROJECT_DIR/.platform/rules/*.yaml"
  rules:
    - if: $CI_PIPELINE_SOURCE == "merge_request_event"
```

### Comment Isolation
- Platform comments use marker `<!-- code-reviewer:platform -->`
- Product comments use marker `<!-- code-reviewer:product -->`
- Each profile's cleanup only touches its own comments — zero cross-deletion

### SARIF Categories
- Platform: `code-reviewer/platform` driver name in SARIF
- Product: `code-reviewer/product` driver name in SARIF
- Appear as separate tools in GitHub Security tab

### Per-Profile Model Override
Use `--platform-model` and `--product-model` to assign different models to each profile:

```yaml
# Platform uses a premium model for deep security analysis
- uses: OpticDiff/code-reviewer-action@v1
  with:
    profile: platform
    platform-model: gemini-2.5-pro

# Product uses a faster model for code quality
- uses: OpticDiff/code-reviewer-action@v1
  with:
    profile: product
    product-model: gemini-2.5-flash
```

Environment variables: `CODE_REVIEW_PLATFORM_MODEL`, `CODE_REVIEW_PRODUCT_MODEL`.

### Platform Visibility

Control whether platform compliance findings are visible to all PR participants or restricted:

```bash
--platform-visibility public              # Default: visible to everyone
--platform-visibility security-team-only  # Minimize info in PR comments
```

When set to `security-team-only`, platform findings are recorded in SARIF and the audit log but the PR comment summary omits detailed vulnerability descriptions. Environment variable: `CODE_REVIEW_PLATFORM_VISIBILITY`.

---

## 1. Multi-File Organization

Rather than maintaining a single monolithic configuration, platform teams can modularize rules and guidelines across multiple files and directories:

```
azra/platform/ci-templates/code-reviewer/
├── rules/
│   ├── 01-hipaa-phi.yaml         # Compliance: SSN, MRN, patient data masking
│   ├── 02-multitenancy.yaml       # Architecture: tenant_id filtering on SQL/gRPC
│   └── 03-auth-zero-trust.yaml    # Security: mTLS, token propagation, no plain API keys
└── guidelines/
    ├── COMPLIANCE.md              # Organization-wide compliance instructions
    └── ARCHITECTURE.md            # Platform infrastructure and design standards
```

---

## 2. Rule Schema Reference

Each YAML file in `rules/` defines rules and an optional `min_severity` floor:

```yaml
# Optional: Platform-mandated severity floor.
# Service repositories cannot loosen this threshold.
min_severity: medium

rules:
  - name: mask-patient-identifiers
    description: "Ensure patient PHI (SSN, MRN, email) is masked with phi.Mask() before logging"
    category: security
    severity: critical
    url: "https://docs.azra.internal/standards/phi-masking"
    allow_suppression: false # Denied: cannot be bypassed via inline comments

  - name: require-tenant-context
    description: "Direct SQL queries and store methods must accept and filter by tenant_id"
    category: security
    severity: high
    paths:
      - "internal/db/**/*.go"
      - "pkg/store/**/*.go"
    url: "https://docs.azra.internal/standards/tenancy"
    allow_suppression: true # Allowed: can be suppressed with valid justification
```

### Rule Fields

| Field | Type | Description |
|---|---|---|
| `name` | `string` | Unique alphanumeric identifier for the rule (e.g., `mask-patient-identifiers`). |
| `description` | `string` | Clear instruction given to the AI reviewer explaining what to look for. |
| `category` | `string` | Finding category: `bug`, `security`, `performance`, `style`. |
| `severity` | `string` | Enforced severity: `low`, `medium`, `high`, `critical`. |
| `paths` | `[]string` | Optional glob patterns limiting the rule to specific files (e.g. `internal/db/**/*.go`). |
| `url` | `string` | Optional link to internal documentation or runbook rendered in review comments. |
| `allow_suppression`| `bool` | Whether developers can suppress the rule using inline comments. **Defaults to `false` for platform rules** and `true` for repository rules. |

---

## 3. How Multi-File Merging Works

`code-reviewer` accepts multi-file configurations via CLI flags, environment variables, or repository YAML:

- **CLI Flags**: `--platform-config` and `--platform-review-md`
- **Environment Variables**: `CODE_REVIEW_PLATFORM_CONFIG` (or `CODE_REVIEWER_PLATFORM_CONFIG`) and `CODE_REVIEW_PLATFORM_REVIEW_MD`
- **Repository YAML**: `platform_config` and `platform_review_md` in `.code-reviewer.yaml`

### Syntax Options

1. **Glob Patterns**:
   ```bash
   code-reviewer --ci \
     --platform-config "rules/*.yaml" \
     --platform-review-md "guidelines/*.md"
   ```
2. **Comma-Separated File Lists**:
   ```bash
   code-reviewer --ci \
     --platform-config "rules/01-hipaa.yaml,rules/02-tenancy.yaml" \
     --platform-review-md "guidelines/security.md,guidelines/architecture.md"
   ```
3. **Combined Multi-Directory Globs**:
   ```bash
   code-reviewer --ci \
     --platform-config "/etc/platform/rules/*.yaml,./team-policies/*.yaml"
   ```

### Deterministic Merging Guarantees

1. **Lexicographical Sorting**: All expanded glob matches are sorted alphabetically (`sort.Strings()`) before loading. This guarantees identical prompt structure and identical cache keys across macOS, Linux, and container runners.
2. **Duplicate Detection & Conflict Precedence**:
   - If two platform files contain a rule with the same name, the parser errors immediately with `duplicate rule name "<name>" in "<file>"`.
   - If a repository's `.code-reviewer.yaml` defines a rule with the same name as a platform rule, the **platform rule takes precedence**, and the repository rule is ignored with a warning log:
     ```
     WARN ignoring repo rule that conflicts with mandatory platform rule rule=mask-patient-identifiers
     ```
3. **Monotonic Severity Floor**:
   - If any loaded platform config specifies `min_severity: medium`, and a repository's `.code-reviewer.yaml` or CLI sets `--min-severity high` (a looser filter), the engine automatically clamps the threshold to `medium`. Platform compliance issues are never silenced by repository settings.
4. **Markdown Guidelines Concatenation**:
   - Multiple guidelines matching `--platform-review-md` are concatenated into a unified document with clear section headers (`### Platform Policy (<filename>)`).
   - Platform guidelines are placed at the highest authority tier in the system prompt, immediately above the immutable output constraints.

---

## 4. Platform-Specific CI/CD Integration

### Option A: GitLab CI Pipeline Execution Policy (Central Enforced)

In GitLab Ultimate (Security & Compliance → Policies), configure a Pipeline Execution Policy to inject this job into all projects across your group. Service repositories cannot remove or alter this job.

```yaml
# templates/code-reviewer.gitlab-ci.yml
code-review-platform:
  stage: test
  image:
    name: ghcr.io/opticdiff/code-reviewer:v0.10.0
    entrypoint: [""]
  rules:
    - if: '$CI_PIPELINE_SOURCE == "merge_request_event"'
  before_script:
    - mkdir -p /tmp/platform-governance
    - curl --proto '=https' -sSf --header "JOB-TOKEN: $CI_JOB_TOKEN" "$PLATFORM_RULES_BUNDLE_URL" -o /tmp/platform-rules.tar.gz
    - tar -xzf /tmp/platform-rules.tar.gz -C /tmp/platform-governance/
  script:
    - code-reviewer --ci \
        --platform-config "/tmp/platform-governance/rules/*.yaml" \
        --platform-review-md "/tmp/platform-governance/guidelines/*.md"
```

### Option B: GitHub Actions Reusable Workflow

In GitHub Enterprise, create an organization-wide reusable workflow (e.g. `org/.github/.github/workflows/code-review.yml`):

```yaml
name: Platform Code Review
on:
  workflow_call:

jobs:
  review:
    runs-on: ubuntu-latest
    permissions:
      contents: read
      pull-requests: write
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0 # Full history for diff base ref resolution

      - name: Download Platform Rules
        run: |
          mkdir -p /tmp/platform-governance
          curl --proto '=https' -sSf -H "Authorization: Bearer ${{ secrets.PLATFORM_READ_TOKEN }}" \
            "https://api.github.com/repos/org/platform-standards/tarball/main" | tar -xz -C /tmp/platform-governance --strip-components=1

      - uses: OpticDiff/code-reviewer-action@v1
        with:
          extra-args: >-
            --platform-config "/tmp/platform-governance/rules/*.yaml"
            --platform-review-md "/tmp/platform-governance/guidelines/*.md"
        env:
          GOOGLE_CLOUD_PROJECT: ${{ secrets.GCP_PROJECT }}
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

### Option C: Baked into Internal Platform Container Image

For air-gapped or strictly regulated Kubernetes/Nix runners, bake platform standards directly into the container image:

```dockerfile
FROM ghcr.io/opticdiff/code-reviewer:v0.10.0
COPY rules/ /etc/code-reviewer/rules/
COPY guidelines/ /etc/code-reviewer/guidelines/
ENV CODE_REVIEW_PLATFORM_CONFIG="/etc/code-reviewer/rules/*.yaml"
ENV CODE_REVIEW_PLATFORM_REVIEW_MD="/etc/code-reviewer/guidelines/*.md"
```

### Option D: In-Repo Protected by `CODEOWNERS`

If platform configs reside directly inside service repositories (e.g., `.platform/`), protect them using `CODEOWNERS` and repository branch rulesets:

```text
# .github/CODEOWNERS or .gitlab/CODEOWNERS
*                              @azra/patient-service-team
.platform/                     @invenero/platform-eng
```

> [!IMPORTANT]
> **Zero-Trust CI Sourcing**: When running in CI mode, `code-reviewer` automatically resolves in-repo platform files from the base commit (`CIDiffBaseSHA`) via `git show`. If a contributor modifies or deletes platform rules on a feature branch, the engine ignores the modified checkout and strictly evaluates rules from the trusted base ref.

---

## 5. How Findings Look in Pull Requests

Findings triggered by platform rules are deterministically badged with their originating policy and documentation link:

> 🔴 **[CRITICAL]** 🛡️ **[Platform Policy: 01-hipaa-phi.yaml / mask-patient-identifiers]** Cleartext SSN logged to stdout
> 
> Direct logging of patient SSN detected on line 42. All patient identifiers must be masked using `phi.Mask(ssn)`.
> 
> 📖 **Documentation**: [mask-patient-identifiers](https://docs.azra.internal/standards/phi-masking)

Findings triggered by service-level repository rules are badged as custom rules:

> 🟡 **[MEDIUM]** 📋 **[Rule: grpc-context-timeout]** Missing context timeout on outbound gRPC call
> 
> Outbound client call does not specify a context with deadline.

---

## 6. Inline Suppressions & Audit Trail

When a rule permits suppression (`allow_suppression: true`), developers can add an inline comment on or adjacent to the flagged line:

```go
// opticdiff:ignore require-tenant-context: internal migration script runs with root context
rows, err := db.Query("SELECT * FROM patients")
```

```python
# opticdiff:ignore require-tenant-context: batch backfill script
rows = db.execute("SELECT * FROM patients")
```

- **Syntax**: `// opticdiff:ignore <rule-name>: <justification>` or `# opticdiff:ignore <rule-name>: <justification>`
- **Suppression of All Rules**: `// opticdiff:ignore all: emergency hotfix approved by lead`
- **Default Behavior**: Platform rules **deny** suppression by default (`allow_suppression: false`) unless explicitly configured with `allow_suppression: true`.
- **Auditing**: Suppressed findings do not block CI, but are recorded in the structured JSONL audit log (`REVIEW_AUDIT_LOG`) with the author's justification, rule name, and SHA-256 file hashes:
  ```json
  {
    "timestamp": "2026-09-08T03:45:20Z",
    "platform_rules_count": 5,
    "platform_rule_hashes": {
      "01-hipaa-phi.yaml": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
      "02-multitenancy.yaml": "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9"
    }
  }
  ```
- **Denied Suppressions**: If a developer attempts to suppress a rule where `allow_suppression` is not `true`, the comment is rejected and a warning is logged:
  ```
  WARN inline suppression denied for mandatory platform rule file=main.go line=42 rule=mask-patient-identifiers
  ```
