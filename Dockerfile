FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=docker
ARG COMMIT=unknown
RUN CGO_ENABLED=0 go build \
    -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" \
    -o /out/zosmf-mcp ./cmd/zosmf-mcp

FROM alpine:3.21
RUN apk add --no-cache ca-certificates openssh-client
COPY --from=build /out/zosmf-mcp /usr/local/bin/zosmf-mcp
EXPOSE 8081
ENTRYPOINT ["/usr/local/bin/zosmf-mcp", "--config", "/config/.smpe-zosmf.yaml"]
