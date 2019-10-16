package iot

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	appsv1 "k8s.io/api/apps/v1"
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
	HeartBeat(deviceID string, ticker *time.Ticker)
	UpdateState(deviceID, state string) error
	PublishEvent(deviceID, payload string) error
}

type cloudIotClient struct {
	client mqtt.Client
}

// NewCloudIotClient returns mqtt client.
func NewCloudIotClient(opts *mqtt.ClientOptions) CloudIotClient {
	if opts == nil {
		opts = mqtt.NewClientOptions()
	}

	cli := mqtt.NewClient(opts)
	if token := cli.Connect(); token.Wait() && token.Error() != nil {
		log.Fatalf("Error: %+v\n", token.Error())
	}

	return &cloudIotClient{
		client: cli,
	}
}

func (c *cloudIotClient) Client() mqtt.Client {
	return c.client
}

func (c *cloudIotClient) HeartBeat(deviceID string, ticker *time.Ticker) {
	for t := range ticker.C {
		log.Println("[heartbeat] timestamp", t)
		c.UpdateState(deviceID, "heartBeat")
	}
}

type LocationStruct struct {
	Latitude  float64 `json:"lat"`
	Longitude float64 `json:"lng"`
	Accuracy  int     `json:"accuracy"`
}

type LocationMessage struct {
	Location LocationStruct `json:"location"`
}

type StateMessage struct {
	Location LocationStruct `json:"location"`
	Deployments appsv1.DeploymentList `json:"deployments"`
}

var StateMsg = StateMessage{
	Location: LocationStruct{
		Latitude: 29.7604,
		Longitude: 95.3698,
		Accuracy: 107,
	},
	Deployments: appsv1.DeploymentList{},
}

func (c *cloudIotClient) UpdateState(deviceID, state string) error {
	var err error
	StateMsg.Deployments, err = GetCurrentDeployments(flag.Lookup("kube_config").Value.(flag.Getter).Get().(string))
	if err != nil {
		log.Println("[state] Error: ", err)
		return err
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
	topic := fmt.Sprintf(TopicFormat, deviceID, "events")
	token := c.client.Publish(topic, QosAtLeastOnce, false, payload)
	if token.Wait() && token.Error() != nil {
		log.Println(token.Error())
		return token.Error()
	}
	return nil
}
