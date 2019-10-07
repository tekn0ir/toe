FROM golang:1.13 as builder
WORKDIR /go/src/bitbucket.com/teknoir/toe
COPY . .
RUN CGO_ENABLED=0 go build -o toe -a -ldflags '-extldflags "-static"' .


FROM alpine:3.8
WORKDIR /
COPY --from=builder /go/src/bitbucket.com/teknoir/toe/toe .

CMD ["/toe", "-project=${PROJECT_ID}", "-registry=${REGISTRY_ID}", "-device=${DEVICE_ID}", "-algorithm=${ALGORITHM}", "-ca_certs=${CA_CERTS}", "-private_key=${PRIVATE_KEY_FILE_PATH}"]