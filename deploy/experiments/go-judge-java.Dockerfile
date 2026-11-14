# P7 local Ubuntu arm64 experiment only. Extend the admitted GCC executor.
FROM eclipse-temurin:21-jdk-noble@sha256:472a05556e24fb10f2a87906cb56c8d76fccedb1b494d0e9201814fc9a95130a AS jdk
FROM oj-m0-sandbox:local
COPY --from=jdk /opt/java/openjdk /usr/local/oj-jdk
COPY --chmod=0555 deploy/experiments/compile-java21.sh /usr/local/bin/oj-compile-java21
