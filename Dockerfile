FROM golang:latest AS builder

ENV GO111MODULE=on \
    CGO_ENABLED=1

WORKDIR /build
COPY go.mod go.sum config.yaml ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main .

WORKDIR /dist
RUN cp /build/main .

FROM alpine:latest

COPY --from=builder /dist/main / 

ENTRYPOINT [ "/main" ]


