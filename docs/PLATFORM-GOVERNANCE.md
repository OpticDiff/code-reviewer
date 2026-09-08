# Platform Governance & Multi-File Configuration

In enterprise platforms (such as healthcare, financial services, or multitenant cloud environments), code review governance requires a strict separation of concerns:

- **Platform Engineering & Security Teams**: Enforce non-negotiable security floors, regulatory compliance (e.g. HIPAA PHI masking), zero-trust architecture, and tenant isolation across all repositories.
- **Service Teams**: Define service-level domain guidelines, style preferences, and repository-specific custom rules.

OpticDiff `code-reviewer` natively composes platform mandates with repository guidelines in a **single evaluation pass**, eliminating duplicate CI runs and duplicate LLM token costs.

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
    allow_suppression: false # Cannot be bypassed via inline comments

  - name: require-tenant-context
    description: "Direct SQL queries and store methods must accept and filter by tenant_id"
    category: security
    severity: high
    paths:
      - "internal/db/**/*.go"
      - "pkg/store/**/*.go"
    url: "https://docs.azra.internal/standards/tenancy"
    allow_suppression: true # Can be suppressed with valid justification
```

### Rule Fields

| Field | Type | Description |
|---|---|---|
| `name` | `string` | Unique alphanumeric identifier for the rule (e.g., `mask-patient-identifiers`). |
| `description` | `string` | Clear instruction given to the AI reviewer explaining what to look for. |
| `category` | `string` | Finding category: `bug`, `security`, `performance`, `style`, `docs`, `custom`. |
| `severity` | `string` | Enforced severity: `low`, `medium`, `high`, `critical`. |
| `paths` | `[]string` | Optional glob patterns limiting the rule to specific files (e.g. `internal/db/**/*.go`). |
| `url` | `string` | Optional link to internal documentation or runbook rendered in review comments. |
| `allow_suppression`| `bool` | Whether developers can suppress the rule using inline comments (defaults to `true`). |

---

## 3. Enforcement in CI/CD

### Option A: GitLab Pipeline Execution Policies (Recommended)
In GitLab (Security & Compliance → Policies), configure a Pipeline Execution Policy that injects the platform job into every project in the group:

```yaml
# templates/code-reviewer.gitlab-ci.yml
code-review-platform:
  stage: test
  image:
    name: ghcr.io/opticdiff/code-reviewer:v0.9.0
    entrypoint: [""]
  rules:
    - if: '$CI_PIPELINE_SOURCE == "merge_request_event"'
  before_script:
    # Fetch platform rules from central ci-templates using ephemeral job token
    - curl -sSf --header "JOB-TOKEN: $CI_JOB_TOKEN" "$PLATFORM_RULES_BUNDLE_URL" -o /tmp/platform-rules.tar.gz
    - tar -xzf /tmp/platform-rules.tar.gz -C /tmp/platform-governance/
  script:
    - code-reviewer --ci \
        --platform-config "/tmp/platform-governance/rules/*.yaml" \
        --platform-review-md "/tmp/platform-governance/guidelines/*.md"
```

### Option B: Baked into Internal Platform Container Image
For air-gapped or strictly regulated environments, bake the rules directly into your platform runner image:

```dockerfile
FROM ghcr.io/opticdiff/code-reviewer:v0.9.0
COPY rules/ /etc/code-reviewer/rules/
COPY guidelines/ /etc/code-reviewer/guidelines/
ENV CODE_REVIEWER_PLATFORM_CONFIG="/etc/code-reviewer/rules/*.yaml"
ENV CODE_REVIEWER_PLATFORM_REVIEW_MD="/etc/code-reviewer/guidelines/*.md"
```

### Option C: In-Repo Protected by `CODEOWNERS`
If platform configs live inside each repository, use `CODEOWNERS`:

```text
# .gitlab/CODEOWNERS
*                              @azra/patient-service-team
.code-reviewer.platform.yaml   @invenero/platform-eng
PLATFORM_REVIEW.md             @invenero/platform-eng
```

> [!NOTE]
> When running in CI mode, `code-reviewer` automatically loads in-repo `.code-reviewer.platform.yaml` and `PLATFORM_REVIEW.md` from the target branch (`CIDiffBaseSHA`), preventing untrusted MR branches from tampering with platform rules.

---

## 4. How Findings Look in the Merge Request

Findings triggered by platform rules are deterministically badged with their originating policy and documentation link:

> 🔴 **[CRITICAL]** 🛡️ **[Platform Policy: 01-hipaa-phi.yaml / mask-patient-identifiers]** Cleartext SSN logged to stdout
> 
> Direct logging of patient SSN detected on line 42. All patient identifiers must be masked using `phi.Mask(ssn)`.
> 
> 📖 **Documentation**: [mask-patient-identifiers](https://docs.azra.internal/standards/phi-masking)

---

## 5. Inline Suppressions & Audit Trail

When a rule allows suppression (`allow_suppression: true`), developers can add an inline comment on or adjacent to the flagged line:

```go
// opticdiff:ignore require-tenant-context: internal migration script runs with root context
rows, err := db.Query("SELECT * FROM patients")
```

- **Syntax**: `// opticdiff:ignore <rule-name>: <justification>` or `# opticdiff:ignore <rule-name>: <justification>`
- **Suppression of All Rules**: `// opticdiff:ignore all: emergency hotfix approved by lead`
- **Auditing**: Suppressed findings do not block CI, but are recorded in the structured JSONL audit log (`REVIEW_AUDIT_LOG`) with the author's justification and rule file checksums.
- **Forbidden Suppressions**: If a platform rule has `allow_suppression: false`, the ignore comment is rejected and a warning is logged.
