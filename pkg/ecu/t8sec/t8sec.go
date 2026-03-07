package t8sec

func CalculateAccessKey(seed []byte, level byte) (byte, byte) {
	val := int(seed[0])<<8 | int(seed[1])

	key := func(seed int) int {
		key := seed>>5 | seed<<11
		return (key + 0xB988) & 0xFFFF
	}(val)

	switch level {
	case 0xFB:
		key ^= 0x8749
		key += 0x06D3
		key ^= 0xCFDF
	case 0xFD:
		key /= 3
		key ^= 0x8749
		key += 0x0ACF
		key ^= 0x81BF
	}

	return (byte)((key >> 8) & 0xFF), (byte)(key & 0xFF)
}

func CalculateKeyForCIM(a_seed []byte, level byte) (byte, byte) {
	seed := int(a_seed[0])<<8 | int(a_seed[1])
	returnKey := make([]byte, 2)

	key := (seed + 0x9130) & 0xFFFF
	key = ((key >> 8) | (key << 8)) & 0xFFFF
	key = (0x3FC7 - key) & 0xFFFF

	returnKey[0] = byte((key >> 8) & 0xFF)
	returnKey[1] = byte(key & 0xFF)

	return (byte)((key >> 8) & 0xFF), (byte)(key & 0xFF)
}
