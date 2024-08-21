package types

import (
	"github.com/colorfulnotion/jam/common"
	//"encoding/json"
)

// TicketBody represents the structure of a ticket
type TicketBody struct {
	Id      common.Hash `json:"id"`
	Attempt uint8       `json:"attempt"`
}
