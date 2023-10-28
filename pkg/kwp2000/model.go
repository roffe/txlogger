package kwp2000

var (
	INIT_MSG_ID        uint32 = 0x222
	REQ_MSG_ID         uint32 = 0x242
	INIT_RESP_ID       uint32 = 0x238
	REQ_CHUNK_CONF_ID  uint32 = 0x270
	RESP_CHUNK_CONF_ID uint32 = 0x266
)

const (
	SIGNED   = 0x01 /* signed flag in type */
	KONST    = 0x02 /* konstant flag in type */
	CHAR     = 0x04 /* character flag in type */
	LONG     = 0x08 /* long flag in type */
	BITFIELD = 0x10 /* bitfield flag in type */
	STRUCT   = 0x20 /* struct flag in type */
)

type Method int

const (
	VAR_METHOD_ADDRESS Method = iota
	VAR_METHOD_LOCID
	VAR_METHOD_SYMBOL
)

func (m Method) String() string {
	switch m {
	case VAR_METHOD_ADDRESS:
		return "Address"
	case VAR_METHOD_LOCID:
		return "Locid"
	case VAR_METHOD_SYMBOL:
		return "Symbol"
	}
	return "Unknown"
}
