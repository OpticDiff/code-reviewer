# Dockerfile Review Checklist

## Focus
- **Security**: Running as root user, hardcoded secrets in ENV/ARG.
- **Best Practices**: COPY vs ADD misuse, missing multi-stage builds (when the image includes build tools that shouldn't ship to production).
- **Caching**: Layer caching issues (COPY before RUN).
- **Reliability**: Unpinned base images, missing HEALTHCHECK (for long-running services, not CLI tools or init containers).

## DO NOT Flag
- Formatting.
- Comment style.
