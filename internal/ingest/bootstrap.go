package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/djkazic/meshora/internal/store"
)

func httpBase(feedURL string) string {
	base := feedURL
	base = strings.Replace(base, "wss://", "https://", 1)
	base = strings.Replace(base, "ws://", "http://", 1)
	return strings.TrimRight(base, "/")
}

type bootstrapNode struct {
	PublicKey string   `json:"public_key"`
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Role      string   `json:"role"`
	NodeRole  string   `json:"nodeRole"`
	Lat       *float64 `json:"lat"`
	Lon       *float64 `json:"lon"`
	HashSize  *int     `json:"hash_size"`
	LastSeen  string   `json:"last_seen"`
}

func (b bootstrapNode) pubkey() string {
	if b.PublicKey != "" {
		return b.PublicKey
	}
	return b.ID
}

func (b bootstrapNode) role() string {
	if b.Role != "" {
		return b.Role
	}
	if b.NodeRole != "" {
		return b.NodeRole
	}
	return "repeater"
}

func Bootstrap(ctx context.Context, st *store.Store, feedURL string) (int, error) {
	base := httpBase(feedURL)
	var firstErr error
	seeded := 0
	seeded += fetchAndSeed(ctx, st, base+"/api/nodes?limit=5000", &firstErr)
	seeded += fetchAndSeed(ctx, st, base+"/api/observers", &firstErr)
	return seeded, firstErr
}

func fetchAndSeed(ctx context.Context, st *store.Store, url string, firstErr *error) int {
	nodes, err := fetchNodes(ctx, url)
	if err != nil {
		if *firstErr == nil {
			*firstErr = err
		}
		return 0
	}
	now := time.Now().Unix()
	seeded := 0
	for _, bn := range nodes {
		pk := bn.pubkey()
		if pk == "" {
			continue
		}
		ls := parseTS(bn.LastSeen)
		if ls == 0 {
			ls = now
		}
		if st.UpsertNode(store.Node{
			PubKey:    strings.ToLower(pk),
			Name:      bn.Name,
			Role:      bn.role(),
			Lat:       bn.Lat,
			Lon:       bn.Lon,
			HashSize:  bn.HashSize,
			FirstSeen: ls,
			LastSeen:  ls,
		}) == nil {
			seeded++
		}
	}
	return seeded
}

func fetchNodes(ctx context.Context, url string) ([]bootstrapNode, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: status %d", url, resp.StatusCode)
	}
	var body struct {
		Nodes     []bootstrapNode `json:"nodes"`
		Observers []bootstrapNode `json:"observers"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	if len(body.Observers) > 0 {
		return body.Observers, nil
	}
	return body.Nodes, nil
}
