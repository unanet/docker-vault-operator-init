

FROM golang:1.16-alpine AS build_base

RUN apk add --no-cache git

# Set the Current Working Directory inside the container
WORKDIR /src

# We want to populate the module cache based on the go.{mod,sum} files.
COPY go.mod .
COPY go.sum .

RUN go mod download

COPY . .

# Build the Go app
RUN go build -o ./bin/vault-operator-init .
RUN chmod +x ./bin/vault-operator-init


FROM alpine:3

ENV VAULT_ADDR http://vault.vault:8200

COPY --from=build_base /src/bin/vault-operator-init /app/vault-operator-init
RUN apk add --no-cache ca-certificates

WORKDIR /app

CMD ["/app/vault-operator-init"]
