# Shell Script Review Checklist

## Focus
- **Injection**: Unquoted variables in command arguments (`rm -rf $DIR` vs `rm -rf "$DIR"`), eval with user input, command substitution without quoting.
- **Error Handling**: Missing `set -euo pipefail`, unchecked command exit codes, missing error traps.
- **Portability**: Bashisms in `#!/bin/sh` scripts, non-POSIX `[[ ]]` in portable scripts, GNU-specific flags.
- **Quoting**: Unquoted `$@`, word splitting in `for f in $(ls)`, glob expansion in variables.
- **Security**: World-writable temp files (`/tmp/foo` vs `mktemp`), secrets in command arguments (visible in `ps`), `curl | bash` patterns.
- **Robustness**: Missing `trap` for cleanup, hardcoded paths, assumptions about working directory.

## DO NOT Flag
- Style preferences (handled by `shellcheck` and `shfmt`).
- Comment style or density.
- ShellCheck findings that are already enforced in CI.
