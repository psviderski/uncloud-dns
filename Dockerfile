ARG ALPINE_VERSION=3.21.3

FROM golang:1.24.0-alpine AS build
COPY / /src
WORKDIR /src
ARG TAG="v0.0.0-dev"
ENV CGO_ENABLED=0
RUN --mount=type=cache,target=/go/pkg --mount=type=cache,target=/root/.cache/go-build \
    go build -o bin/uncloud-dns -ldflags "-s -w -X 'github.com/psviderski/uncloud-dns/pkg/version.Tag=${TAG}'" .

FROM alpine:3.16.2 AS base
RUN apk add --no-cache ca-certificates 
RUN adduser -DH app
RUN mkdir /data && chown app:app /data
USER app
ENTRYPOINT ["/usr/local/bin/uncloud-dns"]
CMD ["server"]

FROM base
COPY --from=build /src/bin/uncloud-dns /usr/local/bin/
