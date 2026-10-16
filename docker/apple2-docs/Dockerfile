FROM debian:bookworm

ARG DEBIAN_FRONTEND=noninteractive

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates=20230311+deb12u1 \
        poppler-utils=22.12.0-2+deb12u3 \
        tesseract-ocr=5.3.0-2 \
        tesseract-ocr-eng=1:4.1.0-2 \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /work

CMD ["sh", "-lc", "pdftoppm -v 2>&1 | head -n 1; tesseract --version | head -n 1"]
