// Package keys defines the shared logical key names that are allowed in config.
//
// These names are platform-neutral at the config layer. Platform-specific
// keycode tables stay in the platform implementations.
package keys

import "slices"

var validKeys = map[string]bool{
	"a": true, "s": true, "d": true, "f": true, "h": true,
	"g": true, "z": true, "x": true, "c": true, "v": true,
	"b": true, "q": true, "w": true, "e": true, "r": true,
	"y": true, "t": true, "1": true, "2": true, "3": true,
	"4": true, "6": true, "5": true, "7": true, "8": true,
	"9": true, "0": true, "p": true, "o": true, "i": true,
	"u": true, "l": true, "j": true, "k": true, "n": true,
	"m": true, "space": true, "tab": true, "return": true,
	"escape": true, "delete": true,
	"f1": true, "f2": true, "f3": true, "f4": true,
	"f5": true, "f6": true, "f7": true, "f8": true,
	"f9": true, "f10": true, "f11": true, "f12": true,
}

func IsKey(name string) bool {
	return validKeys[name]
}

func Names() []string {
	names := make([]string, 0, len(validKeys))
	for name := range validKeys {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}
