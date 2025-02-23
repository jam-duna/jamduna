package statedb

import (
	"fmt"

	"github.com/colorfulnotion/jam/types"
)

func (s *StateDB) GetAuthorizeCode(wp types.WorkPackage) ([]byte, uint32, error) {
	p_h := wp.AuthCodeHost
	p_u := wp.AuthorizationCodeHash
	// author_sevice, _, err := s.getServiceAccount(p_h)
	// if err != nil {
	// 	return nil, 0, err
	// }
	// t := wp.RefineContext.LookupAnchorSlot

	// code := s.HistoricalLookup(author_sevice, t, p_u)

	code, _, err := s.ReadServicePreimageBlob(p_h, p_u)
	if code == nil || len(code) == 0 || err != nil {
		return nil, 0, fmt.Errorf("getAuthorizeCode: Authorization code(%s)not found, err: %v", p_u.String_short(), err)
	}
	return code, p_h, nil
}
