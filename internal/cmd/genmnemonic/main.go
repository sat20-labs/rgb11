// Command genmnemonic translates the frozen mnemonic 1.1.1 Rust word table
// into a Go source file. It is a development-time provenance tool only.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/format"
	"os"
)

func main() {
	source := flag.String("source", "", "mnemonic 1.1.1 src/lib.rs")
	output := flag.String("output", "", "generated Go output")
	flag.Parse()
	if *source == "" || *output == "" {
		panic("-source and -output are required")
	}
	raw, err := os.ReadFile(*source)
	if err != nil {
		panic(err)
	}
	start := bytes.Index(raw, []byte("pub static MN_WORDS"))
	if start < 0 {
		panic("MN_WORDS table not found")
	}
	declaration := raw[start:]
	tableStart := bytes.Index(declaration, []byte("= ["))
	if tableStart < 0 {
		panic("MN_WORDS table start not found")
	}
	table := declaration[tableStart+3:]
	tableEnd := bytes.Index(table, []byte("];"))
	if tableEnd < 0 {
		panic("MN_WORDS table end not found")
	}
	table = table[:tableEnd]
	words := make([][]byte, 0, 1633)
	for {
		open := bytes.Index(table, []byte(`b"`))
		if open < 0 {
			break
		}
		table = table[open+2:]
		close := bytes.IndexByte(table, '"')
		if close < 0 {
			panic("unterminated mnemonic word")
		}
		words = append(words, append([]byte{}, table[:close]...))
		table = table[close+1:]
	}
	if len(words) != 1633 {
		panic(fmt.Sprintf("unexpected word count %d", len(words)))
	}
	var out bytes.Buffer
	out.WriteString("// Code generated from mnemonic 1.1.1; DO NOT EDIT.\n")
	out.WriteString("// Upstream crate checksum: f2b8f3a258db515d5e91a904ce4ae3f73e091149b90cadbdb93d210bee07f63b\n")
	out.WriteString("package baid64\n\nvar mnemonicWords = [...]string{\n")
	for _, word := range words {
		fmt.Fprintf(&out, "\t%q,\n", word)
	}
	out.WriteString("}\n")
	formatted, err := format.Source(out.Bytes())
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(*output, formatted, 0o644); err != nil {
		panic(err)
	}
}
