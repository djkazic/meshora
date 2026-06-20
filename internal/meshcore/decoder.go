package meshcore

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"
)

const (
	RouteTransportFlood  = 0
	RouteFlood           = 1
	RouteDirect          = 2
	RouteTransportDirect = 3
)

const (
	PayloadREQ       = 0x00
	PayloadRESPONSE  = 0x01
	PayloadTXTMsg    = 0x02
	PayloadACK       = 0x03
	PayloadADVERT    = 0x04
	PayloadGRPTxt    = 0x05
	PayloadGRPData   = 0x06
	PayloadANONReq   = 0x07
	PayloadPATH      = 0x08
	PayloadTRACE     = 0x09
	PayloadMULTIPART = 0x0A
	PayloadCONTROL   = 0x0B
	PayloadRAWCustom = 0x0F
)

var routeTypeNames = map[int]string{
	0: "TRANSPORT_FLOOD", 1: "FLOOD", 2: "DIRECT", 3: "TRANSPORT_DIRECT",
}

var payloadTypeNames = map[int]string{
	0x00: "REQ", 0x01: "RESPONSE", 0x02: "TXT_MSG", 0x03: "ACK", 0x04: "ADVERT",
	0x05: "GRP_TXT", 0x06: "GRP_DATA", 0x07: "ANON_REQ", 0x08: "PATH", 0x09: "TRACE",
	0x0A: "MULTIPART", 0x0B: "CONTROL", 0x0F: "RAW_CUSTOM",
}

const (
	maxPathSize      = 64
	maxPacketPayload = 184
)

type Packet struct {
	RouteType       int
	RouteTypeName   string
	PayloadType     int
	PayloadTypeName string
	PayloadVersion  int

	Hops         []string
	PathHashSize int

	Advert *Advert
}

type Advert struct {
	PubKey      string
	Name        string
	Role        string
	Timestamp   uint32
	Lat         *float64
	Lon         *float64
	HasLocation bool
}

func RouteName(rt int) string {
	if n, ok := routeTypeNames[rt]; ok {
		return n
	}
	return "UNKNOWN"
}

func PayloadName(pt int) string {
	if n, ok := payloadTypeNames[pt]; ok {
		return n
	}
	return "UNKNOWN"
}

func isTransportRoute(rt int) bool {
	return rt == RouteTransportFlood || rt == RouteTransportDirect
}

func pathHashSize(pathByte byte) int  { return int(pathByte>>6) + 1 }
func pathHashCount(pathByte byte) int { return int(pathByte & 0x3F) }

func isValidPathLen(pathByte byte) bool {
	hashSize := pathHashSize(pathByte)
	if hashSize == 4 {
		return false
	}
	return pathHashCount(pathByte)*hashSize <= maxPathSize
}

func Decode(rawHex string) (*Packet, error) {
	rawHex = strings.NewReplacer(" ", "", "\n", "", "\r", "").Replace(rawHex)
	buf, err := hex.DecodeString(rawHex)
	if err != nil {
		return nil, fmt.Errorf("invalid hex: %w", err)
	}
	if len(buf) < 2 {
		return nil, fmt.Errorf("packet too short")
	}

	hdr := buf[0]
	rt := int(hdr & 0x03)
	pt := int((hdr >> 2) & 0x0F)
	pv := int((hdr >> 6) & 0x03)

	offset := 1
	if isTransportRoute(rt) {
		if len(buf) < offset+4 {
			return nil, fmt.Errorf("too short for transport codes")
		}
		offset += 4
	}

	if offset >= len(buf) {
		return nil, fmt.Errorf("missing path byte")
	}
	pathByte := buf[offset]
	offset++

	if !isValidPathLen(pathByte) {
		return nil, fmt.Errorf("invalid path byte 0x%02X", pathByte)
	}
	hashSize := pathHashSize(pathByte)
	hashCount := pathHashCount(pathByte)

	hops := make([]string, 0, hashCount)
	for i := 0; i < hashCount; i++ {
		start := offset + i*hashSize
		end := start + hashSize
		if end > len(buf) {
			break
		}
		hops = append(hops, strings.ToUpper(hex.EncodeToString(buf[start:end])))
	}
	offset += hashSize * hashCount
	if offset > len(buf) {
		return nil, fmt.Errorf("path length exceeds buffer")
	}

	payload := buf[offset:]
	if len(payload) > maxPacketPayload {
		return nil, fmt.Errorf("payload too large (%d bytes)", len(payload))
	}

	p := &Packet{
		RouteType:       rt,
		RouteTypeName:   RouteName(rt),
		PayloadType:     pt,
		PayloadTypeName: PayloadName(pt),
		PayloadVersion:  pv,
		Hops:            hops,
		PathHashSize:    hashSize,
	}

	if pt == PayloadADVERT {
		p.Advert = decodeAdvert(payload)
	}
	return p, nil
}

func PathHashSizeOf(rawHex string) (int, bool) {
	p, err := Decode(rawHex)
	if err != nil {
		return 0, false
	}
	return p.PathHashSize, true
}

func decodeAdvert(buf []byte) *Advert {
	if len(buf) < 100 {
		return nil
	}
	a := &Advert{
		PubKey:    hex.EncodeToString(buf[0:32]),
		Timestamp: binary.LittleEndian.Uint32(buf[32:36]),
	}
	appdata := buf[100:]
	if len(appdata) == 0 {
		a.Role = "none"
		return a
	}

	flags := appdata[0]
	advType := int(flags & 0x0F)
	hasLocation := flags&0x10 != 0
	hasFeat1 := flags&0x20 != 0
	hasFeat2 := flags&0x40 != 0
	hasName := flags&0x80 != 0
	a.Role = roleForType(advType)
	a.HasLocation = hasLocation

	off := 1
	if hasLocation && len(appdata) >= off+8 {
		latRaw := int32(binary.LittleEndian.Uint32(appdata[off : off+4]))
		lonRaw := int32(binary.LittleEndian.Uint32(appdata[off+4 : off+8]))
		lat := float64(latRaw) / 1e6
		lon := float64(lonRaw) / 1e6
		if lat >= -90 && lat <= 90 && lon >= -180 && lon <= 180 && !(lat == 0 && lon == 0) {
			a.Lat = &lat
			a.Lon = &lon
		}
		off += 8
	}
	if hasFeat1 && len(appdata) >= off+2 {
		off += 2
	}
	if hasFeat2 && len(appdata) >= off+2 {
		off += 2
	}
	if hasName {
		nameEnd := len(appdata)
		for i := off; i < len(appdata); i++ {
			if appdata[i] == 0x00 {
				nameEnd = i
				break
			}
		}
		name := sanitizeName(string(appdata[off:nameEnd]))
		if len(name) > 32 {
			name = name[:32]
		}
		a.Name = name
	}
	return a
}

func roleForType(t int) string {
	switch t {
	case 0:
		return "none"
	case 1:
		return "companion"
	case 2:
		return "repeater"
	case 3:
		return "room"
	case 4:
		return "sensor"
	default:
		return fmt.Sprintf("type-%d", t)
	}
}

func sanitizeName(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, c := range s {
		if c == '\t' || c == '\n' || (c >= 0x20 && c != 0x7f) {
			b.WriteRune(c)
		}
	}
	return strings.TrimSpace(b.String())
}
