FROM balenalib/intel-nuc-alpine-golang:1.12 as builder

# RUN [ "cross-build-start" ]
WORKDIR /go/src/github.com/tekn0ir/toe
COPY . .
RUN GO111MODULE=on CGO_ENABLED=0 go build -o toe -a -ldflags '-extldflags "-static"' .
# RUN [ "cross-build-end" ]

FROM balenalib/intel-nuc-alpine:3.8 as toe

WORKDIR /
COPY --from=builder /go/src/github.com/tekn0ir/toe/toe .
CMD ["/toe"]
