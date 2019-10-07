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
	"syscall"
	"time"

	jwt "github.com/dgrijalva/jwt-go"
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
	projectID  = flag.String("project", getEnv("TOE_PROJECT", "no-default-project-id"), "GCP Project ID")
	registryID = flag.String("registry", getEnv("TOE_IOT_REGISTRY", "no-default-registry-id"), "Cloud IoT Registry ID (short form)")
	region     = flag.String("region", getEnv("TOE_REGION", "us-central1"), "GCP Region")
	certsCA    = flag.String("ca_certs", getEnv("TOE_CA_CERT", "no-default-ca-cert"), "Download https://pki.google.com/roots.pem")
	privateKey = flag.String("private_key", getEnv("TOE_PRIVATE_KEY", "no-default-private-key"), "Path to private key file")
)

func main() {
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
			token := cli.Subscribe(fmt.Sprintf(iot.TopicFormat, *deviceID, "config"), qosAtLeastOnce, func(client mqtt.Client, msg mqtt.Message) {
				log.Printf("[config] topic: %s, payload: %s\n", msg.Topic(), string(msg.Payload()))
			})
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
			token := cli.Subscribe(fmt.Sprintf(iot.TopicFormat, *deviceID, "commands")+"/#", qosAtLeastOnce, func(client mqtt.Client, msg mqtt.Message) {
				log.Printf("[commands] topic: %s, payload: %s\n", msg.Topic(), string(msg.Payload()))
			})
			if token.Wait() && token.Error() != nil {
				log.Fatal(token.Error())
			}
		}
	})

	log.Println("[main] mqtt Client Connecting")
	c := iot.NewCloudIotClient(opts)
	cli := c.Client()
	defer cli.Disconnect(250)

	log.Println("[main] MQTT Connected!")
	c.UpdateState(*deviceID, "started")
	defer c.UpdateState(*deviceID, "stopped")

	c.PublishEvent(*deviceID, "button")

	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	go c.HeartBeat(*deviceID, ticker)

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

//// Incoming
//opts.SetDefaultPublishHandler(func(client mqtt.Client, msg mqtt.Message) {
//	fmt.Printf("[handler] Topic: %v\n", msg.Topic())
//	fmt.Printf("[handler] Payload: %v\n", msg.Payload())
//})
//
//log.Println("[main] mqtt Client Connecting")
//client := mqtt.NewClient(opts)
//if token := client.Connect(); token.Wait() && token.Error() != nil {
//	log.Fatal(token.Error())
//}
//
//topic := struct {
//	config    string
//	telemetry string
//}{
//	config:    fmt.Sprintf("/devices/%v/config", *deviceID),
//	telemetry: fmt.Sprintf("/devices/%v/events", *deviceID),
//}
//
//log.Println("[main] Creating Subscription")
//client.Subscribe(topic.config, 0, nil)
//
//log.Println("[main] Publishing Messages")
//for i := 0; i < 10; i++ {
//	log.Printf("[main] Publishing Message #%d", i)
//	token := client.Publish(
//		topic.telemetry,
//		0,
//		false,
//		fmt.Sprintf("Message: %d", i))
//	token.WaitTimeout(5 * time.Second)
//}
//
//log.Println("[main] mqtt Client Disconnecting")
//client.Disconnect(250)
//
//log.Println("[main] Done")
//}

//package main
//
//import (
//"crypto/tls"
//"crypto/x509"
//"fmt"
//"io/ioutil"
//"log"
//"os"
//"os/signal"
//"syscall"
//"time"
//
//jwt "github.com/dgrijalva/jwt-go"
//mqtt "github.com/eclipse/paho.mqtt.golang"
//"github.com/ww24/cloud-iot-mqtt/iot"
//)
//
//const (
//	timeout         = 30 * time.Second
//	protocolVersion = 4 // mqtt 3.1.1
//	clientIDFormat  = "projects/%s/locations/%s/registries/%s/devices/%s"
//)
//
//const (
//	qosAtMostOnce byte = iota
//	qosAtLeastOnce
//	qosExactlyOnce
//)
//
//var (
//	broker      = os.Getenv("BROKER")
//	projectID   = os.Getenv("PROJECT_ID")
//	cloudRegion = os.Getenv("CLOUD_REGION")
//	registoryID = os.Getenv("REGISTORY_ID")
//	deviceID    = os.Getenv("DEVICE_ID")
//)
//
//func main() {
//	clientID := fmt.Sprintf(clientIDFormat, projectID, cloudRegion, registoryID, deviceID)
//	log.Printf("Broker: %s, ClientID: %s\n", broker, clientID)
//
//	opts := mqtt.NewClientOptions()
//	opts.AddBroker(broker)
//	opts.SetClientID(clientID)
//	opts.SetConnectTimeout(timeout)
//	opts.SetKeepAlive(60 * time.Second)
//	opts.SetAutoReconnect(true)
//	opts.SetProtocolVersion(protocolVersion)
//	opts.SetStore(mqtt.NewMemoryStore())
//
//	// Set Root CA certificate (optional)
//	data, err := ioutil.ReadFile("roots.pem")
//	if err != nil {
//		log.Printf("Warn: %+v\n", err)
//	} else {
//		pool := x509.NewCertPool()
//		if !pool.AppendCertsFromPEM(data) {
//			log.Fatalf("Error: failed to append root ca")
//		}
//		opts.SetTLSConfig(&tls.Config{
//			RootCAs: pool,
//		})
//	}
//
//	opts.SetUsername("unused")
//
//	cert, err := tls.LoadX509KeyPair("rsa_cert.pem", "rsa_private.pem")
//	if err != nil {
//		log.Fatalf("Error: %+v\n", err)
//	}
//	now := time.Now()
//	t := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.StandardClaims{
//		IssuedAt:  now.Unix(),
//		ExpiresAt: now.Add(time.Hour).Unix(),
//		Audience:  projectID,
//	})
//	password, err := t.SignedString(cert.PrivateKey)
//	if err != nil {
//		log.Fatalf("Error: %+v\n", err)
//	}
//	opts.SetPassword(password)
//
//	opts.SetOnConnectHandler(func(cli mqtt.Client) {
//		{
//			token := cli.Subscribe(fmt.Sprintf(iot.TopicFormat, deviceID, "config"), qosAtLeastOnce, func(client mqtt.Client, msg mqtt.Message) {
//				log.Printf("config:: topic: %s, payload: %s\n", msg.Topic(), string(msg.Payload()))
//			})
//			if token.Wait() && token.Error() != nil {
//				log.Fatal(token.Error())
//			}
//		}
//		{
//			token := cli.Subscribe(fmt.Sprintf(iot.TopicFormat, deviceID, "state"), qosAtLeastOnce, func(client mqtt.Client, msg mqtt.Message) {
//				log.Printf("state:: topic: %s, payload: %s\n", msg.Topic(), string(msg.Payload()))
//			})
//			if token.Wait() && token.Error() != nil {
//				log.Fatal(token.Error())
//			}
//		}
//		{
//			// https://cloud.google.com/iot/docs/how-tos/commands?hl=ja
//			token := cli.Subscribe(fmt.Sprintf(iot.TopicFormat, deviceID, "commands")+"/#", qosAtLeastOnce, func(client mqtt.Client, msg mqtt.Message) {
//				log.Printf("commands:: topic: %s, payload: %s\n", msg.Topic(), string(msg.Payload()))
//			})
//			if token.Wait() && token.Error() != nil {
//				log.Fatal(token.Error())
//			}
//		}
//	})
//
//	c := iot.NewCloudIotClient(opts)
//	cli := c.Client()
//	defer cli.Disconnect(250)
//
//	log.Println("CONNECTED!")
//	c.UpdateState(deviceID, "started")
//	defer c.UpdateState(deviceID, "stopped")
//
//	c.PublishEvent(deviceID, "button")
//
//	ticker := time.NewTicker(time.Minute)
//	defer ticker.Stop()
//	go c.HeartBeat(deviceID, ticker)
//
//	signalHandler()
//}
//
//func signalHandler() {
//	ch := make(chan os.Signal, 0)
//	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
//	select {
//	case sig := <-ch:
//		log.Printf("signal received: %s\n", sig)
//	}
//	os.Exit(0)
//}
