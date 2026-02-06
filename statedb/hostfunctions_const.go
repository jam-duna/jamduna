package statedb

import "github.com/jam-duna/jamduna/pvm/pvmtypes"

const (
	GAS               = pvmtypes.GAS
	FETCH             = pvmtypes.FETCH
	LOOKUP            = pvmtypes.LOOKUP
	READ              = pvmtypes.READ
	WRITE             = pvmtypes.WRITE
	INFO              = pvmtypes.INFO
	HISTORICAL_LOOKUP = pvmtypes.HISTORICAL_LOOKUP
	EXPORT            = pvmtypes.EXPORT
	MACHINE           = pvmtypes.MACHINE
	PEEK              = pvmtypes.PEEK
	POKE              = pvmtypes.POKE
	PAGES             = pvmtypes.PAGES
	INVOKE            = pvmtypes.INVOKE
	EXPUNGE           = pvmtypes.EXPUNGE
	BLESS             = pvmtypes.BLESS
	ASSIGN            = pvmtypes.ASSIGN
	DESIGNATE         = pvmtypes.DESIGNATE
	CHECKPOINT        = pvmtypes.CHECKPOINT
	NEW               = pvmtypes.NEW
	UPGRADE           = pvmtypes.UPGRADE
	TRANSFER          = pvmtypes.TRANSFER
	EJECT             = pvmtypes.EJECT
	QUERY             = pvmtypes.QUERY
	SOLICIT           = pvmtypes.SOLICIT
	FORGET            = pvmtypes.FORGET
	YIELD             = pvmtypes.YIELD
	PROVIDE           = pvmtypes.PROVIDE
	LOG               = pvmtypes.LOG
	FETCH_WITNESS     = pvmtypes.FETCH_WITNESS
	FETCH_UBT         = pvmtypes.FETCH_UBT
)
