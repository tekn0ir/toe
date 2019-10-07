


https://medium.com/google-cloud/cloud-iot-step-by-step-connecting-raspberry-pi-python-2f27a2893ab5

https://github.com/GoogleCloudPlatform/golang-samples/tree/master/iot
https://github.com/GoogleCloudPlatform/golang-samples/blob/master/iotkit/helloworld/main.go
https://github.com/nathany/bobblehat


go test
CGO_ENABLED=0 go build -o toe -a -ldflags '-extldflags "-static"' .
./toe -project=teknoir-poc -region=us-central1 -registry=teknoir-iot-registry-poc -device=go_client_test -ca_certs=roots.pem -private_key=./demo_private.pem