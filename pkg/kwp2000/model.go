package kwp2000

import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"strconv"

	"fyne.io/fyne/v2"
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

const (
	VAR_METHOD_ADDRESS Method = iota
	VAR_METHOD_LOCID
	VAR_METHOD_SYMBOL
)

type VarDefinition struct {
	data             []byte
	Name             string            `json:"name"`
	Method           Method            `json:"method"`
	Value            int               `json:"value"`
	Type             uint8             `json:"type"`
	Length           uint16            `json:"length"`
	Unit             string            `json:"unit,omitempty"`
	Correctionfactor float64           `json:"correctionfactor,omitempty"`
	Visualization    string            `json:"visualization,omitempty"`
	Group            string            `json:"group,omitempty"`
	Widget           fyne.CanvasObject `json:"-"`
}

func (v *VarDefinition) Set(data []byte) {
	v.data = data
}

func (v *VarDefinition) SetWidget(wb fyne.CanvasObject) {
	v.Widget = wb
}

func (v *VarDefinition) Read(r io.Reader) error {
	symbolData := make([]byte, v.Length)
	n, err := r.Read(symbolData)
	if err != nil {
		return fmt.Errorf("VarDefinition failed to Read: %w", err)
	}
	if n != int(v.Length) {
		return fmt.Errorf("VarDefinition expected %d bytes, got %d", v.Length, n)
	}
	//if bytes.Equal(symbolData, v.data) {
	//	v.Changed = true
	//}
	v.data = symbolData
	return nil
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

func (v *VarDefinition) GetUint64() uint64 {
	return binary.BigEndian.Uint64(v.data)
}

func (v *VarDefinition) GetInt64() int64 {
	return int64(binary.BigEndian.Uint64(v.data))
}

func (v *VarDefinition) GetFloat64() float64 {
	switch {
	case v.Length == 1:
		if len(v.data) != 1 {
			return -1
		}
		if v.Type&SIGNED != 0 {
			//if mock {
			//	return int8(r1.Int())
			//}
			return float64(v.GetInt8()) * v.Correctionfactor
		}
		//if mock {
		//	return uint8(r1.Int())
		//}
		return float64(v.GetUint8()) * v.Correctionfactor
	case v.Length == 2:
		if len(v.data) != 2 {
			return -1
		}
		if v.Type&SIGNED != 0 {
			//if mock {
			//	return int16(r1.Int())
			//}
			return float64(v.GetInt16()) * v.Correctionfactor
		}
		//if mock {
		//	return uint16(r1.Int())
		//}
		return float64(v.GetUint16()) * v.Correctionfactor
	case v.Length == 4:
		if len(v.data) != 4 {
			return -1
		}
		if v.Type&SIGNED != 0 {
			//if mock {
			//	return int32(r1.Uint32())
			//}
			return float64(v.GetInt32()) * v.Correctionfactor
		}
		//if mock {
		//	return uint32(r1.Uint32())
		//}
		return float64(v.GetUint32()) * v.Correctionfactor
	case v.Length == 8:
		if len(v.data) != 8 {
			return -1
		}
		if v.Type&SIGNED != 0 {
			//if mock {
			//	return int32(r1.Uint32())
			//}
			return float64(v.GetInt64()) * v.Correctionfactor
		}
		return float64(v.GetUint64()) * v.Correctionfactor
	default:
		return 0.0
	}

	/*

		switch t := v.Decode().(type) {
		case int8:
			return float64(t) * v.Correctionfactor
		case uint8:
			return float64(t) * v.Correctionfactor
		case int16:
			return float64(t) * v.Correctionfactor
		case uint16:
			return float64(t) * v.Correctionfactor
		case int32:
			return float64(t) * v.Correctionfactor
		case uint32:
			return float64(t) * v.Correctionfactor
		default:
			return 0
		}
	*/
}

func (v *VarDefinition) StringValue() string {
	if v.Correctionfactor != 1 {
		var result float64
		switch t := v.Decode().(type) {
		case int8:
			result = float64(t) * v.Correctionfactor
		case uint8:
			result = float64(t) * v.Correctionfactor
		case int16:
			result = float64(t) * v.Correctionfactor
		case uint16:
			result = float64(t) * v.Correctionfactor
		case int32:
			result = float64(t) * v.Correctionfactor
		case uint32:
			result = float64(t) * v.Correctionfactor
		}

		//var format string
		var precission int
		switch {
		case v.Correctionfactor == 0.1:
			precission = 1
		//	format = "%.1f"
		case v.Correctionfactor == 0.01:
			//	format = "%.2f"
			precission = 2
		case v.Correctionfactor == 0.001:
			//	format = "%.3f"
			precission = 3
		}
		return strconv.FormatFloat(result, 'f', precission, 64)
		//return fmt.Sprintf(format, roundFloat(result, precission))

	}
	return fmt.Sprintf("%v", v.Decode())
}

/*
func (v *VarDefinition) StringValue2() string {
	if v.Correctionfactor != "" {
		fs := token.NewFileSet()
		tv, err := types.Eval(fs, nil, token.NoPos, fmt.Sprintf("%v*%s", v.Decode(), v.Correctionfactor))
		if err != nil {
			panic(err)
		}
		return fmt.Sprintf("%v", tv.Value.String())
	}
	return fmt.Sprintf("%v", v.Decode())
}
*/

/*
func (v *VarDefinition) String() string {
	if v.Correctionfactor != "" {
		fs := token.NewFileSet()
		tv, err := types.Eval(fs, nil, token.NoPos, fmt.Sprintf("%v*%s", v.Decode(), v.Correctionfactor))
		if err != nil {
			panic(err)
		}
		return fmt.Sprintf("%s=%v%s", v.Name, strings.ReplaceAll(tv.Value.String(), ".", ","), v.Unit)
	}
	return fmt.Sprintf("%s=%v%s", v.Name, v.Decode(), v.Unit)
}
*/

/*
	func (v *VarDefinition) Tuple() string {
		if v.Correctionfactor != "" {
			fs := token.NewFileSet()
			tv, err := types.Eval(fs, nil, token.NoPos, fmt.Sprintf("%v*%s", v.Decode(), v.Correctionfactor))
			if err != nil {
				panic(err)
			}
			return fmt.Sprintf("%d:%v", v.Value, tv.Value.String())
		}
		return fmt.Sprintf("%d:%v", v.Value, v.Decode())
	}
*/

// var r1 = rand.New(rand.NewSource(time.Now().UnixNano()))
// var mock = true
type Number interface {
	int8 | uint8 | int16 | uint16 | uint32 | int32
}

func (v *VarDefinition) Decode() interface{} {
	switch {
	case v.Length == 1:
		if len(v.data) != 1 {
			return -1
		}
		if v.Type&SIGNED != 0 {
			//if mock {
			//	return int8(r1.Int())
			//}
			return v.GetInt8()
		}
		//if mock {
		//	return uint8(r1.Int())
		//}
		return v.GetUint8()
	case v.Length == 2:
		if len(v.data) != 2 {
			return -1
		}
		if v.Type&SIGNED != 0 {
			//if mock {
			//	return int16(r1.Int())
			//}
			return v.GetInt16()
		}
		//if mock {
		//	return uint16(r1.Int())
		//}
		return v.GetUint16()
	case v.Length == 4:
		if len(v.data) != 4 {
			return -1
		}
		if v.Type&SIGNED != 0 {
			//if mock {
			//	return int32(r1.Uint32())
			//}
			return v.GetInt32()
		}
		//if mock {
		//	return uint32(r1.Uint32())
		//}
		return v.GetUint32()
	case v.Length == 8:
		if len(v.data) != 8 {
			return -1
		}
		if v.Type&SIGNED != 0 {
			//if mock {
			//	return int32(r1.Uint32())
			//}
			return v.GetInt64()
		}
		return v.GetUint64()
	default:
		log.Println("unknown length", v.Length)
		return 0
	}
}
