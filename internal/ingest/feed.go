package ingest

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/gorilla/websocket"
)

type FeedSource struct {
	URL  string
	proc *Processor
}

func NewFeedSource(url string, proc *Processor) *FeedSource {
	return &FeedSource{URL: url, proc: proc}
}

type feedEnvelope struct {
	Type string `json:"type"`
	Data struct {
		RawHex       string   `json:"raw_hex"`
		Hash         string   `json:"hash"`
		ObserverID   string   `json:"observer_id"`
		ObserverName string   `json:"observer_name"`
		SNR          *float64 `json:"snr"`
		RSSI         *float64 `json:"rssi"`
		ResolvedPath []string `json:"resolved_path"`
		Timestamp    string   `json:"timestamp"`
		FirstSeen    string   `json:"first_seen"`
		Decoded      struct {
			Payload struct {
				Channel          string `json:"channel"`
				Text             string `json:"text"`
				DecryptionStatus string `json:"decryptionStatus"`
			} `json:"payload"`
		} `json:"decoded"`
	} `json:"data"`
}

func (f *FeedSource) Run(ctx context.Context) {
	backoff := time.Second
	for ctx.Err() == nil {
		if err := f.stream(ctx); err != nil && ctx.Err() == nil {
			log.Printf("feed: %v; reconnecting in %s", err, backoff)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			if backoff < 30*time.Second {
				backoff *= 2
			}
			continue
		}
		backoff = time.Second
	}
}

func (f *FeedSource) stream(ctx context.Context) error {
	d := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
	conn, _, err := d.DialContext(ctx, f.URL, nil)
	if err != nil {
		return err
	}
	defer conn.Close()
	log.Printf("feed: connected to %s", f.URL)

	go func() {
		<-ctx.Done()
		conn.Close()
	}()

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		var env feedEnvelope
		if json.Unmarshal(msg, &env) != nil || env.Type != "packet" || env.Data.RawHex == "" {
			continue
		}
		ts := parseTS(env.Data.Timestamp)
		if ts == 0 {
			ts = parseTS(env.Data.FirstSeen)
		}
		if ts == 0 {
			ts = time.Now().Unix()
		}
		pk := Packet{
			RawHex:       env.Data.RawHex,
			Hash:         env.Data.Hash,
			ObserverID:   env.Data.ObserverID,
			ObserverName: env.Data.ObserverName,
			SNR:          env.Data.SNR,
			RSSI:         env.Data.RSSI,
			ResolvedPath: env.Data.ResolvedPath,
			Ts:           ts,
		}
		if env.Data.Decoded.Payload.DecryptionStatus == "decrypted" {
			pk.Channel = env.Data.Decoded.Payload.Channel
			pk.Text = env.Data.Decoded.Payload.Text
		}
		f.proc.Handle(pk)
	}
}

func parseTS(s string) int64 {
	if s == "" {
		return 0
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.Unix()
	}
	return 0
}
