// gen-ico creates a multi-resolution Windows .ico from a set of PNG files.
// PNG data is embedded directly (Vista+ "PNG ICO" format).
//
// Usage: go run ./scripts/gen-ico -dir assets/logo -out assets/windows/joicetyper.ico
package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	dir := flag.String("dir", "assets/logo", "directory containing logo-N.png files")
	out := flag.String("out", "assets/windows/joicetyper.ico", "output .ico path")
	flag.Parse()

	sizes := []int{16, 32, 64, 128, 256}

	type entry struct {
		size int
		data []byte
	}
	var entries []entry
	for _, sz := range sizes {
		path := filepath.Join(*dir, fmt.Sprintf("joicetyper-logo-%d.png", sz))
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "skip %dpx: %v\n", sz, err)
			continue
		}
		entries = append(entries, entry{sz, data})
	}
	if len(entries) == 0 {
		fmt.Fprintln(os.Stderr, "no PNG files found — nothing to do")
		os.Exit(1)
	}

	f, err := os.Create(*out)
	if err != nil {
		fmt.Fprintln(os.Stderr, "create:", err)
		os.Exit(1)
	}
	defer f.Close()

	// ICO header
	write16(f, 0) // reserved
	write16(f, 1) // type: icon
	write16(f, uint16(len(entries)))

	// ICONDIRENTRY array starts at byte 6; each entry is 16 bytes
	dataOffset := uint32(6 + len(entries)*16)
	for _, e := range entries {
		w, h := uint8(e.size), uint8(e.size)
		if e.size >= 256 {
			w, h = 0, 0 // 0 means 256 in ICO spec
		}
		write8(f, w)
		write8(f, h)
		write8(f, 0)              // color count
		write8(f, 0)              // reserved
		write16(f, 1)             // planes
		write16(f, 32)            // bit depth
		write32(f, uint32(len(e.data)))
		write32(f, dataOffset)
		dataOffset += uint32(len(e.data))
	}

	for _, e := range entries {
		f.Write(e.data)
	}

	fmt.Printf("wrote %s (%d sizes: ", *out, len(entries))
	for i, e := range entries {
		if i > 0 {
			fmt.Print(", ")
		}
		fmt.Print(e.size)
	}
	fmt.Println(")")
}

func write8(f *os.File, v uint8)   { f.Write([]byte{v}) }
func write16(f *os.File, v uint16) { binary.Write(f, binary.LittleEndian, v) }
func write32(f *os.File, v uint32) { binary.Write(f, binary.LittleEndian, v) }
