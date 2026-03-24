FROM node:22-bookworm-slim AS node-runtime

FROM golang:1.25-bookworm AS builder

ARG GOPROXY=https://goproxy.cn,direct
ARG GOSUMDB=sum.golang.google.cn
ENV GOPROXY=${GOPROXY}
ENV GOSUMDB=${GOSUMDB}

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal
COPY webui ./webui
COPY --from=node-runtime /usr/local/ /usr/local/
RUN npm ci --prefix /src/webui/og-card-demo
RUN CGO_ENABLED=0 go build -o /out/onboarding ./cmd/onboarding
RUN go build -o /out/brale-core ./cmd/brale-core

FROM debian:bookworm-slim AS brale-runtime

ENV TZ=Asia/Shanghai
ENV BRALE_NOTIFY_OG_SCRIPT=/app/webui/og-card-demo/render.mjs

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl tzdata \
    && ln -snf /usr/share/zoneinfo/${TZ} /etc/localtime \
    && echo ${TZ} > /etc/timezone \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=node-runtime /usr/local/ /usr/local/
COPY --from=builder /out/brale-core /usr/local/bin/brale-core
COPY --from=builder /src/webui/og-card-demo /app/webui/og-card-demo

ENTRYPOINT ["brale-core"]
CMD ["-system", "configs/system.toml", "-symbols", "configs/symbols-index.toml"]

FROM docker:28-cli AS onboarding-runtime

ENV TZ=Asia/Shanghai

RUN apk add --no-cache bash curl git make tzdata

WORKDIR /workspace
COPY --from=builder /out/onboarding /usr/local/bin/onboarding

ENTRYPOINT ["onboarding"]
CMD ["serve", "-addr", "0.0.0.0:9992", "-repo", "/workspace"]
