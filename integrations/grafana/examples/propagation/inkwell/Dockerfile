# syntax=docker/dockerfile:1
# golang:1.27.1-alpine, published 2026-09-21
FROM golang:1.27.3-alpine@sha256:fa3d596898d9f126af7c01d14ee7597bab3f39cccf392ef36b69fe77cd681f18 AS build
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
