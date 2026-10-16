FROM debian:bookworm@sha256:813017f3d62be4b5891a7acca6a01bdcd4b8513daa81b1ab99d3a50385b26931

ARG HATARI_VERSION=2.4.1+dfsg-2
ARG XVFB_VERSION=2:21.1.7-3+deb12u12
ARG PYTHON_VERSION=3.11.2-1+b1
ARG XDOTOOL_VERSION=1:3.20160805.1-5

RUN apt-get update \
    && apt-get install --yes --no-install-recommends \
       "hatari=${HATARI_VERSION}" \
       "xvfb=${XVFB_VERSION}" \
       "python3=${PYTHON_VERSION}" \
       "xdotool=${XDOTOOL_VERSION}" \
       ca-certificates \
    && rm -rf /var/lib/apt/lists/*

ENTRYPOINT ["/usr/bin/hatari"]
