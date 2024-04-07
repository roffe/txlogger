package colors

import (
	"hash/crc32"
	"image/color"
)

var colorMap = map[string]color.RGBA{
	"m_Request":               {247, 10, 10, 255},
	"MAF.m_AirInlet":          {6, 245, 34, 255},
	"Lambda.LambdaInt":        {247, 127, 10, 255},
	"IgnProt.fi_Offset":       {247, 21, 223, 255},
	"ActualIn.T_AirInlet":     {26, 160, 253, 255},
	"Out.fi_Ignition":         {244, 251, 18, 255},
	"Out.PWM_BoostCntrl":      {64, 216, 140, 255},
	"DisplProt.LambdaScanner": {105, 20, 253, 255},
	"ActualIn.p_AirInlet":     {8, 126, 2, 255},
	"In.p_AirBefThrottle":     {153, 1, 1, 255},
}

func GetColor(name string) color.RGBA {
	if c, ok := colorMap[name]; ok {
		return c
	}
	return hashToRGB(name)
}

func hashToRGB(input string) color.RGBA {
	// Calculate CRC32 hash
	hash := crc32.ChecksumIEEE([]byte(input))
	// Map the hash value to RGB color space
	return color.RGBA{byte(hash >> 8), byte(hash >> 16), byte(hash), 255}
}
