FROM ghcr.io/acorn-io/images-mirror/golang:1.20-alpine AS build
COPY / /src
WORKDIR /src
ARG TAG="v0.0.0-dev"
ENV CGO_ENABLED=0
RUN --mount=type=cache,target=/go/pkg --mount=type=cache,target=/root/.cache/go-build \
    go build -o bin/acorn-dns -ldflags "-s -w -X 'github.com/psviderski/uncloud-dns/pkg/version.Tag=${TAG}'" .

FROM ghcr.io/acorn-io/images-mirror/alpine:3.16.2 AS base
RUN apk add --no-cache ca-certificates 
RUN adduser -D acorn
# USER acorn commented out so that i can easily get into the container and do stuff to it
ENTRYPOINT ["/usr/local/bin/acorn-dns"]
CMD ["api-server"]

FROM base
COPY --from=build /src/bin/acorn-dns /usr/local/bin/acorn-dns
