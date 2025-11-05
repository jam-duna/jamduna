package common

// ObjectKind identifies the type of DA object
type ObjectKind uint8

const (
	// ObjectKindCode - bytecode storage (kind=0x00)
	ObjectKindCode ObjectKind = 0x00
	// ObjectKindStorageShard - storage shard object (kind=0x01)
	ObjectKindStorageShard ObjectKind = 0x01
	// ObjectKindSsrMetadata - SSR metadata object (kind=0x02)
	ObjectKindSsrMetadata ObjectKind = 0x02
	// ObjectKindReceipt - receipt object (kind=0x03)
	ObjectKindReceipt ObjectKind = 0x03
	// ObjectKindRaw - raw object (kind=0x04) - this is special in that its not in DA but in JAM State
	ObjectKindRaw   ObjectKind = 0x04
	ObjectKindBlock ObjectKind = 0x05
)

// Emoji returns the emoji representation for this object kind
func (k ObjectKind) Emoji() string {
	switch k {
	case ObjectKindCode:
		return "📜"
	case ObjectKindStorageShard:
		return "🗂️"
	case ObjectKindSsrMetadata:
		return "🗺️"
	case ObjectKindReceipt:
		return "🧾"
	case ObjectKindRaw:
		return "📤"
	default:
		return "❓"
	}
}

// FormatObjectIDWithEmoji formats an object ID as hex with emoji prefix based on byte 31
func FormatObjectIDWithEmoji(objectID Hash) string {
	kind := ObjectKind(objectID[31])
	return kind.Emoji() + " " + objectID.Hex()
}
