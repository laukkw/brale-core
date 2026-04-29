FROM node:22-bookworm-slim AS node-runtime

FROM golang:1.25-bookworm AS bralectl-builder

ARG GOPROXY=https://goproxy.cn,direct
ARG GOSUMDB=sum.golang.google.cn
ARG HTTP_PROXY=
ARG HTTPS_PROXY=
ARG NO_PROXY=
ARG http_proxy=
ARG https_proxy=
ARG no_proxy=
ARG NPM_CONFIG_REGISTRY=https://registry.npmmirror.com
ENV GOPROXY=${GOPROXY}
ENV GOSUMDB=${GOSUMDB}
ENV HTTP_PROXY=${HTTP_PROXY}
ENV HTTPS_PROXY=${HTTPS_PROXY}
ENV NO_PROXY=${NO_PROXY}
ENV http_proxy=${http_proxy}
ENV https_proxy=${https_proxy}
ENV no_proxy=${no_proxy}
ENV NPM_CONFIG_REGISTRY=${NPM_CONFIG_REGISTRY}
ENV NPM_CONFIG_REPLACE_REGISTRY_HOST=always
ENV NPM_CONFIG_FETCH_RETRIES=5
ENV NPM_CONFIG_FETCH_RETRY_MINTIMEOUT=20000
ENV NPM_CONFIG_FETCH_RETRY_MAXTIMEOUT=120000

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -buildvcs=false -o /out/bralectl ./cmd/bralectl

FROM bralectl-builder AS builder

COPY webui/og-card-demo/package.json webui/og-card-demo/package-lock.json /src/webui/og-card-demo/
COPY --from=node-runtime /usr/local/ /usr/local/
RUN npm ci --prefix /src/webui/og-card-demo

COPY webui ./webui
RUN CGO_ENABLED=0 go build -buildvcs=false -o /out/brale-core ./cmd/brale-core

FROM debian:bookworm-slim AS brale-runtime

ENV TZ=Asia/Shanghai
ENV BRALE_NOTIFY_OG_SCRIPT=/app/webui/og-card-demo/render.mjs

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl tzdata \
    && ln -snf /usr/share/zoneinfo/${TZ} /etc/localtime \
    && echo ${TZ} > /etc/timezone \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd -g 1000 brale \
    && useradd -u 1000 -g brale -s /usr/sbin/nologin -M brale

WORKDIR /app
COPY --from=node-runtime /usr/local/ /usr/local/
COPY --from=builder /out/bralectl /usr/local/bin/bralectl
COPY --from=builder /out/brale-core /usr/local/bin/brale-core
COPY --from=builder /src/webui/og-card-demo /app/webui/og-card-demo

EXPOSE 9991

USER 1000:1000

ENTRYPOINT ["brale-core"]
CMD ["-system", "configs/system.toml", "-symbols", "configs/symbols-index.toml"]
