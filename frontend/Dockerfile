# syntax=docker/dockerfile:1
#
# Why this file exists: this is items-frontend, a real, minimal Angular
# SPA -- one page, one button, that calls backend/'s GET /api/hello
# and shows the response. Built output is just static HTML/JS/CSS, no Node
# needed to serve it. Without a Dockerfile that's a folder of files someone
# has to remember how to serve correctly (right MIME types, right
# client-side-routing fallback, right headers) by hand on every
# environment. This packages the Angular CLI build *and* a correctly
# configured web server into one image, so "it works on my machine" isn't
# a question anyone has to ask again.
#
# Unlike most of this repo, this one actually builds and runs -- see the
# repo README's "Running things locally" section for the tested steps.

# Lets CI build against a different Node version with
# --build-arg NODE_VERSION=... instead of editing this file.
ARG NODE_VERSION=20

# --platform=$BUILDPLATFORM pins this stage to the machine actually running
# the build, even when producing a multi-arch image via
# `docker buildx build --platform linux/amd64,linux/arm64`. The build output
# here is plain JS/HTML (platform-agnostic), so emulating the build itself
# for each target platform would only be slower, not more correct.
FROM --platform=$BUILDPLATFORM node:${NODE_VERSION}-alpine AS build
# Fixed, predictable path for every instruction below instead of relying
# on the base image's default working directory.
WORKDIR /app
# Copied alone, before the rest of the source, so `npm install` below is
# only re-run when the dependency list itself changes.
COPY package.json ./
# Caches downloaded packages across builds, so re-running `npm install`
# doesn't re-fetch the same packages from the registry every time.
RUN --mount=type=cache,target=/root/.npm \
    npm install
# Only now does the rest of the source get added -- so editing application
# code can't invalidate the (slow) install step above it.
COPY . .
# `npm run build` here just wraps the Angular CLI's own `ng build`, which
# defaults to a production build (AOT compilation, bundling, budgets) since
# Angular CLI 12 -- no separate --configuration production flag needed.
# Produces the static bundle that's actually served -- the only output of
# this whole stage the runtime stage needs.
RUN npm run build

# A second, clean stage with only nginx -- no Node, no npm cache, no
# source -- because from here on the image only has to *serve* static
# files, not build them.
FROM nginx:1.27-alpine AS runtime

# Standard OCI metadata so registries/scanners/`docker inspect` can
# identify this image without guessing from its name or tag alone.
LABEL org.opencontainers.image.title="items-frontend" \
      org.opencontainers.image.description="Minimal Angular hello-world SPA that calls items-api."

# Replaces nginx's default site config with one that actually knows how to
# serve this Angular SPA (falls back to index.html for client-side routes
# the Angular Router owns -- otherwise a direct hit or refresh on any
# route but "/" 404s at nginx before Angular ever gets to handle it).
COPY nginx.conf /etc/nginx/conf.d/default.conf
# Pulls only the compiled static output out of the build stage -- not
# Node, not npm, not source -- into the path nginx.conf above expects.
# `dist/items-frontend/browser` (not just `dist/items-frontend`) because
# the modern Angular CLI application builder (default since Angular 17)
# nests output one level deeper than the older webpack builder did, to
# leave room for a sibling `server/` bundle on SSR projects -- this one
# isn't SSR, but the build still lands in `browser/` either way.
COPY --from=build /app/dist/items-frontend/browser /usr/share/nginx/html

# nginx's own image already ships this unprivileged user, so it's reused
# here instead of creating a new one -- everything from here on runs as it.
USER nginx
# Documents which port the app listens on (set by nginx.conf's own
# `listen 4200;`, deliberately different from backend's 8080 so the two
# port numbers are never confusable with each other while reading either
# file) -- matches the Service targetPort and the probes in
# k8s/base/controllers/frontend-deployment.yaml.
EXPOSE 4200

# Lets Docker (and anything inspecting the container) tell the difference
# between "process started" and "nginx is actually serving pages".
HEALTHCHECK --interval=30s --timeout=3s --retries=3 \
    CMD wget -qO- http://localhost:4200/ || exit 1

# Default command for the image's implicit ENTRYPOINT (none is set here,
# so CMD alone becomes the full command) -- `-g "daemon off;"` keeps nginx
# in the foreground as PID 1, since Docker expects the main process to
# stay running, not fork and exit.
CMD ["nginx", "-g", "daemon off;"]
