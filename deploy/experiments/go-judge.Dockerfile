# M0 synthetic tests only. Dedicated offline containers on Docker Desktop.
FROM criyle/go-judge:v1.13.3@sha256:97bec66d054e4d844e0ad29dfdd5c79ef3d09efba25b06dd58643a6e99918dcd AS sandbox
FROM gcc:15@sha256:eb5e64286ad47187b32bda75c6b6916166ecb41da56559b8b09e0001748d2cce
COPY --from=sandbox /opt/go-judge /opt/go-judge
ENTRYPOINT ["/opt/go-judge", "-http-addr", "127.0.0.1:5050", "-parallelism", "1", "-no-fallback", "-file-timeout", "1m", "-tmp-fs-param", "size=64m,nr_inodes=4k", "-output-limit", "1048576", "-copy-out-limit", "1048576"]
