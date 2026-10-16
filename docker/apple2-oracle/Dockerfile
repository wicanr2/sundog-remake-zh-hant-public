FROM debian:bookworm@sha256:813017f3d62be4b5891a7acca6a01bdcd4b8513daa81b1ab99d3a50385b26931

ARG MAME_VERSION=0.251+dfsg.1-1

RUN apt-get update \
    && apt-get install --yes --no-install-recommends \
       "mame=${MAME_VERSION}" \
       ca-certificates \
    && rm -rf /var/lib/apt/lists/*

ENTRYPOINT ["/usr/games/mame"]
