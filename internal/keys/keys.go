package keys

var keyToKeycode = map[string]int{
	"a": 0x00, "s": 0x01, "d": 0x02, "f": 0x03, "h": 0x04,
	"g": 0x05, "z": 0x06, "x": 0x07, "c": 0x08, "v": 0x09,
	"b": 0x0B, "q": 0x0C, "w": 0x0D, "e": 0x0E, "r": 0x0F,
	"y": 0x10, "t": 0x11, "1": 0x12, "2": 0x13, "3": 0x14,
	"4": 0x15, "6": 0x16, "5": 0x17, "7": 0x1A, "8": 0x1C,
	"9": 0x19, "0": 0x1D, "p": 0x23, "o": 0x1F, "i": 0x22,
	"u": 0x20, "l": 0x25, "j": 0x26, "k": 0x28, "n": 0x2D,
	"m": 0x2E, "space": 0x31, "tab": 0x30, "return": 0x24,
	"escape": 0x35, "delete": 0x33,
	"f1": 0x7A, "f2": 0x78, "f3": 0x63, "f4": 0x76,
	"f5": 0x60, "f6": 0x61, "f7": 0x62, "f8": 0x64,
	"f9": 0x65, "f10": 0x6D, "f11": 0x67, "f12": 0x6F,
}

var keycodeToKey map[int]string

func init() {
	keycodeToKey = make(map[int]string, len(keyToKeycode))
	for key, code := range keyToKeycode {
		keycodeToKey[code] = key
	}
}

func IsKey(name string) bool {
	_, ok := keyToKeycode[name]
	return ok
}

func Keycode(name string) (int, bool) {
	code, ok := keyToKeycode[name]
	return code, ok
}

func KeyName(code int) (string, bool) {
	name, ok := keycodeToKey[code]
	return name, ok
}
