# Teknoir Ork3stration Engine
A small footprint IIoT device ork3strator.

# TLDR;
```bash
export DEVICE_ID=my_new_device
export DEVICE_HOST=pi@raspberrypi.local
# Copy device keys
scp -r devices/$DEVICE_ID/* $DEVICE_HOST:~/toe_conf
# Copy toe manifest
scp toe-deployment.yaml $DEVICE_HOST:~/
# ssh to your device
ssh $DEVICE_HOST
# Install k3s
curl -sfL https://get.k3s.io | sh -
# Deploy toe manifest
sudo kubectl apply -f toe-deployment.yaml
```

## Register device
### Generate keys
```bash 
export DEVICE_ID=my_new_device
mkdir devices/$DEVICE_ID
openssl req -x509 -newkey rsa:2048 -keyout devices/$DEVICE_ID/rsa_private.pem -nodes -out devices/$DEVICE_ID/rsa_public.pem -subj "/CN=unused"
curl https://pki.goog/roots.pem > devices/$DEVICE_ID/roots.pem
```

### Register device
```bash
export DEVICE_ID=my_new_device
export PROJECT_ID=teknoir-poc
export REGION=us-central1
export REGISTRY_ID=teknoir-iot-registry-poc
gcloud iot devices create $DEVICE_ID \
  --project=$PROJECT_ID \
  --region=$REGION \
  --registry=$REGISTRY_ID \
  --public-key path=devices/$DEVICE_ID/rsa_public.pem,type=rsa-x509-pem
```

## Build and publish docker image
```bash
docker build -t tekn0ir/toe:latest .
docker push tekn0ir/toe:latest
```

## Build locally
```bash
GO111MODULE=on go test
GO111MODULE=on CGO_ENABLED=0 go build -o toe -a -ldflags '-extldflags "-static"' .
```

## Run on localhost
```bash
docker run -it -p 1883:1883 -p 9001:9001 eclipse-mosquitto
./toe -project=teknoir-poc -region=us-central1 -registry=teknoir-iot-registry-poc -device=localhost -ca_certs=./devices/localhost/roots.pem -private_key=./devices/localhost/rsa_private.pem -mqtt_broker_host=localhost -kube_config=${HOME}/.kube/config
```