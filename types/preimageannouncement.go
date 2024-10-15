package types

import (
	"github.com/colorfulnotion/jam/common"
)

type PreimageAnnouncement struct {
	ValidatorIndex uint16
	PreimageHash   common.Hash
}
