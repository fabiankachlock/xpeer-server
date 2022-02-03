FROM golang:1.17-alpine AS build

WORKDIR /app

COPY go.mod ./
COPY go.sum ./

RUN go mod download

COPY ./ ./

RUN go build -o /xpeer-server ./pkg/xpeer/main.go

FROM alpine

WORKDIR /

COPY --from=build /xpeer-server /xpeer-server

ENTRYPOINT ["/xpeer-server"]

