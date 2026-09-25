# Dockerfile Review Checklist

## Focus
- **Security**: Running as root user, hardcoded secrets in ENV/ARG.
- **Best Practices**: COPY vs ADD misuse, missing multi-stage builds.
- **Caching**: Layer caching issues (COPY before RUN).
- **Reliability**: Unpinned base images, missing HEALTHCHECK.

## DO NOT Flag
- Formatting.
- Comment style.
