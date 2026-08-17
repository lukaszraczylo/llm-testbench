# Runtime image for the llm-testbench test runner. goreleaser copies the
# prebuilt, statically linked linux binary into the build context, so this
# file only assembles the runtime environment - it does not compile Go.
#
# The image carries the go, python3, and C toolchains so the exec
# evaluators (GoRun/PyRun/CRun) run inside the container instead of being
# skipped. That is the point of the image; a slim variant would silently
# skip a third of the programming category.
FROM golang:1.26-alpine

RUN apk add --no-cache python3 gcc musl-dev \
    && adduser -D -u 10001 llmtest \
    && mkdir -p /work \
    && chown llmtest:llmtest /work

COPY llmtest /usr/local/bin/llmtest

USER llmtest
WORKDIR /work

# Toolchain caches for the exec evaluators live in the user's home.
ENV GOCACHE=/home/llmtest/.cache/go-build \
    GOMODCACHE=/home/llmtest/go/pkg/mod

# Mount a config at /work/config.yaml (or pass --config) and run:
#   docker run --rm -v $PWD/config.yaml:/work/config.yaml \
#     ghcr.io/lukaszraczylo/llm-testbench run --format table
ENTRYPOINT ["/usr/local/bin/llmtest"]
CMD ["--help"]
