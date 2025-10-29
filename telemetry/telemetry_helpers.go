package telemetry

import (
	"encoding/binary"
	"fmt"
	"net"

	"github.com/colorfulnotion/jam/common"
)

// Helper functions for parsing telemetry payloads

func parseUint64(payload []byte, offset int) (uint64, int) {
	if offset+8 > len(payload) {
		return 0, offset
	}
	value := binary.LittleEndian.Uint64(payload[offset : offset+8])
	return value, offset + 8
}

func parseUint32(payload []byte, offset int) (uint32, int) {
	if offset+4 > len(payload) {
		return 0, offset
	}
	value := binary.LittleEndian.Uint32(payload[offset : offset+4])
	return value, offset + 4
}

func parseUint16(payload []byte, offset int) (uint16, int) {
	if offset+2 > len(payload) {
		return 0, offset
	}
	value := binary.LittleEndian.Uint16(payload[offset : offset+2])
	return value, offset + 2
}

func parseUint8(payload []byte, offset int) (uint8, int) {
	if offset+1 > len(payload) {
		return 0, offset
	}
	return payload[offset], offset + 1
}

func parseHash(payload []byte, offset int) (string, int) {
	if offset+32 > len(payload) {
		return "0x0000000000000000000000000000000000000000000000000000000000000000", offset
	}
	hash := common.Hash{}
	copy(hash[:], payload[offset:offset+32])
	return hash.String(), offset + 32
}

func parsePeerID(payload []byte, offset int) (string, int) {
	if offset+32 > len(payload) {
		return "0x0000000000000000000000000000000000000000000000000000000000000000", offset
	}
	return fmt.Sprintf("0x%x", payload[offset:offset+32]), offset + 32
}

func parseAddress(payload []byte, offset int) (string, int) {
	if offset+18 > len(payload) {
		return "[::]:0", offset
	}

	ipBytes := payload[offset : offset+16]
	port := binary.LittleEndian.Uint16(payload[offset+16 : offset+18])

	ip := net.IP(ipBytes)
	return fmt.Sprintf("[%s]:%d", ip.String(), port), offset + 18
}

func parseString(payload []byte, offset int) (string, int) {
	if offset >= len(payload) {
		return "", offset
	}

	// Parse variable-length encoding for string length
	strLen, newOffset := parseVariableLength(payload, offset)
	if newOffset+int(strLen) > len(payload) {
		return "", offset
	}

	str := string(payload[newOffset : newOffset+int(strLen)])
	return str, newOffset + int(strLen)
}

func parseVariableLength(payload []byte, offset int) (uint64, int) {
	if offset >= len(payload) {
		return 0, offset
	}

	// Simple variable-length encoding: if first byte < 253, it's the length
	// Otherwise, use more complex encoding (simplified here)
	if payload[offset] < 253 {
		return uint64(payload[offset]), offset + 1
	}

	// For simplicity, assume it's a single byte length
	return uint64(payload[offset]), offset + 1
}

func formatBytes(data []byte) string {
	if len(data) == 0 {
		return "0x"
	}
	return fmt.Sprintf("0x%x", data)
}
