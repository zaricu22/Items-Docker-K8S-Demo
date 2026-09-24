# syntax=docker/dockerfile:1
#
# Why this file exists: "items-api" is a real, minimal Spring Boot service
# -- one endpoint, GET /api/hello, returning {"message":"Hello from Backend"} for
# frontend/'s button to call. Without a Dockerfile it's just a jar
# that only runs if someone's machine happens to have the right JDK --
# this packages it (and everything the JVM needs) into one image Kubernetes
# can pull, schedule, restart and health-check identically on every node,
# the same image in dev and in prod.
#
# Unlike most of this repo, this one actually builds and runs -- see the
# repo README's "Running things locally" section for the tested steps
# (docker compose, and separately kind/minikube + kubectl).

# Lets CI build against a different JDK major version with
# --build-arg JAVA_VERSION=... instead of editing this file.
ARG JAVA_VERSION=21

#   docker build --target build -t backend-build backend/   # compile-only, skip the runtime stage
# ---- build: compile + package with Maven, cached separately from runtime ----
# Full Maven + JDK toolchain -- needed to compile, but isolated to this
# stage so none of it ends up in the image that actually gets deployed.
FROM maven:3.9-eclipse-temurin-${JAVA_VERSION} AS build
# Gives every instruction below a fixed, predictable path instead of
# relying on the base image's default working directory.
WORKDIR /app
# Copied alone, before any source -- so the dependency-resolution step
# below only re-runs when pom.xml itself changes, not on every code edit.
COPY pom.xml ./
# Resolves dependencies into a cache that persists across builds, so
# rebuilding doesn't re-download the same jars from Maven Central each time.
RUN --mount=type=cache,target=/root/.m2 \
    mvn -B dependency:go-offline
# Only now, after dependencies are cached, does source get added -- so
# editing application code can't invalidate the (slow) step above it.
COPY src ./src
# Compiles the jar. Tests are skipped here because packaging the image is
# not where the test suite belongs -- that already ran in CI before this
# build started.
RUN --mount=type=cache,target=/root/.m2 \
    mvn -B package -DskipTests

# ---- runtime ----
# A second, clean stage with only a JRE -- no Maven, no compiler -- because
# from here on the image only has to *run* the jar, not build it.
FROM eclipse-temurin:${JAVA_VERSION}-jre AS runtime

# Lets CI stamp the image with the real release version via
# --build-arg APP_VERSION=... instead of hardcoding it here.
ARG APP_VERSION=dev
# Standard OCI metadata so registries/scanners/`docker inspect` can
# identify this image without guessing from its name or tag alone.
LABEL org.opencontainers.image.title="items-api" \
      org.opencontainers.image.version="${APP_VERSION}" \
      org.opencontainers.image.description="Minimal Spring Boot hello-world service for frontend/ to call."

# Runtime config baked in as sane defaults, so the container works with
# zero extra flags, while still overridable via `docker run -e` / k8s env.
ENV APP_ENV=demo \
    SERVER_PORT=8080 \
    JAVA_OPTS=""

# Same reasoning as the build stage's WORKDIR -- also what USER app's home
# directory below resolves against.
WORKDIR /app

# eclipse-temurin (Ubuntu-based) rather than -alpine/-alpine-jre specifically
# so bash exists -- needed for the heredoc RUN and the /dev/tcp HEALTHCHECK
# further down to actually work. SHELL only affects shell-form
# RUN/CMD/ENTRYPOINT instructions from this point on in this stage.
SHELL ["/bin/bash", "-c"]

# Bakes a small build-info file into the image (build time, JDK version) so
# `docker exec`/logs can answer "what exactly is running" without
# re-deriving it later. Heredoc form avoids chaining commands with `&&`/`\`.
RUN <<EOF
mkdir -p /app/meta
echo "built: $(date -u +%FT%TZ)" > /app/meta/build-info.txt
echo "java: $(java -version 2>&1 | head -n1)" >> /app/meta/build-info.txt
if [[ "$APP_ENV" == "demo" ]]; then
  echo "mode: demo" >> /app/meta/build-info.txt
fi
EOF

# Unlike node:slim, this base image ships no unprivileged application user
# out of the box -- and a network-facing service must not run as root.
RUN groupadd --system app && useradd --system --gid app --home /app app

# Pulls only the compiled jar out of the build stage -- not Maven, not the
# JDK, not source -- and hands it straight to the user that will run it.
# --chown sets ownership as part of this layer, instead of a separate
# `RUN chown -R` layer afterwards.
COPY --chown=app:app --from=build /app/target/*.jar ./app.jar

# Everything from here on, including the running container, executes as
# this unprivileged user instead of root.
USER app

# Marks this path as externally-managed state, so data written here isn't
# silently lost (or bloating the image) the way writes to other paths would
# be -- this hello-world app doesn't actually write anything, kept here to
# demonstrate VOLUME itself (and matches backend-deployment.yaml's own
# `data` emptyDir mount at this same path).
VOLUME ["/app/data"]

# Documents which port the app listens on -- matches the Service
# targetPort and the probes in k8s/base/backend-deployment.yaml.
EXPOSE 8080

# Makes the shutdown signal explicit rather than leaning on Docker's
# default -- Spring Boot already shuts down gracefully on SIGTERM.
STOPSIGNAL SIGTERM

# Lets Docker (and anything inspecting the container) tell the difference
# between "process started" and "app can actually serve traffic". No
# curl/wget in this base image, so the check uses bash's own /dev/tcp
# instead of shelling out to an HTTP client.
HEALTHCHECK --interval=30s --timeout=3s --start-period=20s --retries=3 \
    CMD bash -c 'exec 3<>/dev/tcp/127.0.0.1/${SERVER_PORT} && printf "GET /healthz HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n" >&3 && grep -q "200" <&3' || exit 1

# Exec form fixes java as the process that always runs, as PID 1 directly
# -- so it receives SIGTERM itself and can shut down gracefully, instead of
# a shell-form ENTRYPOINT leaving a shell as PID 1 that would swallow it.
ENTRYPOINT ["java"]
# Default arguments to that entrypoint -- overridable at `docker run`
# without having to touch ENTRYPOINT itself.
CMD ["-jar", "app.jar"]
