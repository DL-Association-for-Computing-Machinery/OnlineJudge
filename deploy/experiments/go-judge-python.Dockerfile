# Local P7 Ubuntu arm64 executor. Keep the admitted GCC and Java tools.
FROM python:3.14.10-slim-trixie@sha256:973d41424c3fd81029ac9230bcdec2580fb67b49af9004279001c301182ae868 AS cpython
FROM oj-p7-java21:local
COPY --from=cpython /usr/local/bin/python3.14 /usr/local/bin/python3.14
COPY --from=cpython /usr/local/lib/libpython3.14.so.1.0 /usr/local/lib/libpython3.14.so.1.0
COPY --from=cpython /usr/local/lib/python3.14 /usr/local/lib/python3.14
