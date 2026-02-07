# Stage 1: builder
FROM golang:1.24-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /bin/minitowerd ./cmd/minitowerd
RUN CGO_ENABLED=0 go build -o /bin/minitower-runner ./cmd/minitower-runner
RUN CGO_ENABLED=0 go build -o /bin/minitower-cli ./cmd/minitower-cli

# Stage 2: minitowerd
FROM alpine:3.21 AS minitowerd
COPY --from=builder /bin/minitowerd /usr/local/bin/
EXPOSE 8080
ENTRYPOINT ["minitowerd"]

# Stage 3: minitower-runner
FROM python:3.12-slim AS minitower-runner
RUN apt-get update && apt-get install -y --no-install-recommends tar && rm -rf /var/lib/apt/lists/*
COPY --from=builder /bin/minitower-runner /usr/local/bin/
ENTRYPOINT ["minitower-runner"]

# Stage 4: minitower-cli
FROM alpine:3.21 AS minitower-cli
COPY --from=builder /bin/minitower-cli /usr/local/bin/
ENTRYPOINT ["minitower-cli"]
