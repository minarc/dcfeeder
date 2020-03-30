FROM golang:latest AS builder

ENV GO111MODULE=on \
    CGO_ENABLED=1

WORKDIR /build
COPY /src /public ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -a -ldflags '-s' -installsuffix cgo -o dcfeeder .

WORKDIR /dist
RUN cp -r /build/dcfeeder /build/public .

FROM alpine:latest

COPY --from=builder /dist/dcfeeder /dist/public / 

ENTRYPOINT [ "/dcfeeder" ]


