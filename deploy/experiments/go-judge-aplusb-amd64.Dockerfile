# Candidate for the dedicated A+B amd64 trial. Building it does not admit a Worker profile.
FROM criyle/go-judge:v1.13.3@sha256:537632f86eef5eac173b1738fa24dcfba51c53fba5cff64435452d675157619f AS sandbox
FROM gcc:15@sha256:df8a4d9d058f59c2a4ee8cca45e59cea987f442fdb348aa6258357886a13afa8
COPY --from=sandbox /opt/go-judge /opt/go-judge
ENTRYPOINT ["/opt/go-judge", "-http-addr", "127.0.0.1:5050", "-parallelism", "1", "-no-fallback", "-file-timeout", "1m", "-tmp-fs-param", "size=64m,nr_inodes=4k", "-output-limit", "1048576", "-copy-out-limit", "1048576"]
