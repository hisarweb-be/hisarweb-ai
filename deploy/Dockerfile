# Production image for the HisarWeb AI gateway (fork of new-api).
#
# The repository root keeps the upstream Dockerfile unchanged; this one is ours:
# a non-root runtime user, a healthcheck on /api/status and pinned base images.
# Build from the repository root, so the context sees web/ and the Go modules:
#   docker build -f deploy/Dockerfile -t hisarweb-ai:prod .
#
# Base images are pinned by digest, the same digests the upstream image uses.

FROM oven/bun:1.4.0@sha256:5ff609364c049b54eb0ff560ec96319729a972078ef2c755d758f0c6ef89c2d6 AS web

WORKDIR /build/web
COPY web/package.json web/bun.lock ./
RUN bun install --frozen-lockfile
COPY ./web ./
COPY ./VERSION /build/VERSION
RUN DISABLE_ESLINT_PLUGIN='true' VITE_REACT_APP_VERSION=$(cat /build/VERSION) bun run build

FROM golang:1.26.9-alpine@sha256:3082400e369fa24d5fc60bca20edab3f6d604e0c5a690ec66b295eff4dd87ade AS build

ENV GO111MODULE=on CGO_ENABLED=0 GOWORK=off
ARG TARGETOS
ARG TARGETARCH
ENV GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64}
ENV GOEXPERIMENT=greenteagc

WORKDIR /build

ADD go.mod go.sum ./
# relaykit and tokenkit are local submodules referenced through replace, so
# their go.mod files must be present before go mod download resolves the graph.
ADD relaykit/go.mod ./relaykit/go.mod
ADD tokenkit/go.mod ./tokenkit/go.mod
RUN go mod download

COPY . .
COPY --from=web /build/web/dist ./web/dist
RUN go build -pgo="$(go list -m -f '{{.Dir}}' github.com/Calcium-Ion/moejs)/default.pgo" -ldflags "-s -w -X 'github.com/QuantumNous/new-api/common.Version=$(cat VERSION)'" -o new-api

FROM debian:bookworm-slim@sha256:f06537653ac770703bc45b4b113475bd402f451e85223f0f2837acbf89ab020a AS runtime

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates tzdata wget \
    && rm -rf /var/lib/apt/lists/* \
    && update-ca-certificates

# The gateway writes its SQLite database, uploads and logs under /data, so the
# runtime user owns that directory and nothing else is writable for it.
RUN groupadd --system --gid 10001 new-api \
    && useradd --system --uid 10001 --gid new-api --home-dir /data --shell /usr/sbin/nologin new-api \
    && mkdir -p /data/logs /licenses \
    && chown -R new-api:new-api /data

COPY --from=build /build/new-api /usr/local/bin/new-api
COPY LICENSE NOTICE THIRD-PARTY-LICENSES.md /licenses/

# Timestamps in the customer-facing logs and mails follow Turkish time.
ENV TZ=Europe/Istanbul \
    PORT=3000

USER 10001:10001
WORKDIR /data
EXPOSE 3000

# wget exits non-zero on a connection error and on any HTTP error status, so a
# failed start or a gateway that stopped answering marks the container unhealthy.
HEALTHCHECK --interval=30s --timeout=5s --start-period=40s --retries=3 \
    CMD wget -q -O /dev/null http://127.0.0.1:3000/api/status || exit 1

ENTRYPOINT ["/usr/local/bin/new-api"]
