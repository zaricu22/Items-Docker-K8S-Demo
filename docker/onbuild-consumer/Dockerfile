# syntax=docker/dockerfile:1
#
# Purpose: one of the "many services" from onbuild-base/Dockerfile.
# Because the base already queued the install/copy steps as ONBUILD, this single FROM line is the whole Dockerfile.
#
# No real app here (no package.json or app.js), so the inherited COPY steps fail -- the point is how short the file gets, not a working service.
#
#   docker build -t docker-kubernetes-app/onbuild-base:latest docker/onbuild-base/
#   docker build -t docker-kubernetes-app/onbuild-consumer:latest docker/onbuild-consumer/

# Building FROM the base triggers its ONBUILD steps (COPY package.json, npm install, COPY .) as if written here, before anything else in this file.
FROM docker-kubernetes-app/onbuild-base:latest
