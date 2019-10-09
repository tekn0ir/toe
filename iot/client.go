package iot

import (
	"fmt"
	"log"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
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
	PublishEvent(deviceID, eventName string) error
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

func (c *cloudIotClient) UpdateState(deviceID, state string) error {
	topic := fmt.Sprintf(TopicFormat, deviceID, "state")
	token := c.client.Publish(topic, QosAtLeastOnce, false, state)
	if token.Wait() && token.Error() != nil {
		log.Println(token.Error())
		return token.Error()
	}
	return nil
}

func (c *cloudIotClient) PublishEvent(deviceID, eventName string) error {
	topic := fmt.Sprintf(TopicFormat, deviceID, "events/"+eventName)
	token := c.client.Publish(topic, QosAtLeastOnce, false, "on")
	if token.Wait() && token.Error() != nil {
		log.Println(token.Error())
		return token.Error()
	}
	return nil
}
