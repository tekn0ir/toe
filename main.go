package main

import (
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/dgrijalva/jwt-go"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/tekn0ir/toe/iot"
)

const (
	timeout         = 30 * time.Second
	protocolVersion = 4 // MQTT 3.1.1
	clientIDFormat  = "projects/%v/locations/%v/registries/%v/devices/%v"
)

const (
	qosAtMostOnce byte = iota
	qosAtLeastOnce
	qosExactlyOnce
)

// getEnv get key environment variable if exist otherwise return defalutValue
func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if len(value) == 0 {
		return defaultValue
	}
	return value
}

var (
	deviceID = flag.String("device", getEnv("TOE_DEVICE", "no-default-device-id"), "Cloud IoT Core Device ID")
	bridge   = struct {
		host *string
		port *string
	}{
		flag.String("mqtt_host", getEnv("TOE_MQTT_HOST", "mqtt.googleapis.com"), "mqtt Bridge Host"),
		flag.String("mqtt_port", getEnv("TOE_MQTT_PORT", "8883"), "mqtt Bridge Port"),
	}
	projectID   = flag.String("project", getEnv("TOE_PROJECT", "no-default-project-id"), "GCP Project ID")
	registryID  = flag.String("registry", getEnv("TOE_IOT_REGISTRY", "no-default-registry-id"), "Cloud IoT Registry ID (short form)")
	region      = flag.String("region", getEnv("TOE_REGION", "us-central1"), "GCP Region")
	certsCA     = flag.String("ca_certs", getEnv("TOE_CA_CERT", "no-default-ca-cert"), "Download https://pki.google.com/roots.pem")
	privateKey  = flag.String("private_key", getEnv("TOE_PRIVATE_KEY", "no-default-private-key"), "Path to private key file")
	kubeConfig  = flag.String("kube_config", getEnv("KUBE_CONFIG", ""), "The path to the kubernetes config, or defaults to in cluster config")
	demuxBroker = struct {
		host *string
		port *string
	}{
		flag.String("mqtt_broker_host", getEnv("HMQ_SERVICE_HOST", "hmq"), "mqtt Broker Host"),
		flag.String("mqtt_broker_port", getEnv("HMQ_SERVICE_PORT", "1883"), "mqtt Broker Port"),
	}
)

var c iot.CloudIotClient
var demuxEventHandler mqtt.MessageHandler = func(client mqtt.Client, msg mqtt.Message) {
	log.Printf("[demux] topic: %s, payload: %s\n", msg.Topic(), string(msg.Payload()))
	c.PublishEvent(*deviceID, string(msg.Payload()))
}
var demuxClient mqtt.Client
var onCommandReceived mqtt.MessageHandler = func(_ mqtt.Client, msg mqtt.Message) {
	log.Printf("[commands] topic: %s, payload: %s\n", msg.Topic(), string(msg.Payload()))
	if msg.Topic() == fmt.Sprintf("/devices/%s/commands", *deviceID) {
		log.Printf("[commands] Command meant for toe: %s\n", string(msg.Payload()))
	} else {
		topicParts := strings.Split(msg.Topic(), "/")
		if len(topicParts) >= 4 {
			internalTopic := fmt.Sprintf("toe/%s", strings.Join(topicParts[3:], "/"))
			log.Printf("[demux] Command forwarded to topic: %s: %s\n", internalTopic, string(msg.Payload()))
			token := demuxClient.Publish(internalTopic, iot.QosAtLeastOnce, false, msg.Payload())
			if token.Wait() && token.Error() != nil {
				log.Println(token.Error())
			}
		}
	}
}

func main() {
	//mqtt.DEBUG = log.New(os.Stdout, "", 0)
	//mqtt.ERROR = log.New(os.Stdout, "", 0)
	log.Println("[main] Entered")

	log.Println("[main] Flags")
	flag.Parse()

	log.Println("[main] Loading Google's roots")
	certpool := x509.NewCertPool()
	pemCerts, err := ioutil.ReadFile(*certsCA)
	if err == nil {
		certpool.AppendCertsFromPEM(pemCerts)
	}

	log.Println("[main] Creating TLS Config")

	config := &tls.Config{
		RootCAs:            certpool,
		ClientAuth:         tls.NoClientCert,
		ClientCAs:          nil,
		InsecureSkipVerify: true,
		Certificates:       []tls.Certificate{},
		MinVersion:         tls.VersionTLS12,
	}

	clientID := fmt.Sprintf(clientIDFormat,
		*projectID,
		*region,
		*registryID,
		*deviceID,
	)

	log.Println("[main] Creating mqtt Client Options")
	opts := mqtt.NewClientOptions()

	broker := fmt.Sprintf("ssl://%v:%v", *bridge.host, *bridge.port)
	log.Printf("[main] Broker '%v'", broker)

	opts.AddBroker(broker)
	opts.SetClientID(clientID).SetTLSConfig(config)
	opts.SetConnectTimeout(timeout)
	opts.SetKeepAlive(60 * time.Second)
	opts.SetAutoReconnect(true)
	opts.SetProtocolVersion(protocolVersion)
	opts.SetStore(mqtt.NewMemoryStore())
	opts.SetUsername("unused")

	token := jwt.New(jwt.SigningMethodRS256)
	token.Claims = jwt.StandardClaims{
		Audience:  *projectID,
		IssuedAt:  time.Now().Unix(),
		ExpiresAt: time.Now().Add(24 * time.Hour).Unix(),
	}

	log.Println("[main] Load Private Key")
	keyBytes, err := ioutil.ReadFile(*privateKey)
	if err != nil {
		log.Fatal(err)
	}

	log.Println("[main] Parse Private Key")
	key, err := jwt.ParseRSAPrivateKeyFromPEM(keyBytes)
	if err != nil {
		log.Fatal(err)
	}

	log.Println("[main] Sign String")
	tokenString, err := token.SignedString(key)
	if err != nil {
		log.Fatal(err)
	}

	opts.SetPassword(tokenString)

	log.Println("[main] Creating Handler to Subscribe on Connection")
	opts.SetOnConnectHandler(func(cli mqtt.Client) {
		{
			token := cli.Subscribe(fmt.Sprintf(iot.TopicFormat, *deviceID, "config"), qosAtLeastOnce, iot.OnConfigReceived)
			if token.Wait() && token.Error() != nil {
				log.Fatal(token.Error())
			}
		}
		{
			token := cli.Subscribe(fmt.Sprintf(iot.TopicFormat, *deviceID, "state"), qosAtLeastOnce, func(client mqtt.Client, msg mqtt.Message) {
				log.Printf("[state] topic: %s, payload: %s\n", msg.Topic(), string(msg.Payload()))
			})
			if token.Wait() && token.Error() != nil {
				log.Fatal(token.Error())
			}
		}
		{
			// https://cloud.google.com/iot/docs/how-tos/commands?hl=ja
			token := cli.Subscribe(fmt.Sprintf(iot.TopicFormat, *deviceID, "commands")+"/#", qosAtLeastOnce, onCommandReceived)
			if token.Wait() && token.Error() != nil {
				log.Fatal(token.Error())
			}
		}
	})

	log.Println("[main] mqtt Client Connecting")
	c = iot.NewCloudIotClient(opts)
	cli := c.Client()
	defer cli.Disconnect(250)

	log.Println("[main] MQTT Connected!")
	c.UpdateState(*deviceID, "started")
	defer c.UpdateState(*deviceID, "stopped")

	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	go c.HeartBeat(*deviceID, ticker)

	// DEMUX
	demuxBroker := fmt.Sprintf("tcp://%v:%v", *demuxBroker.host, *demuxBroker.port)
	demuxOpts := mqtt.NewClientOptions().AddBroker(demuxBroker).SetClientID("toe").SetCleanSession(true)
	demuxOpts.OnConnect = func(c mqtt.Client) {
		if token := c.Subscribe("toe/events", 0, demuxEventHandler); token.Wait() && token.Error() != nil {
			log.Fatal(token.Error())
		}
	}
	demuxClient = mqtt.NewClient(demuxOpts)
	if token := demuxClient.Connect(); token.Wait() && token.Error() != nil {
		log.Fatal(token.Error())
	} else {
		log.Printf("[demux] Connected to server")
	}

	signalHandler()
}

func signalHandler() {
	ch := make(chan os.Signal, 0)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	select {
	case sig := <-ch:
		log.Printf("[main] signal received: %s\n", sig)
	}
	os.Exit(0)
}
