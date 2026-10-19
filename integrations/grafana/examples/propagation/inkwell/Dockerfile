# syntax=docker/dockerfile:1
# golang:1.27.1-alpine, published 2026-09-21
FROM golang:1.27.1-alpine@sha256:8a5910f31396cd4d89662f56c68b3ae31d374308270a1c3bd96672ee5ed43414 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /inkwell .

FROM scratch
COPY --from=build /inkwell /inkwell
USER 65534:65534
EXPOSE 8080
ENTRYPOINT ["/inkwell"]
