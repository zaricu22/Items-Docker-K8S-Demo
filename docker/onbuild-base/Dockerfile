# syntax=docker/dockerfile:1
#
# Purpose: a shared base image for many similar Node services.
# Instead of every service repeating "COPY package.json / npm install / COPY .",
# those steps are declared once here as ONBUILD instructions and inherited by each consumer.
#
# ONBUILD steps only fire when another Dockerfile does `FROM docker-kubernetes-app/onbuild-base`;
# building this file alone skips them.
#
#   docker build -t docker-kubernetes-app/onbuild-base:latest docker/onbuild-base/

FROM node:20-alpine
WORKDIR /app

# The three ONBUILD steps are queued, not run here.
# They fire in this order as the first steps of a consumer's build, using the consumer's own files:
ONBUILD COPY package.json ./
ONBUILD RUN npm install --omit=dev
ONBUILD COPY . .

# Default command for every consumer (assumes an app.js entrypoint); a consumer can override it.
CMD ["node", "app.js"]
