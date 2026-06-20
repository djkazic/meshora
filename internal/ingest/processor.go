package ingest

import (
	"encoding/json"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/djkazic/meshora/internal/meshcore"
	"github.com/djkazic/meshora/internal/server"
	"github.com/djkazic/meshora/internal/store"
)

type Publisher interface {
	Broadcast(v any)
}

type Packet struct {
	RawHex       string
	Hash         string
	ObserverID   string
	ObserverName string
	SNR          *float64
	RSSI         *float64
	ResolvedPath []string
	Channel      string
	Text         string
	Ts           int64
}

var chatWhitelist = map[string]bool{"Public": true, "#ma-mesh": true}

type Processor struct {
	st  *store.Store
	pub Publisher

	mu  sync.RWMutex
	pos map[string][2]float64
}

func NewProcessor(st *store.Store, pub Publisher) (*Processor, error) {
	p := &Processor{st: st, pub: pub, pos: map[string][2]float64{}}
	all, err := st.NodePositions()
	if err != nil {
		return nil, err
	}
	for pk, ll := range all {
		p.pos[pk] = ll
	}
	return p, nil
}

func (p *Processor) ReloadPositions() error {
	all, err := p.st.NodePositions()
	if err != nil {
		return err
	}
	next := make(map[string][2]float64, len(all))
	for pk, ll := range all {
		next[pk] = ll
	}
	p.mu.Lock()
	p.pos = next
	p.mu.Unlock()
	return nil
}

func (p *Processor) Handle(pk Packet) {
	decoded, err := meshcore.Decode(pk.RawHex)
	if err != nil {
		return
	}

	hash := pk.Hash
	if hash == "" {
		hash = meshcore.ContentHash(pk.RawHex)
	}

	var originPub *string
	if decoded.Advert != nil && decoded.Advert.PubKey != "" {
		op := decoded.Advert.PubKey
		originPub = &op
		hashSize := 0
		if len(decoded.Hops) > 0 {
			hashSize = decoded.PathHashSize
		}
		p.handleAdvert(decoded.Advert, pk.Ts, hashSize)
	}

	chatChannel, chatText := "", ""
	if decoded.PayloadType == meshcore.PayloadGRPTxt && chatWhitelist[pk.Channel] && pk.Text != "" {
		chatChannel, chatText = pk.Channel, pk.Text
	}

	t := store.Transmission{
		Hash:         hash,
		RawHex:       strings.ToUpper(pk.RawHex),
		RouteType:    decoded.RouteType,
		PayloadType:  decoded.PayloadType,
		OriginPubKey: originPub,
		PathJSON:     joinHops(decoded.Hops),
		ResolvedPath: joinHops(pk.ResolvedPath),
		Channel:      chatChannel,
		MsgText:      chatText,
		PathHashSize: decoded.PathHashSize,
		Ts:           pk.Ts,
	}
	obs := store.Observation{
		ObserverID:   pk.ObserverID,
		ObserverName: pk.ObserverName,
		SNR:          pk.SNR,
		RSSI:         pk.RSSI,
		Ts:           pk.Ts,
	}
	isNew, err := p.st.RecordTransmission(t, obs)
	if err != nil {
		log.Printf("record transmission: %v", err)
		return
	}

	if !isNew {
		return
	}
	now := time.Now().Unix()

	detail := ""
	if decoded.Advert != nil {
		if decoded.Advert.Name != "" {
			detail = decoded.Advert.Name
		} else if len(decoded.Advert.PubKey) >= 8 {
			detail = decoded.Advert.PubKey[:8]
		}
	} else if chatText != "" {
		detail = chatText
	}
	p.pub.Broadcast(server.PacketEvent{
		Type:             "packet",
		Hash:             hash,
		PayloadType:      decoded.PayloadType,
		PayloadName:      decoded.PayloadTypeName,
		RouteName:        decoded.RouteTypeName,
		FirstSeen:        now,
		ObservationCount: 1,
		Hops:             len(decoded.Hops),
		Detail:           detail,
	})

	wp := p.buildWaypoints(decoded, pk.ResolvedPath, pk.ObserverID)
	if len(wp) == 0 {
		return
	}

	f := server.NewFlow()
	f.Hash = hash
	f.PayloadType = decoded.PayloadType
	f.PayloadName = decoded.PayloadTypeName
	f.RouteType = decoded.RouteType
	f.SNR = pk.SNR
	f.Waypoints = wp
	f.Ts = now
	p.pub.Broadcast(f)

	if wpJSON, err := json.Marshal(wp); err == nil {
		_ = p.st.InsertFlow(store.FlowRow{
			Hash:        f.Hash,
			PayloadType: f.PayloadType,
			PayloadName: f.PayloadName,
			RouteType:   f.RouteType,
			SNR:         f.SNR,
			Waypoints:   string(wpJSON),
			Ts:          f.Ts,
		})
	}
}

func (p *Processor) handleAdvert(a *meshcore.Advert, ts int64, hashSize int) {
	n := store.Node{
		PubKey:    a.PubKey,
		Name:      a.Name,
		Role:      a.Role,
		Lat:       a.Lat,
		Lon:       a.Lon,
		FirstSeen: ts,
		LastSeen:  ts,
	}
	if hashSize >= 1 && hashSize <= 3 {
		n.HashSize = &hashSize
	}
	if err := p.st.UpsertNode(n); err != nil {
		log.Printf("upsert node: %v", err)
		return
	}
	if a.Lat == nil || a.Lon == nil {
		return
	}
	p.mu.Lock()
	p.pos[a.PubKey] = [2]float64{*a.Lat, *a.Lon}
	p.mu.Unlock()

	ev := server.NodeEvent{Type: "node", Node: n}
	p.pub.Broadcast(ev)
}

func (p *Processor) buildWaypoints(decoded *meshcore.Packet, resolved []string, observerID string) [][2]float64 {
	p.mu.RLock()
	defer p.mu.RUnlock()

	seq := make([]string, 0, len(resolved)+2)
	if decoded.Advert != nil && decoded.Advert.PubKey != "" {
		seq = append(seq, decoded.Advert.PubKey)
	}
	if len(resolved) > 0 {
		for _, pk := range resolved {
			if pk != "" {
				seq = append(seq, strings.ToLower(pk))
			}
		}
	} else {
		for _, hop := range decoded.Hops {
			if pk := p.resolvePrefix(hop); pk != "" {
				seq = append(seq, pk)
			}
		}
	}
	if observerID != "" {
		seq = append(seq, strings.ToLower(observerID))
	}

	wp := make([][2]float64, 0, len(seq))
	for _, pk := range seq {
		ll, ok := p.pos[pk]
		if !ok {
			continue
		}
		if len(wp) > 0 && wp[len(wp)-1] == ll {
			continue
		}
		wp = append(wp, ll)
	}
	return wp
}

func (p *Processor) resolvePrefix(hop string) string {
	prefix := strings.ToLower(hop)
	match := ""
	for pk := range p.pos {
		if strings.HasPrefix(pk, prefix) {
			if match != "" {
				return ""
			}
			match = pk
		}
	}
	return match
}

func joinHops(h []string) string {
	if len(h) == 0 {
		return ""
	}
	return strings.Join(h, ",")
}
