package server

import "github.com/djkazic/meshora/internal/store"

type Flow struct {
	Type        string       `json:"type"`
	Hash        string       `json:"hash"`
	PayloadType int          `json:"payload_type"`
	PayloadName string       `json:"payload_name"`
	RouteType   int          `json:"route_type"`
	SNR         *float64     `json:"snr"`
	Waypoints   [][2]float64 `json:"waypoints"`
	Ts          int64        `json:"ts"`
}

type NodeEvent struct {
	Type string     `json:"type"`
	Node store.Node `json:"node"`
}

type PacketEvent struct {
	Type             string `json:"type"`
	Hash             string `json:"hash"`
	PayloadType      int    `json:"payload_type"`
	PayloadName      string `json:"payload_name"`
	RouteName        string `json:"route_name"`
	FirstSeen        int64  `json:"first_seen"`
	ObservationCount int    `json:"observation_count"`
	Hops             int    `json:"hops"`
	Detail           string `json:"detail"`
}

func NewFlow() Flow { return Flow{Type: "flow"} }
