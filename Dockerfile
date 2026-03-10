# Build stage
FROM golang:1.26-alpine AS build

RUN apk add --no-cache git

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.buildDate=${BUILD_DATE}" \
    -o /gitvista-site ./cmd/site

# Runtime stage
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata git openssh-client

COPY --from=build /gitvista-site /usr/local/bin/gitvista-site

# Data directory for managed/cloned repos
RUN mkdir -p /data/repos
WORKDIR /data/repos

EXPOSE 8080

ENTRYPOINT ["gitvista-site"]
CMD ["-host", "0.0.0.0"]
