package kwp2000

import (
	"encoding/binary"
	"fmt"
	"go/token"
	"go/types"
	"io"
	"log"
	"strings"
)

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

/*
type Type int

func (t Type) String() string {
	_, file, no, ok := runtime.Caller(1)
	if ok {
		fmt.Printf("called from %s#%d\n", file, no)
	}

	log.Printf("Type: %X", uint8(t))

	if uint8(t)&0x01 != 0 {
		log.Println("Signed")
	}
	if uint8(t)&0x02 != 0 {
		log.Println("Const")
	}
	if uint8(t)&0x04 != 0 {
		log.Println("Char")
	}
	if uint8(t)&0x08 != 0 {
		log.Println("Long")
	}
	if uint8(t)&0x10 != 0 {
		log.Println("Bitfield")
	}
	if uint8(t)&0x20 != 0 {
		log.Println("Struct")
	}


		switch t {
		case TYPE_WORD:
			return "Word"
		case TYPE_LONG:
			return "Long"
		case TYPE_BYTE:
			return "Byte"
		case TYPE_UWORD:
			return "UWord"
		}

	return strconv.Itoa(int(t))
}
*/

const (
	VAR_METHOD_ADDRESS Method = iota
	VAR_METHOD_LOCID
	VAR_METHOD_SYMBOL
)

/*
const (
	TYPE_WORD  Type = 21
	TYPE_BYTE  Type = 24
	TYPE_LONG  Type = 69
	TYPE_UWORD Type = 32
)
*/

type VarDefinition struct {
	data             []byte
	Name             string `json:"name"`
	Method           Method `json:"method"`
	Value            int    `json:"value"`
	Type             uint8  `json:"type"`
	Length           uint16 `json:"length"`
	Unit             string `json:"unit,omitempty"`
	Correctionfactor string `json:"formula,omitempty"`
}

/*
func (v *VarDefinition) Length() byte {
	switch v.Type {
	case TYPE_WORD, TYPE_UWORD:
		return 2
	case TYPE_LONG:
		return 4
	case TYPE_BYTE:
		return 1
	}
	return 0
}
*/

func (v *VarDefinition) Set(data []byte) {
	v.data = data
}

func (v *VarDefinition) GetBool() bool {
	return v.data[0] == 1
}

func (v *VarDefinition) GetUint8() uint8 {
	return uint8(v.data[0])
}

func (v *VarDefinition) GetInt8() int8 {
	return int8(v.data[0])
}

func (v *VarDefinition) GetUint16() uint16 {
	return binary.BigEndian.Uint16(v.data)
}

func (v *VarDefinition) GetInt16() int16 {
	return int16(binary.BigEndian.Uint16(v.data))
}

func (v *VarDefinition) GetUint32() uint32 {
	return binary.BigEndian.Uint32(v.data)
}

func (v *VarDefinition) GetInt32() int32 {
	return int32(binary.BigEndian.Uint32(v.data))
}

func (v *VarDefinition) String() string {
	if v.Correctionfactor != "" {
		fs := token.NewFileSet()
		tv, err := types.Eval(fs, nil, token.NoPos, fmt.Sprintf("%v*%v", v.decode(), v.Correctionfactor))
		if err != nil {
			panic(err)
		}
		return fmt.Sprintf("%s=%v%s", v.Name, strings.ReplaceAll(tv.Value.String(), ".", ","), v.Unit)
	}

	return fmt.Sprintf("%s=%v%s", v.Name, v.decode(), v.Unit)
}

func (v *VarDefinition) T7L() string {
	if v.Correctionfactor != "" {
		fs := token.NewFileSet()
		tv, err := types.Eval(fs, nil, token.NoPos, fmt.Sprintf("%v*%v", v.Correctionfactor, v.decode()))
		if err != nil {
			panic(err)
		}
		return fmt.Sprintf("%s=%v", v.Name, strings.ReplaceAll(tv.Value.String(), ".", ","))
	}

	return fmt.Sprintf("%s=%v", v.Name, v.decode())
}

func (v *VarDefinition) decode() interface{} {
	switch {
	case v.Length == 1:
		if len(v.data) != 1 {
			return -1
		}
		if v.Type&SIGNED != 0 {
			return int8(v.data[0])
		}
		return float64(uint8(v.data[0]))
	case v.Length == 2:
		if len(v.data) != 2 {
			return -1
		}
		if v.Type&SIGNED != 0 {
			return int16(binary.BigEndian.Uint16(v.data))
		}
		return binary.BigEndian.Uint16(v.data)
	case v.Length == 4:
		if len(v.data) != 4 {
			return -1
		}
		if v.Type&SIGNED != 0 {
			return int32(binary.BigEndian.Uint32(v.data))
		}
		return binary.BigEndian.Uint32(v.data)
	default:
		log.Println("unknown length", v.Length)
		return 0
	}
}

func (v *VarDefinition) Read(r io.Reader) error {
	symbolData := make([]byte, v.Length)
	n, err := r.Read(symbolData)
	if err != nil {
		return fmt.Errorf("VarDefinition failed to Read: %w", err)
	}
	if n != int(v.Length) {
		return fmt.Errorf("expected %d bytes, got %d", v.Length, n)
	}
	v.data = symbolData
	return nil
}
