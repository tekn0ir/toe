package ork3strate

import (
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"log"
)

//type Ork3strator interface {
//	Client() mqtt.Client
//	HeartBeat(deviceID string, ticker *time.Ticker)
//	UpdateState(deviceID, state string) error
//	PublishEvent(deviceID, eventName string) error
//}

func onConfigReceived(client mqtt.Client, msg mqtt.Message) {
	log.Printf("[config] topic: %s, payload: %s\n", msg.Topic(), string(msg.Payload()))
}
