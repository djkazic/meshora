package ingest

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"log"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type BrokerSource struct {
	Broker   string
	Username string
	Password string
	Topic    string
	proc     *Processor
}

func NewBrokerSource(broker, username, password, topic string, proc *Processor) *BrokerSource {
	if topic == "" {
		topic = "meshcore/#"
	}
	return &BrokerSource{Broker: broker, Username: username, Password: password, Topic: topic, proc: proc}
}

type observerEnvelope struct {
	Raw       string   `json:"raw"`
	SNR       *float64 `json:"SNR"`
	RSSI      *float64 `json:"RSSI"`
	Origin    string   `json:"origin"`
	Timestamp string   `json:"timestamp"`
}

func (b *BrokerSource) Run(ctx context.Context) error {
	opts := mqtt.NewClientOptions().
		AddBroker(b.Broker).
		SetClientID("meshora-" + randomSuffix()).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectTimeout(10 * time.Second).
		SetKeepAlive(30 * time.Second).
		SetTLSConfig(&tls.Config{})
	if b.Username != "" {
		opts.SetUsername(b.Username)
	}
	if b.Password != "" {
		opts.SetPassword(b.Password)
	}
	opts.SetDefaultPublishHandler(func(_ mqtt.Client, m mqtt.Message) {
		var env observerEnvelope
		if json.Unmarshal(m.Payload(), &env) != nil || env.Raw == "" {
			return
		}
		b.proc.Handle(Packet{
			RawHex:       env.Raw,
			ObserverID:   observerFromTopic(m.Topic()),
			ObserverName: env.Origin,
			SNR:          env.SNR,
			RSSI:         env.RSSI,
			Ts:           parseTSOrNow(env.Timestamp),
		})
	})

	c := mqtt.NewClient(opts)
	if t := c.Connect(); t.Wait() && t.Error() != nil {
		return t.Error()
	}
	log.Printf("broker: connected to %s", b.Broker)
	if t := c.Subscribe(b.Topic, 0, nil); t.Wait() && t.Error() != nil {
		c.Disconnect(250)
		return t.Error()
	}
	log.Printf("broker: subscribed to %s", b.Topic)

	<-ctx.Done()
	c.Disconnect(250)
	return nil
}

func observerFromTopic(topic string) string {
	parts := splitTopic(topic)
	if len(parts) >= 3 {
		return parts[2]
	}
	return ""
}

func splitTopic(t string) []string {
	return strings.Split(t, "/")
}

func parseTSOrNow(s string) int64 {
	if ts := parseTS(s); ts != 0 {
		return ts
	}
	return time.Now().Unix()
}

func randomSuffix() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
