// Package oui resolves MAC address prefixes to manufacturer names using an
// embedded copy of the IEEE MA-L registry (oui.tsv.gz, generated from
// https://standards-oui.ieee.org/oui/oui.csv, fetched 2026-07-18).
package oui

import (
	"bufio"
	"compress/gzip"
	"bytes"
	_ "embed"
	"strings"
	"sync"
)

//go:embed oui.tsv.gz
var ouiData []byte

var (
	once  sync.Once
	table map[string]string
)

func load() {
	table = make(map[string]string, 40000)
	zr, err := gzip.NewReader(bytes.NewReader(ouiData))
	if err != nil {
		return
	}
	sc := bufio.NewScanner(zr)
	for sc.Scan() {
		prefix, name, ok := strings.Cut(sc.Text(), "\t")
		if ok {
			table[prefix] = name
		}
	}
}

// Lookup returns the manufacturer for a MAC address like "a0:f8:49:74:8b:20",
// or "" if unknown. Locally-administered addresses (randomized MACs) return "".
func Lookup(mac string) string {
	once.Do(load)
	hex := strings.ToUpper(strings.NewReplacer(":", "", "-", "", ".", "").Replace(mac))
	if len(hex) < 6 {
		return ""
	}
	// Locally-administered bit set = randomized/virtual MAC, not in the registry
	if hex[1] == '2' || hex[1] == '6' || hex[1] == 'A' || hex[1] == 'E' {
		return ""
	}
	return table[hex[:6]]
}
