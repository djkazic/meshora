package store

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const schema = `
PRAGMA journal_mode=WAL;
PRAGMA synchronous=NORMAL;
PRAGMA foreign_keys=ON;
PRAGMA busy_timeout=5000;

CREATE TABLE IF NOT EXISTS nodes (
  pubkey       TEXT PRIMARY KEY,
  name         TEXT NOT NULL DEFAULT '',
  role         TEXT NOT NULL DEFAULT '',
  lat          REAL,
  lon          REAL,
  first_seen   INTEGER NOT NULL,
  last_seen    INTEGER NOT NULL,
  advert_count INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_nodes_last_seen ON nodes(last_seen);

CREATE TABLE IF NOT EXISTS transmissions (
  id                INTEGER PRIMARY KEY AUTOINCREMENT,
  hash              TEXT NOT NULL UNIQUE,
  raw_hex           TEXT NOT NULL,
  route_type        INTEGER NOT NULL,
  payload_type      INTEGER NOT NULL,
  origin_pubkey     TEXT,
  path_json         TEXT,
  resolved_path     TEXT,
  first_seen        INTEGER NOT NULL,
  last_seen         INTEGER NOT NULL,
  observation_count INTEGER NOT NULL DEFAULT 1
);
CREATE INDEX IF NOT EXISTS idx_tx_first_seen ON transmissions(first_seen);

CREATE TABLE IF NOT EXISTS observations (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  transmission_id INTEGER NOT NULL REFERENCES transmissions(id) ON DELETE CASCADE,
  observer_id     TEXT,
  observer_name   TEXT,
  snr             REAL,
  rssi            REAL,
  ts              INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_obs_tx ON observations(transmission_id);

CREATE TABLE IF NOT EXISTS flows (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  hash         TEXT NOT NULL,
  payload_type INTEGER NOT NULL,
  payload_name TEXT NOT NULL,
  route_type   INTEGER NOT NULL,
  snr          REAL,
  waypoints    TEXT NOT NULL,
  ts           INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_flows_ts ON flows(ts);
`

type Store struct {
	db *sql.DB
}

type Node struct {
	PubKey      string   `json:"pubkey"`
	Name        string   `json:"name"`
	Role        string   `json:"role"`
	Lat         *float64 `json:"lat"`
	Lon         *float64 `json:"lon"`
	HashSize    *int     `json:"hash_size"`
	FirstSeen   int64    `json:"first_seen"`
	LastSeen    int64    `json:"last_seen"`
	AdvertCount int      `json:"advert_count"`
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	for _, m := range []string{
		`ALTER TABLE transmissions ADD COLUMN channel TEXT`,
		`ALTER TABLE transmissions ADD COLUMN msg_text TEXT`,
		`ALTER TABLE transmissions ADD COLUMN path_hash_size INTEGER`,
		`ALTER TABLE nodes ADD COLUMN hash_size INTEGER`,
	} {
		_, _ = db.Exec(m)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) UpsertNode(n Node) error {
	_, err := s.db.Exec(`
INSERT INTO nodes (pubkey, name, role, lat, lon, hash_size, first_seen, last_seen, advert_count)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1)
ON CONFLICT(pubkey) DO UPDATE SET
  name         = CASE WHEN excluded.name != '' THEN excluded.name ELSE nodes.name END,
  role         = CASE WHEN excluded.role != '' THEN excluded.role ELSE nodes.role END,
  lat          = COALESCE(excluded.lat, nodes.lat),
  lon          = COALESCE(excluded.lon, nodes.lon),
  hash_size    = COALESCE(excluded.hash_size, nodes.hash_size),
  last_seen    = MAX(nodes.last_seen, excluded.last_seen),
  advert_count = nodes.advert_count + 1
`, n.PubKey, n.Name, n.Role, n.Lat, n.Lon, n.HashSize, n.FirstSeen, n.LastSeen)
	return err
}

func (s *Store) RecordTransmission(t Transmission, o Observation) (isNew bool, err error) {
	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`
INSERT INTO transmissions
  (hash, raw_hex, route_type, payload_type, origin_pubkey, path_json, resolved_path, channel, msg_text, path_hash_size, first_seen, last_seen, observation_count)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1)
ON CONFLICT(hash) DO UPDATE SET
  observation_count = transmissions.observation_count + 1,
  last_seen         = MAX(transmissions.last_seen, excluded.last_seen)
`, t.Hash, t.RawHex, t.RouteType, t.PayloadType, t.OriginPubKey, t.PathJSON, t.ResolvedPath, t.Channel, t.MsgText, t.PathHashSize, t.Ts, t.Ts); err != nil {
		return false, err
	}
	var obsCount int
	var txID int64
	if err := tx.QueryRow(`SELECT id, observation_count FROM transmissions WHERE hash = ?`, t.Hash).Scan(&txID, &obsCount); err != nil {
		return false, err
	}
	isNew = obsCount == 1

	if _, err := tx.Exec(`
INSERT INTO observations (transmission_id, observer_id, observer_name, snr, rssi, ts)
VALUES (?, ?, ?, ?, ?, ?)
`, txID, o.ObserverID, o.ObserverName, o.SNR, o.RSSI, o.Ts); err != nil {
		return false, err
	}
	return isNew, tx.Commit()
}

type Transmission struct {
	Hash         string
	RawHex       string
	RouteType    int
	PayloadType  int
	OriginPubKey *string
	PathJSON     string
	ResolvedPath string
	Channel      string
	MsgText      string
	PathHashSize int
	Ts           int64
}

type Observation struct {
	ObserverID   string
	ObserverName string
	SNR          *float64
	RSSI         *float64
	Ts           int64
}

type FlowRow struct {
	Hash        string   `json:"hash"`
	PayloadType int      `json:"payload_type"`
	PayloadName string   `json:"payload_name"`
	RouteType   int      `json:"route_type"`
	SNR         *float64 `json:"snr"`
	Waypoints   string   `json:"-"`
	Ts          int64    `json:"ts"`
}

func (s *Store) InsertFlow(f FlowRow) error {
	_, err := s.db.Exec(`
INSERT INTO flows (hash, payload_type, payload_name, route_type, snr, waypoints, ts)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
		f.Hash, f.PayloadType, f.PayloadName, f.RouteType, f.SNR, f.Waypoints, f.Ts)
	return err
}

type PacketSummary struct {
	Hash             string
	PayloadType      int
	RouteType        int
	FirstSeen        int64
	ObservationCount int
	Hops             int
	OriginPubKey     *string
	Channel          *string
	MsgText          *string
}

func (s *Store) RecentPackets(limit, payloadType int, nodePubkeys []string) ([]PacketSummary, error) {
	q := `SELECT hash, payload_type, route_type, first_seen, observation_count, COALESCE(path_json,''), origin_pubkey, channel, msg_text
FROM transmissions`
	var conds []string
	var args []any
	if payloadType >= 0 {
		conds = append(conds, "payload_type = ?")
		args = append(args, payloadType)
	}
	if len(nodePubkeys) > 0 {
		var ors []string
		for _, pk := range nodePubkeys {
			ors = append(ors, "lower(resolved_path) LIKE ?")
			args = append(args, "%"+strings.ToLower(pk)+"%")
		}
		conds = append(conds, "("+strings.Join(ors, " OR ")+")")
	}
	if len(conds) > 0 {
		q += " WHERE " + strings.Join(conds, " AND ")
	}
	q += " ORDER BY first_seen DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PacketSummary
	for rows.Next() {
		var p PacketSummary
		var path string
		if err := rows.Scan(&p.Hash, &p.PayloadType, &p.RouteType, &p.FirstSeen, &p.ObservationCount, &path, &p.OriginPubKey, &p.Channel, &p.MsgText); err != nil {
			return nil, err
		}
		if path != "" {
			p.Hops = strings.Count(path, ",") + 1
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) RecentFlows(sinceTs int64, limit int) ([]FlowRow, error) {
	rows, err := s.db.Query(`
SELECT hash, payload_type, payload_name, route_type, snr, waypoints, ts
FROM flows WHERE ts >= ? ORDER BY ts ASC LIMIT ?`, sinceTs, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FlowRow
	for rows.Next() {
		var f FlowRow
		if err := rows.Scan(&f.Hash, &f.PayloadType, &f.PayloadName, &f.RouteType, &f.SNR, &f.Waypoints, &f.Ts); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

type PacketObs struct {
	ObserverID   string   `json:"observer_id"`
	ObserverName string   `json:"observer_name"`
	SNR          *float64 `json:"snr"`
	RSSI         *float64 `json:"rssi"`
	Ts           int64    `json:"ts"`
}

type Packet struct {
	Hash             string
	RawHex           string
	RouteType        int
	PayloadType      int
	PathJSON         string
	ResolvedPath     string
	OriginPubKey     *string
	Channel          *string
	MsgText          *string
	FirstSeen        int64
	LastSeen         int64
	ObservationCount int
	Observations     []PacketObs
}

func (s *Store) GetPacket(hash string) (*Packet, error) {
	var p Packet
	var txID int64
	err := s.db.QueryRow(`
SELECT id, hash, raw_hex, route_type, payload_type, path_json, resolved_path,
       origin_pubkey, channel, msg_text, first_seen, last_seen, observation_count
FROM transmissions WHERE hash = ?`, hash).Scan(
		&txID, &p.Hash, &p.RawHex, &p.RouteType, &p.PayloadType, &p.PathJSON,
		&p.ResolvedPath, &p.OriginPubKey, &p.Channel, &p.MsgText, &p.FirstSeen, &p.LastSeen, &p.ObservationCount)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	rows, err := s.db.Query(`
SELECT COALESCE(observer_id,''), COALESCE(observer_name,''), snr, rssi, ts
FROM observations WHERE transmission_id = ? ORDER BY ts ASC`, txID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var o PacketObs
		if err := rows.Scan(&o.ObserverID, &o.ObserverName, &o.SNR, &o.RSSI, &o.Ts); err != nil {
			return nil, err
		}
		p.Observations = append(p.Observations, o)
	}
	return &p, rows.Err()
}

type NodeDetail struct {
	Node
	HeardAsObserver int `json:"heard_as_observer"`
	Originated      int `json:"originated"`
	OnPath1h        int `json:"on_path_1h"`
	OnPath24h       int `json:"on_path_24h"`
}

func (s *Store) GetNode(pubkey string) (*NodeDetail, error) {
	var d NodeDetail
	err := s.db.QueryRow(`
SELECT pubkey, name, role, lat, lon, hash_size, first_seen, last_seen, advert_count
FROM nodes WHERE lower(pubkey) = lower(?)`, pubkey).Scan(
		&d.PubKey, &d.Name, &d.Role, &d.Lat, &d.Lon, &d.HashSize, &d.FirstSeen, &d.LastSeen, &d.AdvertCount)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := s.db.QueryRow(`SELECT count(*) FROM observations WHERE lower(observer_id) = lower(?)`, pubkey).Scan(&d.HeardAsObserver); err != nil {
		return nil, err
	}
	if err := s.db.QueryRow(`SELECT count(*) FROM transmissions WHERE lower(origin_pubkey) = lower(?)`, pubkey).Scan(&d.Originated); err != nil {
		return nil, err
	}

	if d.Role == "repeater" {
		now := time.Now().Unix()
		like := "%" + strings.ToLower(d.PubKey) + "%"
		if err := s.db.QueryRow(`SELECT count(*) FROM transmissions WHERE first_seen >= ? AND lower(resolved_path) LIKE ?`, now-3600, like).Scan(&d.OnPath1h); err != nil {
			return nil, err
		}
		if err := s.db.QueryRow(`SELECT count(*) FROM transmissions WHERE first_seen >= ? AND lower(resolved_path) LIKE ?`, now-86400, like).Scan(&d.OnPath24h); err != nil {
			return nil, err
		}
	}
	return &d, nil
}

type RawPathGroup struct {
	ResolvedPath string
	Count        int
	LastSeen     int64
}

func (s *Store) PathsThroughNode(pubkey string, limit int) ([]RawPathGroup, error) {
	rows, err := s.db.Query(`
SELECT resolved_path, count(*), MAX(first_seen)
FROM transmissions
WHERE resolved_path != '' AND lower(resolved_path) LIKE ?
GROUP BY resolved_path
ORDER BY count(*) DESC, MAX(first_seen) DESC
LIMIT ?`, "%"+strings.ToLower(pubkey)+"%", limit)
	if err != nil {
		return nil, err
	}
	return scanPathGroups(rows)
}

func (s *Store) TopRoutes(limit int) ([]RawPathGroup, error) {
	rows, err := s.db.Query(`
SELECT resolved_path, count(*), MAX(first_seen)
FROM transmissions
WHERE resolved_path != ''
GROUP BY resolved_path
ORDER BY count(*) DESC, MAX(first_seen) DESC
LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	return scanPathGroups(rows)
}

func scanPathGroups(rows *sql.Rows) ([]RawPathGroup, error) {
	defer rows.Close()
	var out []RawPathGroup
	for rows.Next() {
		var g RawPathGroup
		if err := rows.Scan(&g.ResolvedPath, &g.Count, &g.LastSeen); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

type NodeInfo struct {
	Name string `json:"name"`
	Role string `json:"role"`
}

func (s *Store) NodePubkeysMatching(query string, limit int) ([]string, error) {
	like := "%" + strings.ToLower(query) + "%"
	rows, err := s.db.Query(`SELECT pubkey FROM nodes WHERE lower(name) LIKE ? OR lower(pubkey) LIKE ? LIMIT ?`, like, like, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var pk string
		if err := rows.Scan(&pk); err != nil {
			return nil, err
		}
		out = append(out, pk)
	}
	return out, rows.Err()
}

func (s *Store) NodeInfos(pubkeys []string) (map[string]NodeInfo, error) {
	out := make(map[string]NodeInfo)
	for _, pk := range pubkeys {
		if pk == "" {
			continue
		}
		var info NodeInfo
		err := s.db.QueryRow(`SELECT name, role FROM nodes WHERE lower(pubkey) = lower(?)`, pk).Scan(&info.Name, &info.Role)
		if err == nil {
			out[pk] = info
		}
	}
	return out, nil
}

func (s *Store) NodesPositioned(nowTs, maxAgeSecs int64) ([]Node, error) {
	q := `
SELECT pubkey, name, role, lat, lon, hash_size, first_seen, last_seen, advert_count
FROM nodes
WHERE lat IS NOT NULL AND lon IS NOT NULL`
	var args []any
	if maxAgeSecs > 0 {
		q += ` AND last_seen >= ?`
		args = append(args, nowTs-maxAgeSecs)
	}
	q += ` ORDER BY last_seen DESC`
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Node
	for rows.Next() {
		var n Node
		if err := rows.Scan(&n.PubKey, &n.Name, &n.Role, &n.Lat, &n.Lon, &n.HashSize, &n.FirstSeen, &n.LastSeen, &n.AdvertCount); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *Store) NodePositions() (map[string][2]float64, error) {
	rows, err := s.db.Query(`SELECT pubkey, lat, lon FROM nodes WHERE lat IS NOT NULL AND lon IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := make(map[string][2]float64)
	for rows.Next() {
		var pk string
		var lat, lon float64
		if err := rows.Scan(&pk, &lat, &lon); err != nil {
			return nil, err
		}
		m[pk] = [2]float64{lat, lon}
	}
	return m, rows.Err()
}

type Stats struct {
	Nodes              int   `json:"nodes"`
	NodesWithPos       int   `json:"nodes_with_pos"`
	Transmissions      int   `json:"transmissions"`
	Observations       int   `json:"observations"`
	LastTransmissionTs int64 `json:"last_transmission_ts"`
}

func (s *Store) Stats() (Stats, error) {
	var st Stats
	row := s.db.QueryRow(`
SELECT
  (SELECT COUNT(*) FROM nodes),
  (SELECT COUNT(*) FROM nodes WHERE lat IS NOT NULL),
  (SELECT COUNT(*) FROM transmissions),
  (SELECT COUNT(*) FROM observations),
  (SELECT COALESCE(MAX(last_seen), 0) FROM transmissions)`)
	err := row.Scan(&st.Nodes, &st.NodesWithPos, &st.Transmissions, &st.Observations, &st.LastTransmissionTs)
	return st, err
}

type HashSizeCount struct {
	Size  int `json:"size"`
	Count int `json:"count"`
}

type PathHashStats struct {
	Packets   []HashSizeCount `json:"packets"`
	Repeaters []HashSizeCount `json:"repeaters"`
}

func (s *Store) PathHashStats() (PathHashStats, error) {
	packets, err := s.sizeCounts(`SELECT path_hash_size, COUNT(*) FROM transmissions WHERE path_hash_size IN (1, 2, 3) GROUP BY path_hash_size`)
	if err != nil {
		return PathHashStats{}, err
	}
	repeaters, err := s.sizeCounts(`SELECT hash_size, COUNT(*) FROM nodes WHERE role = 'repeater' AND hash_size IN (1, 2, 3) GROUP BY hash_size`)
	if err != nil {
		return PathHashStats{}, err
	}
	return PathHashStats{Packets: spread(packets), Repeaters: spread(repeaters)}, nil
}

func (s *Store) sizeCounts(query string) (map[int]int, error) {
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[int]int{}
	for rows.Next() {
		var size, n int
		if err := rows.Scan(&size, &n); err != nil {
			return nil, err
		}
		m[size] = n
	}
	return m, rows.Err()
}

func spread(m map[int]int) []HashSizeCount {
	out := make([]HashSizeCount, 0, 3)
	for size := 1; size <= 3; size++ {
		out = append(out, HashSizeCount{Size: size, Count: m[size]})
	}
	return out
}

func (s *Store) BackfillPathHashSizes(decode func(rawHex string) (int, bool)) (int, error) {
	rows, err := s.db.Query(`SELECT id, raw_hex FROM transmissions WHERE path_hash_size IS NULL`)
	if err != nil {
		return 0, err
	}
	type pending struct {
		id  int64
		raw string
	}
	var todo []pending
	for rows.Next() {
		var p pending
		if err := rows.Scan(&p.id, &p.raw); err != nil {
			rows.Close()
			return 0, err
		}
		todo = append(todo, p)
	}
	rows.Close()
	if len(todo) == 0 {
		return 0, nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`UPDATE transmissions SET path_hash_size = ? WHERE id = ?`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()
	filled := 0
	for _, p := range todo {
		size, ok := decode(p.raw)
		if !ok {
			continue
		}
		if _, err := stmt.Exec(size, p.id); err != nil {
			return filled, err
		}
		filled++
	}
	return filled, tx.Commit()
}

type HopBucket struct {
	Hops  int `json:"hops"`
	Count int `json:"count"`
}

func (s *Store) HopDistribution() ([]HopBucket, error) {
	rows, err := s.db.Query(`
SELECT CASE WHEN path_json IS NULL OR path_json = '' THEN 0
            ELSE length(path_json) - length(replace(path_json, ',', '')) + 1 END AS hops,
       COUNT(*)
FROM transmissions
GROUP BY hops
ORDER BY hops`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []HopBucket{}
	for rows.Next() {
		var b HopBucket
		if err := rows.Scan(&b.Hops, &b.Count); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

type CentralityRow struct {
	PubKey     string  `json:"pubkey"`
	Name       string  `json:"name"`
	PathCount  int     `json:"path_count"`
	Percentile float64 `json:"percentile"`
}

func (s *Store) RepeaterCentrality(limit int) ([]CentralityRow, error) {
	names := map[string]string{}
	score := map[string]int{}
	rows, err := s.db.Query(`SELECT lower(pubkey), name FROM nodes WHERE role = 'repeater'`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var pk, name string
		if err := rows.Scan(&pk, &name); err != nil {
			rows.Close()
			return nil, err
		}
		names[pk] = name
		score[pk] = 0
	}
	rows.Close()
	if len(score) == 0 {
		return []CentralityRow{}, nil
	}

	paths, err := s.db.Query(`SELECT resolved_path FROM transmissions WHERE resolved_path != ''`)
	if err != nil {
		return nil, err
	}
	for paths.Next() {
		var rp string
		if err := paths.Scan(&rp); err != nil {
			paths.Close()
			return nil, err
		}
		seen := map[string]bool{}
		for _, pk := range strings.Split(rp, ",") {
			pk = strings.ToLower(pk)
			if pk == "" || seen[pk] {
				continue
			}
			seen[pk] = true
			if _, ok := score[pk]; ok {
				score[pk]++
			}
		}
	}
	if err := paths.Err(); err != nil {
		paths.Close()
		return nil, err
	}
	paths.Close()

	type rec struct {
		pk    string
		count int
	}
	ranked := make([]rec, 0, len(score))
	freq := map[int]int{}
	for pk, n := range score {
		ranked = append(ranked, rec{pk, n})
		freq[n]++
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].count != ranked[j].count {
			return ranked[i].count > ranked[j].count
		}
		return ranked[i].pk < ranked[j].pk
	})

	below := map[int]int{}
	scores := make([]int, 0, len(freq))
	for sc := range freq {
		scores = append(scores, sc)
	}
	sort.Ints(scores)
	cum := 0
	for _, sc := range scores {
		below[sc] = cum
		cum += freq[sc]
	}
	total := float64(len(ranked))

	if limit > len(ranked) {
		limit = len(ranked)
	}
	out := make([]CentralityRow, 0, limit)
	for _, r := range ranked[:limit] {
		out = append(out, CentralityRow{
			PubKey:     r.pk,
			Name:       names[r.pk],
			PathCount:  r.count,
			Percentile: float64(below[r.count]) / total * 100,
		})
	}
	return out, nil
}

type NetworkEdge struct {
	A      string
	B      string
	Weight int
}

func (s *Store) edgeWeights() (map[string]int, error) {
	rows, err := s.db.Query(`SELECT resolved_path FROM transmissions WHERE resolved_path != ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	weights := map[string]int{}
	for rows.Next() {
		var rp string
		if err := rows.Scan(&rp); err != nil {
			return nil, err
		}
		var hops []string
		for _, pk := range strings.Split(rp, ",") {
			if pk = strings.ToLower(pk); pk != "" {
				hops = append(hops, pk)
			}
		}
		for i := 0; i+1 < len(hops); i++ {
			a, b := hops[i], hops[i+1]
			if a == b {
				continue
			}
			if a > b {
				a, b = b, a
			}
			weights[a+"|"+b]++
		}
	}
	return weights, rows.Err()
}

func (s *Store) NetworkEdges(limit int) ([]NetworkEdge, error) {
	weights, err := s.edgeWeights()
	if err != nil {
		return nil, err
	}
	out := make([]NetworkEdge, 0, len(weights))
	for k, w := range weights {
		sep := strings.IndexByte(k, '|')
		out = append(out, NetworkEdge{A: k[:sep], B: k[sep+1:], Weight: w})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Weight != out[j].Weight {
			return out[i].Weight > out[j].Weight
		}
		if out[i].A != out[j].A {
			return out[i].A < out[j].A
		}
		return out[i].B < out[j].B
	})
	if limit < len(out) {
		out = out[:limit]
	}
	return out, nil
}

type CriticalNode struct {
	PubKey    string
	Degree    int
	Fragments int
	Isolated  int
}

func (s *Store) CriticalNodes(limit int) ([]CriticalNode, error) {
	weights, err := s.edgeWeights()
	if err != nil {
		return nil, err
	}

	id := map[string]int{}
	var pubkeys []string
	var adj [][]int
	node := func(pk string) int {
		if i, ok := id[pk]; ok {
			return i
		}
		i := len(adj)
		id[pk] = i
		pubkeys = append(pubkeys, pk)
		adj = append(adj, nil)
		return i
	}
	for k := range weights {
		sep := strings.IndexByte(k, '|')
		a := node(k[:sep])
		b := node(k[sep+1:])
		adj[a] = append(adj[a], b)
		adj[b] = append(adj[b], a)
	}
	if len(adj) == 0 {
		return []CriticalNode{}, nil
	}

	baseCount, baseLargest := components(adj, -1)

	type rec struct {
		idx, fragments, isolated, degree int
	}
	var cuts []rec
	for v := range adj {
		if len(adj[v]) == 0 {
			continue
		}
		count, largest := components(adj, v)
		fragments := count - (baseCount - 1)
		if fragments < 2 {
			continue
		}
		isolated := baseLargest - largest - 1
		if isolated < 0 {
			isolated = 0
		}
		cuts = append(cuts, rec{v, fragments, isolated, len(adj[v])})
	}
	sort.Slice(cuts, func(i, j int) bool {
		if cuts[i].isolated != cuts[j].isolated {
			return cuts[i].isolated > cuts[j].isolated
		}
		if cuts[i].fragments != cuts[j].fragments {
			return cuts[i].fragments > cuts[j].fragments
		}
		return cuts[i].degree > cuts[j].degree
	})
	if limit < len(cuts) {
		cuts = cuts[:limit]
	}

	out := make([]CriticalNode, 0, len(cuts))
	for _, c := range cuts {
		out = append(out, CriticalNode{
			PubKey:    pubkeys[c.idx],
			Degree:    c.degree,
			Fragments: c.fragments,
			Isolated:  c.isolated,
		})
	}
	return out, nil
}

func components(adj [][]int, skip int) (count int, largest int) {
	seen := make([]bool, len(adj))
	for i := range adj {
		if i == skip || seen[i] {
			continue
		}
		size := 0
		stack := []int{i}
		seen[i] = true
		for len(stack) > 0 {
			x := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			size++
			for _, y := range adj[x] {
				if y == skip || seen[y] {
					continue
				}
				seen[y] = true
				stack = append(stack, y)
			}
		}
		count++
		if size > largest {
			largest = size
		}
	}
	return count, largest
}
