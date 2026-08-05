// Command gen builds the embedded ASN tables from APNIC's published view of
// the global routing table. See the package doc of internal/asn.
//
//	curl -O https://thyme.apnic.net/current/data-raw-table
//	curl -O https://thyme.apnic.net/current/data-used-autnums
//	go run ./internal/asn/gen
//
// Reads those two files from the working directory and writes
// internal/asn/asn-ranges.bin.gz and internal/asn/asn-names.tsv.gz.
package main

import (
	"bufio"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
)

type ipRange struct {
	start, end uint32
	asn        uint32
}

func main() {
	ranges, err := readTable("data-raw-table")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("prefixes: %d\n", len(ranges))

	sort.Slice(ranges, func(i, j int) bool { return ranges[i].start < ranges[j].start })
	merged := merge(ranges)
	fmt.Printf("after merging contiguous same-AS ranges: %d\n", len(merged))

	used := map[uint32]bool{}
	for _, r := range merged {
		used[r.asn] = true
	}
	names, err := readNames("data-used-autnums", used)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("AS names kept: %d of %d announced\n", len(names), len(used))

	if err := writeRanges("internal/asn/asn-ranges.bin.gz", merged); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := writeNames("internal/asn/asn-names.tsv.gz", names); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("wrote internal/asn/asn-ranges.bin.gz and asn-names.tsv.gz")
}

func readTable(path string) ([]ipRange, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []ipRange
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) != 2 {
			continue
		}
		_, cidr, err := net.ParseCIDR(fields[0])
		if err != nil || cidr.IP.To4() == nil {
			continue
		}
		asn, err := strconv.ParseUint(fields[1], 10, 32)
		if err != nil {
			continue
		}
		start := binary.BigEndian.Uint32(cidr.IP.To4())
		ones, bits := cidr.Mask.Size()
		end := start + (1<<uint(bits-ones) - 1)
		out = append(out, ipRange{start: start, end: end, asn: uint32(asn)})
	}
	return out, sc.Err()
}

// merge collapses ranges that are contiguous and announced by the same AS.
// This is what makes the table embeddable: 1.07M prefixes become ~370k.
func merge(sorted []ipRange) []ipRange {
	var out []ipRange
	for _, r := range sorted {
		if n := len(out); n > 0 && out[n-1].asn == r.asn && r.start <= out[n-1].end+1 {
			if r.end > out[n-1].end {
				out[n-1].end = r.end
			}
			continue
		}
		out = append(out, r)
	}
	return out
}

func readNames(path string, used map[uint32]bool) (map[uint32][2]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	out := map[uint32][2]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		num, desc, ok := strings.Cut(line, " ")
		if !ok {
			continue
		}
		asn, err := strconv.ParseUint(num, 10, 32)
		if err != nil || !used[uint32(asn)] {
			continue
		}
		desc = strings.TrimSpace(desc)

		// A trailing ", CC" is the AS registration country.
		country := ""
		if i := strings.LastIndex(desc, ", "); i >= 0 && len(desc)-i == 4 {
			country, desc = desc[i+2:], desc[:i]
		}
		// "SHORTNAME - Long Company Name": the long name is the useful half.
		if _, long, ok := strings.Cut(desc, " - "); ok {
			desc = long
		}
		if len(desc) > 60 {
			desc = desc[:60]
		}
		out[uint32(asn)] = [2]string{strings.TrimSpace(desc), country}
	}
	return out, sc.Err()
}

func writeRanges(path string, ranges []ipRange) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	zw, _ := gzip.NewWriterLevel(f, gzip.BestCompression)
	buf := make([]byte, 12)
	for _, r := range ranges {
		binary.BigEndian.PutUint32(buf[0:], r.start)
		binary.BigEndian.PutUint32(buf[4:], r.end)
		binary.BigEndian.PutUint32(buf[8:], r.asn)
		if _, err := zw.Write(buf); err != nil {
			return err
		}
	}
	return zw.Close()
}

func writeNames(path string, names map[uint32][2]string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	asns := make([]int, 0, len(names))
	for a := range names {
		asns = append(asns, int(a))
	}
	sort.Ints(asns)

	zw, _ := gzip.NewWriterLevel(f, gzip.BestCompression)
	for _, a := range asns {
		n := names[uint32(a)]
		if _, err := fmt.Fprintf(zw, "%d\t%s\t%s\n", a, n[0], n[1]); err != nil {
			return err
		}
	}
	return zw.Close()
}
