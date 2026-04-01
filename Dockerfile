ARG GO_VERSION=1.23.4
ARG NODE_VERSION=18
ARG ALPINE_VERSION=3.20

FROM node:${NODE_VERSION}-alpine AS node-builder
WORKDIR /app
COPY admin/package.json admin/yarn.lock ./
RUN yarn install --frozen-lockfile
COPY admin/ .
ENV NEXT_TELEMETRY_DISABLED=1
RUN yarn run export

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