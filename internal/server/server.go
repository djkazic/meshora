package server

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/djkazic/meshora/internal/geo"
	"github.com/djkazic/meshora/internal/meshcore"
	"github.com/djkazic/meshora/internal/store"
)

type Server struct {
	Hub  *Hub
	st   *store.Store
	bbox geo.BBox
	mux  *http.ServeMux
}

func New(st *store.Store, hub *Hub, bbox geo.BBox, staticFS fs.FS) *Server {
	s := &Server{Hub: hub, st: st, bbox: bbox, mux: http.NewServeMux()}
	s.mux.HandleFunc("/api/stats", s.handleStats)
	s.mux.HandleFunc("/api/nodes", s.handleNodes)
	s.mux.HandleFunc("/api/nodes/{pubkey}", s.handleNode)
	s.mux.HandleFunc("/api/nodes/{pubkey}/paths", s.handleNodePaths)
	s.mux.HandleFunc("/api/flows/recent", s.handleRecentFlows)
	s.mux.HandleFunc("/api/packets", s.handlePacketList)
	s.mux.HandleFunc("/api/packets/{hash}", s.handlePacket)
	s.mux.HandleFunc("/api/config", s.handleConfig)
	s.mux.HandleFunc("/ws", s.handleWS)
	s.mux.Handle("/", cacheControl(http.FileServer(http.FS(staticFS))))
	return s
}

func (s *Server) Handler() http.Handler { return s.mux }

const nodeMaxAgeSecs = 7 * 24 * 3600

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	st, err := s.st.Stats()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, struct {
		store.Stats
		Clients int `json:"clients"`
	}{st, s.Hub.ClientCount()})
}

func (s *Server) handleNodes(w http.ResponseWriter, r *http.Request) {
	nodes, err := s.st.NodesPositioned(time.Now().Unix(), nodeMaxAgeSecs)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if nodes == nil {
		nodes = []store.Node{}
	}
	writeJSON(w, nodes)
}

func (s *Server) handleNodePaths(w http.ResponseWriter, r *http.Request) {
	groups, err := s.st.PathsThroughNode(r.PathValue("pubkey"), 25)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	seen := map[string]bool{}
	for _, g := range groups {
		for _, pk := range splitNonEmpty(g.ResolvedPath) {
			seen[pk] = true
		}
	}
	pks := make([]string, 0, len(seen))
	for pk := range seen {
		pks = append(pks, pk)
	}
	infos, _ := s.st.NodeInfos(pks)

	type hop struct {
		Pubkey string `json:"pubkey"`
		Name   string `json:"name"`
		Role   string `json:"role"`
	}
	type pathOut struct {
		Count    int   `json:"count"`
		LastSeen int64 `json:"last_seen"`
		Hops     []hop `json:"hops"`
	}
	out := make([]pathOut, 0, len(groups))
	for _, g := range groups {
		hops := []hop{}
		for _, pk := range splitNonEmpty(g.ResolvedPath) {
			hops = append(hops, hop{Pubkey: pk, Name: infos[pk].Name, Role: infos[pk].Role})
		}
		out = append(out, pathOut{Count: g.Count, LastSeen: g.LastSeen, Hops: hops})
	}
	writeJSON(w, out)
}

func (s *Server) handlePacketList(w http.ResponseWriter, r *http.Request) {
	limit := 200
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 && v <= 1000 {
		limit = v
	}
	payloadType := -1
	if v, err := strconv.Atoi(r.URL.Query().Get("type")); err == nil {
		payloadType = v
	}
	var nodePubkeys []string
	if node := r.URL.Query().Get("node"); node != "" {
		nodePubkeys, _ = s.st.NodePubkeysMatching(node, 40)
		if len(nodePubkeys) == 0 {
			writeJSON(w, []any{})
			return
		}
	}
	rows, err := s.st.RecentPackets(limit, payloadType, nodePubkeys)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var advPks []string
	for _, p := range rows {
		if p.PayloadType == meshcore.PayloadADVERT && p.OriginPubKey != nil {
			advPks = append(advPks, *p.OriginPubKey)
		}
	}
	names, _ := s.st.NodeInfos(advPks)

	type item struct {
		Hash             string `json:"hash"`
		PayloadType      int    `json:"payload_type"`
		PayloadName      string `json:"payload_name"`
		RouteName        string `json:"route_name"`
		FirstSeen        int64  `json:"first_seen"`
		ObservationCount int    `json:"observation_count"`
		Hops             int    `json:"hops"`
		Detail           string `json:"detail"`
	}
	out := make([]item, 0, len(rows))
	for _, p := range rows {
		detail := ""
		if p.PayloadType == meshcore.PayloadADVERT && p.OriginPubKey != nil {
			if info, ok := names[*p.OriginPubKey]; ok && info.Name != "" {
				detail = info.Name
			} else {
				detail = (*p.OriginPubKey)[:min(8, len(*p.OriginPubKey))]
			}
		} else if p.MsgText != nil && *p.MsgText != "" {
			detail = *p.MsgText
		}
		out = append(out, item{
			Hash:             p.Hash,
			PayloadType:      p.PayloadType,
			PayloadName:      meshcore.PayloadName(p.PayloadType),
			RouteName:        meshcore.RouteName(p.RouteType),
			FirstSeen:        p.FirstSeen,
			ObservationCount: p.ObservationCount,
			Hops:             p.Hops,
			Detail:           detail,
		})
	}
	writeJSON(w, out)
}

func (s *Server) handleNode(w http.ResponseWriter, r *http.Request) {
	n, err := s.st.GetNode(r.PathValue("pubkey"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if n == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	writeJSON(w, n)
}

func (s *Server) handleRecentFlows(w http.ResponseWriter, r *http.Request) {
	mins := 5
	if v, err := strconv.Atoi(r.URL.Query().Get("mins")); err == nil && v > 0 && v <= 30 {
		mins = v
	}
	since := time.Now().Add(-time.Duration(mins) * time.Minute).Unix()
	rows, err := s.st.RecentFlows(since, 2000)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	out := make([]Flow, 0, len(rows))
	for _, fr := range rows {
		f := NewFlow()
		f.Hash = fr.Hash
		f.PayloadType = fr.PayloadType
		f.PayloadName = fr.PayloadName
		f.RouteType = fr.RouteType
		f.SNR = fr.SNR
		f.Ts = fr.Ts
		if json.Unmarshal([]byte(fr.Waypoints), &f.Waypoints) != nil {
			continue
		}
		out = append(out, f)
	}
	writeJSON(w, out)
}

func (s *Server) handlePacket(w http.ResponseWriter, r *http.Request) {
	pkt, err := s.st.GetPacket(r.PathValue("hash"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if pkt == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	decoded, _ := meshcore.Decode(pkt.RawHex)

	hops := splitNonEmpty(pkt.PathJSON)
	resolvedPks := splitNonEmpty(pkt.ResolvedPath)
	infos, _ := s.st.NodeInfos(resolvedPks)
	type hop struct {
		Pubkey string `json:"pubkey"`
		Name   string `json:"name"`
		Role   string `json:"role"`
	}
	resolved := make([]hop, 0, len(resolvedPks))
	for _, pk := range resolvedPks {
		resolved = append(resolved, hop{Pubkey: pk, Name: infos[pk].Name, Role: infos[pk].Role})
	}

	resp := map[string]any{
		"hash":              pkt.Hash,
		"raw_hex":           pkt.RawHex,
		"route_type":        pkt.RouteType,
		"route_name":        meshcore.RouteName(pkt.RouteType),
		"payload_type":      pkt.PayloadType,
		"payload_name":      meshcore.PayloadName(pkt.PayloadType),
		"first_seen":        pkt.FirstSeen,
		"last_seen":         pkt.LastSeen,
		"observation_count": pkt.ObservationCount,
		"hops":              hops,
		"resolved":          resolved,
		"observers":         dedupObservers(pkt.Observations),
		"advert":            nil,
		"channel":           deref(pkt.Channel),
		"message":           deref(pkt.MsgText),
	}
	if decoded != nil {
		resp["payload_version"] = decoded.PayloadVersion
		if len(decoded.Hops) > 0 {
			resp["hops"] = decoded.Hops
		}
		if a := decoded.Advert; a != nil {
			resp["advert"] = map[string]any{
				"pubkey": a.PubKey, "name": a.Name, "role": a.Role, "lat": a.Lat, "lon": a.Lon,
			}
		}
	}
	writeJSON(w, resp)
}

func dedupObservers(obs []store.PacketObs) []map[string]any {
	type agg struct {
		o     store.PacketObs
		count int
	}
	byObs := map[string]*agg{}
	var order []string
	for _, o := range obs {
		key := o.ObserverID
		if key == "" {
			key = o.ObserverName
		}
		a := byObs[key]
		if a == nil {
			a = &agg{o: o}
			byObs[key] = a
			order = append(order, key)
		}
		a.count++
		if o.Ts < a.o.Ts || a.o.Ts == 0 {
			a.o.Ts = o.Ts
		}
		if a.o.SNR == nil && o.SNR != nil {
			a.o.SNR = o.SNR
		}
		if a.o.RSSI == nil && o.RSSI != nil {
			a.o.RSSI = o.RSSI
		}
	}
	out := make([]map[string]any, 0, len(order))
	for _, key := range order {
		a := byObs[key]
		out = append(out, map[string]any{
			"observer_id":   a.o.ObserverID,
			"observer_name": a.o.ObserverName,
			"snr":           a.o.SNR,
			"rssi":          a.o.RSSI,
			"ts":            a.o.Ts,
			"count":         a.count,
		})
	}
	return out
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func splitNonEmpty(csv string) []string {
	out := []string{}
	for _, s := range strings.Split(csv, ",") {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{
		"bbox": map[string]float64{
			"minLat": s.bbox.MinLat, "maxLat": s.bbox.MaxLat,
			"minLon": s.bbox.MinLon, "maxLon": s.bbox.MaxLon,
		},
		"center": map[string]float64{"lat": geo.Center.Lat, "lon": geo.Center.Lon},
	})
}

func cacheControl(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if strings.HasPrefix(p, "/assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache, must-revalidate")
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
