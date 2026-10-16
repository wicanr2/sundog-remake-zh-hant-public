# Research-only extension of the existing release toolchain. Adds RAR and
# Traditional Chinese OCR; the release build continues to use the base image.
FROM sundog-remake-build:go1.25.12-bookworm-20261014-r1

# Revision r2 replaces r1: libarchive/unrar-free cannot decode this legacy
# archive's Unicode filenames. Use Debian's upstream RAR decoder instead.
# The base fixes Debian repositories to snapshot 20261001T000000Z.
RUN sed -i 's/ main$/ main non-free/' /etc/apt/sources.list \
    && apt-get -o Acquire::Check-Valid-Until=false update \
    && apt-get install --yes --no-install-recommends unrar tesseract-ocr tesseract-ocr-chi-tra \
    && dpkg-query -W > /usr/local/share/sundog-manual-toolchain-packages.txt \
    && rm -rf /var/lib/apt/lists/*
