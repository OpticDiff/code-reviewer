# syntax=docker/dockerfile:1

# Runtime image (~10MB).
# gcr.io/distroless/static-debian12
FROM gcr.io/distroless/static-debian12@sha256:6447365a6337c3732f412d1b74357b30a633831955b2bc45552b0086be907687

ARG TARGETPLATFORM
COPY $TARGETPLATFORM/code-reviewer /usr/local/bin/code-reviewer

ENTRYPOINT ["code-reviewer"]
