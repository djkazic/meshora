package meshcore

import (
	"crypto/sha256"
	"encoding/hex"
)

func ContentHash(rawHex string) string {
	buf, err := hex.DecodeString(rawHex)
	if err != nil || len(buf) < 2 {
		return fallbackHash(rawHex)
	}

	hdr := buf[0]
	offset := 1
	if isTransportRoute(int(hdr & 0x03)) {
		offset += 4
	}
	if offset >= len(buf) {
		return fallbackHash(rawHex)
	}
	pathByte := buf[offset]
	offset++
	payloadStart := offset + pathHashSize(pathByte)*pathHashCount(pathByte)
	if payloadStart > len(buf) {
		return fallbackHash(rawHex)
	}
	payload := buf[payloadStart:]

	payloadType := (hdr >> 2) & 0x0F
	toHash := []byte{payloadType}
	if int(payloadType) == PayloadTRACE {
		toHash = append(toHash, pathByte, 0x00)
	}
	toHash = append(toHash, payload...)

	sum := sha256.Sum256(toHash)
	return hex.EncodeToString(sum[:])[:16]
}

func fallbackHash(rawHex string) string {
	if len(rawHex) >= 16 {
		return rawHex[:16]
	}
	return rawHex
}
