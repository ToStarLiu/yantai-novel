FROM node:24.22.3-alpine AS web
WORKDIR /src/web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM node:24.22.3-alpine AS harness
WORKDIR /src/harness
COPY harness/package*.json ./
RUN npm ci
COPY harness/ ./
RUN npm run build && npm run bundle

FROM golang:1.27.4-alpine AS go
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/api ./cmd/api && CGO_ENABLED=0 go build -trimpath -o /out/worker ./cmd/worker

FROM node:24.22.3-alpine
RUN apk add --no-cache ca-certificates tzdata && adduser -D -u 10001 novel && mkdir -p /app/data && chown 10001:10001 /app/data
WORKDIR /app
COPY --from=go /out/api /app/api
COPY --from=go /out/worker /app/worker
COPY --from=web /src/web/dist /app/web/dist
COPY LICENSE NOTICE.md /app/
COPY assets/fonts/ /app/assets/fonts/
COPY assets/licenses/ /app/assets/licenses/
COPY --from=harness /src/harness/bundle/ /app/harness/bundle/
USER novel
EXPOSE 8088
ENV APP_ADDR=0.0.0.0:8088 RUN_WORKER=false HARNESS_NODE_PATH=node HARNESS_ENTRY_PATH=/app/harness/bundle/main.mjs
ENTRYPOINT ["/app/api"]
