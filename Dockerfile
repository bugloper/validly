# syntax=docker/dockerfile:1

# Validly — Simple email validation for every stack.
FROM golang:1.26.4-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY openapi ./openapi
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/validly ./cmd/validly

# Distroless static: no shell or package manager, runs as an unprivileged user,
# and ships CA certificates for SMTP STARTTLS and HTTPS list syncing.
FROM gcr.io/distroless/static-debian12:nonroot
ARG VERSION=dev
LABEL org.opencontainers.image.title="Validly" \
      org.opencontainers.image.description="Validly — Simple email validation for every stack." \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.licenses="MIT"
COPY --from=build /out/validly /validly
USER nonroot:nonroot
EXPOSE 8080 9090
HEALTHCHECK --interval=15s --timeout=5s --start-period=5s --retries=3 CMD ["/validly", "healthcheck"]
ENTRYPOINT ["/validly"]
CMD ["serve"]
