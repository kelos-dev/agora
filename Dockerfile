# syntax=docker/dockerfile:1

FROM golang:1.26-alpine AS build
WORKDIR /src

COPY go.* ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal

RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/agora-server ./cmd/agora-server

FROM alpine:3

RUN addgroup -S agora \
	&& adduser -S -G agora agora \
	&& mkdir -p /data \
	&& chown agora:agora /data

ENV AGORA_ADDR=0.0.0.0:8080
ENV AGORA_DATA=/data/agora.jsonl

EXPOSE 8080
VOLUME ["/data"]

COPY --from=build /out/agora-server /usr/local/bin/agora-server

USER agora
ENTRYPOINT ["/usr/local/bin/agora-server"]
