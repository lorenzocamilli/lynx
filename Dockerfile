ARG GO_VERSION=1.25
ARG NODE_VERSION=20
ARG ALPINE_VERSION=3.20

FROM node:${NODE_VERSION}-alpine AS node-builder
WORKDIR /app
COPY admin/package.json admin/package-lock.json ./
RUN npm ci --legacy-peer-deps
COPY admin/ .
ENV NEXT_TELEMETRY_DISABLED=1
RUN npm run export

FROM golang:${GO_VERSION}-alpine AS go-builder
ARG HETTY_VERSION=0.0.0
ENV CGO_ENABLED=0
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY pkg ./pkg
COPY --from=node-builder /app/dist ./cmd/hetty/admin
RUN go build -ldflags="-s -w -X main.version=${HETTY_VERSION}" ./cmd/hetty

FROM alpine:${ALPINE_VERSION}
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=go-builder /app/hetty .

ENTRYPOINT ["./hetty"]
EXPOSE 8080
