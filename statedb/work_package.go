package statedb

import (
	"fmt"

	log "github.com/jam-duna/jamduna/log"
	"github.com/jam-duna/jamduna/types"
)

func (s *StateDB) GetAuthorizeCode(wp types.WorkPackage) (auth_code_real []byte, package_metadata []byte, host uint32, err error) {
	p_h := wp.AuthCodeHost
	p_u := wp.AuthorizationCodeHash
	log.Trace(log.G, "GetAuthorizeCode: start", "workPackage", wp.Hash(), "codeHost", p_h, "codeHash", p_u, "stateRoot", s.StateRoot)
	// author_sevice, _, err := s.getServiceAccount(p_h)
	// if err != nil {
	// 	return nil, 0, err
	// }
	// t := wp.RefineContext.LookupAnchorSlot

	// code := s.HistoricalLookup(author_sevice, t, p_u)

	code, ok, err := s.ReadServicePreimageBlob(p_h, p_u)
	if err != nil || !ok {
		log.Error(log.G, "GetAuthorizeCode: ReadServicePreimageBlob not found",
			"codeHost", p_h,
			"codeHash", p_u,
			"stateRoot", s.StateRoot,
			"ok", ok,
			"error", err)
		return nil, nil, 0, fmt.Errorf("getAuthorizeCode: auth code preimage not found in service %d, hash=%s, stateRoot=%s", p_h, p_u.Hex(), s.StateRoot.Hex())
	}
	auth_code := AuthorizeCode{}
	err = auth_code.Decode(code)
	if err != nil {
		log.Error(log.G, "GetAuthorizeCode: Authorization code decode error", "workPackage", wp.String(), "codeHost", p_h, "codeHash", p_u, "error", err)
		return nil, nil, 0, fmt.Errorf("getAuthorizeCode: Authorization code(%s) decode error, err: %v", p_u.String_short(), err)
	}
	if !ok || len(code) == 0 {
		log.Error(log.G, "GetAuthorizeCode: Authorization code not found", "workPackage", wp.String(), "codeHost", p_h, "codeHash", p_u, "ok", ok)
		return nil, nil, 0, fmt.Errorf("getAuthorizeCode: Authorization code(%s)not found, err: %v", p_u.String_short(), err)
	}
	// return auth_code.AuthorizationCode, auth_code.PackageMetaData, p_h, nil
	return code, auth_code.PackageMetaData, p_h, nil
}

func (s *StateDB) VerifyPackage(wpb types.WorkPackageBundle) error {
	err := wpb.Validate()
	if err != nil {
		return fmt.Errorf("VerifyPackage: %v", err)
	}
	wp := wpb.WorkPackage
	code, _, _, err := s.GetAuthorizeCode(wp)
	if err != nil {
		log.Error(log.G, "VerifyPackage: GetAuthorizeCode failed", "workPackage", wp, "error", err)
		return err
	}
	if len(code) == 0 {
		return fmt.Errorf("VerifyPackage: Authorization code is empty")
	}
	return nil
}
