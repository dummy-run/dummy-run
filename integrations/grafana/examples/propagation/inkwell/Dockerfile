# syntax=docker/dockerfile:1
# golang:1.27.1-alpine, published 2026-09-21
FROM golang:1.27.2-alpine@sha256:f92b6ef800e499660581efdabdf25d9d817a9d124eaf900924f0504e7e27e12d AS build
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
