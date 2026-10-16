# Revision go1.25.12-bookworm-20261014-r1 replaces unavailable
# sundog-remake-build:20260812 and its unavailable dsds-go:1.25 parent.
# Docker-only build: timeout 900s docker build --memory 4g --cpu-period 100000
# --cpu-quota 200000 -f docker/sundog-remake-build/Dockerfile
# -t sundog-remake-build:go1.25.12-bookworm-20261014-r1 .
# Go and Debian repositories are fixed; Go dependencies follow go.sum.
FROM golang:1.25.12-bookworm@sha256:c62765daa3fb92521a46cc8242797b81b03cf592b5aeffc36bae63d9abc1385c

ARG DEBIAN_SNAPSHOT=20261001T000000Z
RUN rm /etc/apt/sources.list.d/debian.sources \
    && printf 'deb [check-valid-until=no] http://snapshot.debian.org/archive/debian/%s bookworm main\ndeb [check-valid-until=no] http://snapshot.debian.org/archive/debian-security/%s bookworm-security main\n' "$DEBIAN_SNAPSHOT" "$DEBIAN_SNAPSHOT" > /etc/apt/sources.list \
    && apt-get -o Acquire::Check-Valid-Until=false update \
    && apt-get install --yes --no-install-recommends \
       build-essential pkg-config libgl1-mesa-dev libx11-dev libxcursor-dev \
       libxrandr-dev libxinerama-dev libxi-dev libxxf86vm-dev libasound2-dev \
       xvfb xdotool x11-utils imagemagick python3 python3-pil ffmpeg file \
       patchelf hatari \
    && dpkg-query -W > /usr/local/share/sundog-toolchain-packages.txt \
    && rm -rf /var/lib/apt/lists/*

ENV GOTOOLCHAIN=local

WORKDIR /workspace

COPY go.mod go.sum /tmp/sundog-remake-modules/
RUN cd /tmp/sundog-remake-modules && go mod download

ENTRYPOINT ["/bin/sh"]
