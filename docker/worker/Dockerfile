# syntax=docker/dockerfile:1
#
# Purpose: a real, minimal background agent -- logs this container's load average and available memory on an interval,
# reading /proc directly (stdlib only, no external module or network calls).
# Baked into one static binary image, the same one any container using it would run.
#
# Builds and runs like backend/frontend -- see docker/docker-compose.yml's `worker` service.

#   docker buildx build --platform linux/amd64,linux/arm64 -t worker docker/worker/
# Build on the host's own platform; cross-compile for the target below (Go doesn't need to emulate the target arch).
# $BUILDPLATFORM is a BuildKit-injected, automatic build argument (also TARGETPLATFORM, TARGETOS, TARGETARCH, TARGETVARIANT, BUILDOS, BUILDARCH, BUILDVARIANT).
FROM --platform=$BUILDPLATFORM golang:1.22-alpine AS build
WORKDIR /src

# Filled in by buildx for each requested --platform (e.g. linux/arm64).
# Same as above, automatic build arguments.
ARG TARGETOS
ARG TARGETARCH

# One COPY of multiple files, no cache optimization possible here
COPY go.mod main.go ./

# Mount type=secret: visible only during this RUN at /run/secrets/build_token, never stored in a layer or the cache (unlike ARG/ENV).
# Only demonstrates the syntax; this build doesn't actually use a secret.
#   docker build --secret id=build_token,src=./build_token.txt -t worker .
RUN --mount=type=secret,id=build_token \
    if [ -f /run/secrets/build_token ]; then \
      echo "using build_token of length $(wc -c < /run/secrets/build_token)"; \
    else \
      echo "no build_token secret provided -- fine, unused by this build anyway"; \
    fi

# Mount type=cache: lives only in BuildKit's own persistent cache store, so it survives even when the layer cache below misses.
# Two caches are used here:
#   Docker/BuildKit's layer cache (decides whether this whole RUN needs to execute again at all; instruction/inputs changes invalidate the cache)
#   Go's build cache (so 'go build' doesn't have to recompile every package; Docker doesn't see this cache but it should never delete it)
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -o /worker .

# scratch = empty base (no shell, no libc): \
# a static binary that only reads the kernel-provided /proc needs nothing else, so anything more is extra attack surface.
FROM scratch AS runtime
# Only the binary -- no Go toolchain or source.
# Above 'go build -o /worker .' explicitly wrote it to /worker (becomes new workdir), at the filesystem root, deliberately outside /src,
# it also set executable permission bits (chmod +x) and first few content bytes (how to run) so ENTRYPOINT/CMD can run it directly.
COPY --from=build /worker /worker

# Choose desired SIG.. on 'docker stop' command; because worker process have own signal handler,
# it expects: SIGTERM(docker stop)/SIGINT(terminal ctrl+c).
STOPSIGNAL SIGTERM

# Worker is a long-running process, written to run forever even when bad reads occurs.
# Real, minimal Go binary (stdlib only): logs container's load average and available memory on an WORKER_INTERVAL, reading /proc dir directly;
ENTRYPOINT ["/worker"]

# We must periodically use a HEALTHCHECK to call process own 'healthcheck' code
# to verify does it still print meaningful output or only errors to kill them.
#   docker inspect --format '{{.State.Health.Status}}' <container>   # check the result yourself
HEALTHCHECK --interval=30s --timeout=3s --retries=3 \
    CMD ["/worker", "-healthcheck"]
