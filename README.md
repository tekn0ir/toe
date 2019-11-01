# Teknoir Ork3stration Engine
A small footprint IIoT device ork3strator.

# TLDR;
```bash
export DEVICE_ID=my_new_device
export DEVICE_HOST=pi@raspberrypi.local
# Copy device keys
ssh $DEVICE_HOST 'mkdir -p ~/toe_conf'
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

## Build and publish docker images
```bash
docker build -t tekn0ir/toe:amd64 -f amd64.Dockerfile .
docker push tekn0ir/toe:amd64
docker build -t tekn0ir/toe:arm32v7 -f arm32v7.Dockerfile .
docker push tekn0ir/toe:arm32v7
docker build -t tekn0ir/toe:arm64v8 -f arm64v8.Dockerfile .
docker push tekn0ir/toe:arm64v8
docker run -ti --rm -v $(pwd):/app -v ${HOME}/.docker:/root/.docker -w /app mplatform/manifest-tool:latest --username tekn0ir --password <<password>> push from-spec multi-arch-manifest.yaml
```

## Build locally
```bash
GO111MODULE=on go test
GO111MODULE=on CGO_ENABLED=0 go build -o toe -a -ldflags '-extldflags "-static"' .
```

## Run on localhost
```bash
docker run -it --rm -p 1883:1883 -p 9001:9001 eclipse-mosquitto
./toe -project=teknoir-poc -region=us-central1 -registry=teknoir-iot-registry-poc -device=localhost -ca_certs=./devices/localhost/roots.pem -private_key=./devices/localhost/rsa_private.pem -mqtt_broker_host=localhost -kube_config=${HOME}/.kube/config -heartbeat_interval=15
```

## Update TOE software on device
```bash
gcloud iot devices commands send \
    --command-file=update_command.json \
    --region=$REGION  \
    --registry=$REGISTRY_ID \
    --device=$DEVICE_ID
```
## Update device configuration with manifest
```bash
gcloud iot devices configs update \
  --config-file=devices/$DEVICE_ID/manifest.json \
  --device=$DEVICE_ID \
  --registry=$REGISTRY_ID \
  --region=$REGION
```

gcloud iot devices states list \
    --registry=$REGISTRY_ID \
    --device=$DEVICE_ID \
    --region=$REGION

gcloud iot devices configs update \
  --config-file=devices/$DEVICE_ID/manifest.json \
  --device=$DEVICE_ID \
  --registry=$REGISTRY_ID \
  --region=$REGION

gcloud iot devices commands send \
    --command-file=devices/$DEVICE_ID/command.json \
    --region=$REGION  \
    --registry=$REGISTRY_ID \
    --device=$DEVICE_ID \
    --subfolder=iot-sense-pod

for p in $(sudo kubectl -n kube-system get pods | grep Terminating | awk '{print $1}'); do sudo kubectl -n kube-system delete pod $p --grace-period=0 --force;done


# Troubleshooting
* Some OS does not enable network interface (wifi) until logged in

## Jetson Nano
Shipped ubuntu does not have ipset kernel module installed
```bash
sudo cp /etc/apt/sources.list /etc/apt/sources.list~
sudo sed -Ei 's/^# deb-src /deb-src /' /etc/apt/sources.list
sudo apt-get update
sudo apt-get build-dep ipset
```
Edit /etc/depmod.d/ubuntu.conf and add ‘extra’ to the end of the line, so it looked like:
```
search updates ubuntu built-in extra
```
Build ipset kernel module and install
```bash
sudo apt-get source ipset
cd ipset-6.*
./autogen.sh
./configure
make modules
sudo make modules_install
```
Probably gives you an error but run:
```bash
modprobe xt_set
lsmod
```
And xt_set (yes xt_set!) should be in the list.
It’s possible that the version you compiled the kernel module for isn’t the same as the one you downloaded earlier from the repositories. If you’re having problems, remove the one that was installed earlier (apt-get purge ipset) and do ‘make && make install’ from the downloaded ipset folder.
