# Setting Up a Husky Pre-Push Hook with code-reviewer

Integrate `code-reviewer` with [Husky](https://typicode.github.io/husky/) to automatically inspect your changes for bugs, security vulnerabilities, and code quality issues before every `git push`.

---

## 1. Prerequisites

1. Install `code-reviewer`:
   ```bash
   # Homebrew
   brew install OpticDiff/tap/code-reviewer

   # mise
   mise use -g ubi:OpticDiff/code-reviewer

   # Go
   go install github.com/OpticDiff/code-reviewer/cmd/code-reviewer@latest
   ```

2. Ensure your local credentials are configured (e.g. `gcloud auth application-default login` for Vertex AI, or `--api-url` for local Ollama).

---

## 2. Install Husky

If Husky is not already installed in your JavaScript / TypeScript / Node project:

```bash
npm install husky --save-dev
npx husky init
```

---

## 3. Create the Pre-Push Hook

Create the `.husky/pre-push` hook file:

```bash
cat << 'EOF' > .husky/pre-push
#!/usr/bin/env sh
. "$(dirname "$0")/_/husky.sh"

echo "🔍 Running code-reviewer pre-push check..."

# Run code-reviewer against the upstream tracking branch.
# If no upstream branch is set, defaults to origin/main.
UPSTREAM=$(git rev-parse --abbrev-ref --symbolic-full-name @{u} 2>/dev/null || echo "origin/main")

code-reviewer --diff "$UPSTREAM" --min-severity high

EXIT_CODE=$?
if [ $EXIT_CODE -ne 0 ]; then
  echo "🚫 Push blocked by code-reviewer. Resolve high/critical findings or use --no-verify to bypass."
  exit $EXIT_CODE
fi
EOF

chmod +x .husky/pre-push
```

---

## 4. Test the Hook

Push a branch to trigger the hook:

```bash
git push origin feature/my-new-feature
```

Sample output:

```text
🔍 Running code-reviewer pre-push check...
  ❌ HIGH  internal/auth/handler.go:58
           Nil pointer dereference — token is used before nil check

🚫 Push blocked by code-reviewer. Resolve high/critical findings or use --no-verify to bypass.
```

To bypass the pre-push check during emergencies:

```bash
git push --no-verify
```
