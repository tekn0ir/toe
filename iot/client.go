package iot

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	appsv1 "k8s.io/api/apps/v1"
	apiv1 "k8s.io/api/core/v1"
)

const (
	TopicFormat = "/devices/%s/%s"
)

const (
	QosAtMostOnce byte = iota
	QosAtLeastOnce
	QosExactlyOnce
)

type CloudIotClient interface {
	Client() mqtt.Client
	ReAuth(ticker *time.Ticker, opts *mqtt.ClientOptions, projectID string, privateKeyPath string, expiration time.Duration)
	HeartBeat(deviceID string, ticker *time.Ticker)
	UpdateState(deviceID, state string) error
	PublishEvent(deviceID, payload string) error
	Disconnect(quiesce uint)
}

type cloudIotClient struct {
	client mqtt.Client
}

// NewCloudIotClient returns mqtt client.
func NewCloudIotClient(opts *mqtt.ClientOptions, projectID string, privateKeyPath string, expiration time.Duration) CloudIotClient {
	if opts == nil {
		log.Fatal("[iot] MQTT Client Options are nil")
	}

	log.Println("[iot] Create JWT Token")
	tokenString, err := CreateJWTToken(projectID, privateKeyPath, expiration)
	if err != nil {
		log.Fatal(err)
	}

	opts.SetPassword(tokenString)

	cli := mqtt.NewClient(opts)
	if token := cli.Connect(); token.Wait() && token.Error() != nil {
		log.Fatalf("[iot] Error: %+v\n", token.Error())
	}

	return &cloudIotClient{
		client: cli,
	}
}

func (c *cloudIotClient) ReAuth(ticker *time.Ticker, opts *mqtt.ClientOptions, projectID string, privateKeyPath string, expiration time.Duration) {
	for t := range ticker.C {
		log.Println("[iot] Token claim has expired, reauthenticating at:", t)
		ClientMutex.Lock()
		log.Println("[iot] Disconnect")
		c.client.Disconnect(125)

		log.Println("[iot] Update JWT Token")
		tokenString, err := CreateJWTToken(projectID, privateKeyPath, expiration)
		if err != nil {
			log.Fatal(err)
		}

		opts.SetPassword(tokenString)

		log.Println("[iot] Reconnect")
		c.client = mqtt.NewClient(opts)
		if token := c.client.Connect(); token.Wait() && token.Error() != nil {
			log.Fatalf("[iot] Error: %+v\n", token.Error())
		}

		log.Println("[iot] New token claimed successfully")
		ClientMutex.Unlock()
	}
}

func (c *cloudIotClient) Client() mqtt.Client {
	return c.client
}

func (c *cloudIotClient) Disconnect(quiesce uint) {
	ClientMutex.Lock()
	defer ClientMutex.Unlock()
	c.client.Disconnect(quiesce)
}

func (c *cloudIotClient) HeartBeat(deviceID string, ticker *time.Ticker) {
	for t := range ticker.C {
		log.Println("[heartbeat] timestamp", t)
		c.UpdateState(deviceID, "heartBeat")
	}
}

func PrettyPrint(v interface{}) (err error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err == nil {
		log.Println(string(b))
	}
	return
}

type CommandStruct struct {
	Command  string `json:"command"`
}

type LocationStruct struct {
	Lat      float64 `json:"lat"`
	Lng      float64 `json:"lng"`
}

type LocationMessage struct {
	Location LocationStruct `json:"location"`
	Accuracy float64 `json:"accuracy"`
}

type App struct {
	Version  string `json:"version"`
	Status string `json:"status"`
	Restarts int32 `json:"restarts"`
}

type StateMessage struct {
	Location LocationStruct `json:"location"`
	Accuracy float64 `json:"accuracy"`
	Apps map[string]App `json:"apps"`
}

var StateMsg = StateMessage{
	Location: LocationStruct{
		Lat:      29.7604,
		Lng:      -95.3698,
	},
	Accuracy: 1000.0,
	Apps: map[string]App{},
}
var ClientMutex = &sync.Mutex{}

func getDeploymentCondition(status appsv1.DeploymentStatus, condType appsv1.DeploymentConditionType) *appsv1.DeploymentCondition {
	for i := range status.Conditions {
		c := status.Conditions[i]
		if c.Type == condType {
			return &c
		}
	}
	return nil
}

func (c *cloudIotClient) UpdateState(deviceID, state string) error {
	ClientMutex.Lock()
	defer ClientMutex.Unlock()
	var err error
	var pods apiv1.PodList
	pods, err = GetCurrentPods(flag.Lookup("kube_config").Value.(flag.Getter).Get().(string))
	if err != nil {
		log.Println("[state] Error: ", err)
		return err
	}

	for _, d := range pods.Items {
		//PrettyPrint(d)
		for _, c := range d.Status.ContainerStatuses {
			var app App
			app.Version = c.Image
			app.Restarts = c.RestartCount
			b, err := json.MarshalIndent(c.State, "", "  ")
			if err != nil {
				log.Println("[state] Error: ", err)
				return err
			}
			app.Status = string(b)
			StateMsg.Apps[c.Name] = app
		}
	}

	topic := fmt.Sprintf(TopicFormat, deviceID, "state")
	payload, err := json.Marshal(StateMsg)
	if err != nil {
		log.Println("[state] Error: ", err)
		return err
	}
	log.Println("[state] Sending state: ", string(payload))
	token := c.client.Publish(topic, QosAtLeastOnce, false, payload)
	if token.Wait() && token.Error() != nil {
		log.Println(token.Error())
		return token.Error()
	}
	return nil
}

func (c *cloudIotClient) PublishEvent(deviceID, payload string) error {
	ClientMutex.Lock()
	defer ClientMutex.Unlock()
	topic := fmt.Sprintf(TopicFormat, deviceID, "events")
	token := c.client.Publish(topic, QosAtLeastOnce, false, payload)
	if token.Wait() && token.Error() != nil {
		log.Println(token.Error())
		return token.Error()
	}
	return nil
}
