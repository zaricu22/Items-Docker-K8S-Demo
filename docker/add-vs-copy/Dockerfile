# syntax=docker/dockerfile:1
#
# Purpose: show the one case where ADD beats COPY (auto-extracting a local archive),
# next to plain COPY and ADD's discouraged remote-URL form.
#
# release.tar.gz and app.sh don't exist in this repo, so the build fails past those steps;
# read the comments, don't expect a working image.

#   docker build -t add-vs-copy docker/add-vs-copy/
FROM alpine:3.20
WORKDIR /app

# COPY: copies exactly what is in the build context -- no extraction, no network.
# The right default for almost everything.
COPY app.sh ./

# ADD, local archive: a local .tar/.tar.gz/.tgz is extracted into the destination.
# COPY would leave the .tar.gz itself in the image, still compressed.
# This is the one good reason to use ADD.
ADD release.tar.gz ./release/

# ADD, remote URL: downloads the file (COPY can't reach the network).
# Generally discouraged:
#   - the download is NOT extracted, even if it's an archive
#   - the same URL always hits the layer cache, so a changed remote file silently keeps the stale layer
#   - no inline checksum check
# Prefer `RUN curl -fsSL ... -o file && ...` so caching, extraction and verification are explicit.
ADD https://example.com/config/defaults.json /app/config/defaults.json

# COPY/ADD keep the source file's permission bits, and a local app.sh often isn't executable.
RUN chmod +x /app/app.sh

# No ENTRYPOINT is set, so this CMD is the full command.
CMD ["/app/app.sh"]
