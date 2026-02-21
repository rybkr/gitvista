# Build stage
FROM golang:1.26-alpine AS build

RUN apk add --no-cache git

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /gitvista ./cmd/vista

# Runtime stage
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata git openssh-client

COPY --from=build /gitvista /usr/local/bin/gitvista

# Default repo path — mount or clone a repo here
RUN mkdir -p /repo

# Data directory for managed/cloned repos
RUN mkdir -p /data/repos
WORKDIR /repo

EXPOSE 8080

ENTRYPOINT ["gitvista"]
CMD ["-host", "0.0.0.0"]
