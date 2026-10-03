# Code generators with pinned versions, so `pnpm gen` gives the same output on macOS, Windows
# and Linux: buf with the Go and TypeScript plugins, and sqlc.
FROM golang:1.26-alpine
RUN apk add --no-cache nodejs npm
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.12 \
 && go install connectrpc.com/connect/v2/cmd/protoc-gen-connect-go@v2.0.0-rc.1 \
 && npm install -g @bufbuild/protoc-gen-es@2.16.0
COPY --from=bufbuild/buf:1.73.0 /usr/local/bin/buf /usr/local/bin/buf
COPY --from=sqlc/sqlc:1.30.0 /workspace/sqlc /usr/local/bin/sqlc
WORKDIR /src
CMD ["sh", "-c", "buf lint && buf generate && cd backend && sqlc generate"]
