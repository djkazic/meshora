package meshcore

import (
	"bufio"
	"encoding/json"
	"math"
	"os"
	"strings"
	"testing"
)

const nashobaRaw = "1143F625873BB27C4EA581DE7323C6B58E9A818D42D2649D4E731C5F0478B4A8FA45C6843D928A693BC8356AC1F23FB5094E284EB47B2046C07930B76B38D00EA923341EC2F65A9203C15A9EEE1C71375AFF28DEC9AF69B42D300F6293B3ED5B8534012B626EC0E1C96D720192812A8802D9D1BEFB434F4E2D4E6173686F626142726F6F6B73"

func TestDecodeAdvertGolden(t *testing.T) {
	p, err := Decode(nashobaRaw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if p.PayloadType != PayloadADVERT {
		t.Errorf("payload type = %d (%s), want ADVERT", p.PayloadType, p.PayloadTypeName)
	}
	if p.RouteType != RouteFlood {
		t.Errorf("route type = %d (%s), want FLOOD", p.RouteType, p.RouteTypeName)
	}
	wantHops := []string{"F625", "873B", "B27C"}
	if strings.Join(p.Hops, ",") != strings.Join(wantHops, ",") {
		t.Errorf("hops = %v, want %v", p.Hops, wantHops)
	}
	if p.Advert == nil {
		t.Fatal("advert is nil")
	}
	a := p.Advert
	if len(a.PubKey) != 64 || !strings.HasPrefix(a.PubKey, "4ea581de7323c6b5") {
		t.Errorf("pubkey = %q", a.PubKey)
	}
	if a.Name != "CON-NashobaBrooks" {
		t.Errorf("name = %q", a.Name)
	}
	if a.Role != "repeater" {
		t.Errorf("role = %q, want repeater", a.Role)
	}
	if a.Lat == nil || a.Lon == nil {
		t.Fatal("expected location")
	}
	if math.Abs(*a.Lat-42.47821) > 1e-5 || math.Abs(*a.Lon-(-71.38052)) > 1e-5 {
		t.Errorf("coords = %.5f,%.5f want 42.47821,-71.38052", *a.Lat, *a.Lon)
	}
}

func TestRejectsMalformed(t *testing.T) {
	for _, raw := range []string{"", "00", "zz", "11"} {
		if _, err := Decode(raw); err == nil {
			t.Errorf("expected error for %q", raw)
		}
	}
}

type corpusEntry struct {
	RawHex      string `json:"raw_hex"`
	PayloadType int    `json:"payload_type"`
	Hash        string `json:"hash"`
}

func loadCorpus(t *testing.T) []corpusEntry {
	t.Helper()
	f, err := os.Open("../../testdata/sample_feed.jsonl")
	if err != nil {
		t.Skipf("no corpus: %v", err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	var out []corpusEntry
	for sc.Scan() {
		var e corpusEntry
		if json.Unmarshal(sc.Bytes(), &e) == nil && e.RawHex != "" {
			out = append(out, e)
		}
	}
	return out
}

func TestDecodeCorpus(t *testing.T) {
	corpus := loadCorpus(t)
	if len(corpus) == 0 {
		t.Skip("empty corpus")
	}
	for i, e := range corpus {
		p, err := Decode(e.RawHex)
		if err != nil {
			t.Errorf("entry %d (pt=%d): decode error: %v", i, e.PayloadType, err)
			continue
		}
		if p.PayloadType != e.PayloadType {
			t.Errorf("entry %d: payload type ours=%d feed=%d", i, p.PayloadType, e.PayloadType)
		}
	}
	t.Logf("decoded %d real packets cleanly", len(corpus))
}

func TestContentHashMatchesFeed(t *testing.T) {
	corpus := loadCorpus(t)
	if len(corpus) == 0 {
		t.Skip("empty corpus")
	}
	checked, mismatched := 0, 0
	for _, e := range corpus {
		if e.Hash == "" {
			continue
		}
		checked++
		if got := ContentHash(e.RawHex); got != e.Hash {
			mismatched++
			if mismatched <= 5 {
				t.Errorf("hash mismatch: ours=%s feed=%s raw=%s", got, e.Hash, e.RawHex)
			}
		}
	}
	t.Logf("content hash: %d checked, %d mismatched", checked, mismatched)
}
