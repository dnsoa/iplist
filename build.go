package iplist

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Build creates a database file from the repository-style data directory.
//
// Expected inputs:
// - dataDir/country/*.txt (ISO 3166-1 alpha-2)
// - dataDir/cncity/*.txt (CN admin code, 6 digits)
// - dataDir/isp/*.txt (provider key)
func Build(dataDir, outPath string) error {
	providerNames := defaultProviderNames()
	cloudSet := defaultCloudSet()

	strIndex := newStringInterner()

	countryLabelIndex := make(map[string]uint32)
	cnLabelIndex := make(map[string]uint32)
	providerLabelIndex := make(map[string]uint32)

	countryLabels := make([]label2, 0, 260)
	cnLabels := make([]label2, 0, 500)
	providerLabels := make([]providerLabel, 0, 64)

	getCountryLabel := func(code string) uint32 {
		if idx, ok := countryLabelIndex[code]; ok {
			return idx
		}
		name, _ := docsCountryName(code)
		if name == "" {
			name = code
		}
		idx := uint32(len(countryLabels))
		countryLabelIndex[code] = idx
		countryLabels = append(countryLabels, label2{Code: strIndex.intern(code), Name: strIndex.intern(name)})
		return idx
	}
	getCNLabel := func(code string) uint32 {
		if idx, ok := cnLabelIndex[code]; ok {
			return idx
		}
		name, _ := docsCNCityName(code)
		if name == "" {
			name = code
		}
		idx := uint32(len(cnLabels))
		cnLabelIndex[code] = idx
		cnLabels = append(cnLabels, label2{Code: strIndex.intern(code), Name: strIndex.intern(name)})
		return idx
	}
	getProviderLabel := func(key string) uint32 {
		if idx, ok := providerLabelIndex[key]; ok {
			return idx
		}
		name := providerNames[key]
		if name == "" {
			name = key
		}
		kind := ProviderKindISP
		if cloudSet[key] {
			kind = ProviderKindCloud
		}
		idx := uint32(len(providerLabels))
		providerLabelIndex[key] = idx
		providerLabels = append(providerLabels, providerLabel{Key: strIndex.intern(key), Name: strIndex.intern(name), Kind: uint32(kind)})
		return idx
	}

	var countryEntries []entry
	{
		glob := filepath.Join(dataDir, "country", "*.txt")
		files, _ := filepath.Glob(glob)
		for _, p := range files {
			base := strings.TrimSuffix(filepath.Base(p), ".txt")
			if len(base) != 2 {
				continue
			}
			label := getCountryLabel(base)
			rs, err := readCIDRFileAsRanges(p)
			if err != nil {
				return err
			}
			for _, r := range rs {
				countryEntries = append(countryEntries, entry{Start: r.start, End: r.end, Label: label})
			}
		}
	}

	var cnProvEntries []entry
	var cnCityEntries []entry
	{
		glob := filepath.Join(dataDir, "cncity", "*.txt")
		files, _ := filepath.Glob(glob)
		for _, p := range files {
			code := strings.TrimSuffix(filepath.Base(p), ".txt")
			if len(code) != 6 {
				continue
			}
			label := getCNLabel(code)
			rs, err := readCIDRFileAsRanges(p)
			if err != nil {
				return err
			}
			isProv := strings.HasSuffix(code, "0000")
			isCity := strings.HasSuffix(code, "00") && !isProv
			if !isProv && !isCity {
				continue
			}
			for _, r := range rs {
				e := entry{Start: r.start, End: r.end, Label: label}
				if isCity {
					cnCityEntries = append(cnCityEntries, e)
				} else {
					cnProvEntries = append(cnProvEntries, e)
				}
			}
		}
	}

	var providerEntries []entry
	{
		glob := filepath.Join(dataDir, "isp", "*.txt")
		files, _ := filepath.Glob(glob)
		for _, p := range files {
			key := strings.TrimSuffix(filepath.Base(p), ".txt")
			label := getProviderLabel(key)
			rs, err := readCIDRFileAsRanges(p)
			if err != nil {
				return err
			}
			for _, r := range rs {
				providerEntries = append(providerEntries, entry{Start: r.start, End: r.end, Label: label})
			}
		}
	}

	// Sort and validate for lookup.
	sort.Slice(countryEntries, func(i, j int) bool { return countryEntries[i].Start < countryEntries[j].Start })
	sort.Slice(cnProvEntries, func(i, j int) bool { return cnProvEntries[i].Start < cnProvEntries[j].Start })
	sort.Slice(cnCityEntries, func(i, j int) bool { return cnCityEntries[i].Start < cnCityEntries[j].Start })
	sort.Slice(providerEntries, func(i, j int) bool { return providerEntries[i].Start < providerEntries[j].Start })

	if err := validateNoOverlapDifferentLabel(countryEntries); err != nil {
		return fmt.Errorf("country: %w", err)
	}
	if err := validateNoOverlapDifferentLabel(cnProvEntries); err != nil {
		return fmt.Errorf("cn province: %w", err)
	}
	if err := validateNoOverlapDifferentLabel(cnCityEntries); err != nil {
		return fmt.Errorf("cn city: %w", err)
	}
	// Provider ranges might overlap in future; we currently still enforce disjointness.
	if err := validateNoOverlapDifferentLabel(providerEntries); err != nil {
		return fmt.Errorf("provider: %w", err)
	}

	// Note: we intentionally do not densify ranges by default.
	// Densifying (filling gaps) can significantly increase the number of entries,
	// which hurts cache locality and makes binary search slower on this dataset.

	// Build strings blob.
	stringsBlob := strIndex.encode()

	buf := &bytes.Buffer{}
	buf.Grow(16 * 1024)

	// Reserve header.
	buf.Write(make([]byte, headerSize))

	stringsOff := uint32(buf.Len())
	buf.Write(stringsBlob)
	stringsSize := uint32(len(stringsBlob))

	// Align following fixed-width tables for safe unsafe.Slice on strict-alignment arches.
	// Header is 64 bytes; by aligning here we keep all later offsets aligned.
	if pad := (-buf.Len()) & 7; pad != 0 {
		buf.Write(make([]byte, pad))
	}

	// Section header (v2, 160 bytes)
	secOff := uint32(buf.Len())
	secSize := uint32(160)
	buf.Write(make([]byte, secSize))

	countryLabelsOff, countryLabelsCnt, err := writeFixed(buf, countryLabels)
	if err != nil {
		return err
	}
	cnLabelsOff, cnLabelsCnt, err := writeFixed(buf, cnLabels)
	if err != nil {
		return err
	}
	providerLabelsOff, providerLabelsCnt, err := writeFixed(buf, providerLabels)
	if err != nil {
		return err
	}
	countryStarts, countryEnds, countryLbls := splitEntries(countryEntries)
	cnProvStarts, cnProvEnds, cnProvLbls := splitEntries(cnProvEntries)
	cnCityStarts, cnCityEnds, cnCityLbls := splitEntries(cnCityEntries)
	providerStarts, providerEnds, providerLbls := splitEntries(providerEntries)

	countryStartsOff, countryEntriesCnt, err := writeU32Slice(buf, countryStarts)
	if err != nil {
		return err
	}
	countryEndsOff, _, err := writeU32Slice(buf, countryEnds)
	if err != nil {
		return err
	}
	countryLblsOff, _, err := writeU32Slice(buf, countryLbls)
	if err != nil {
		return err
	}

	cnProvStartsOff, cnProvEntriesCnt, err := writeU32Slice(buf, cnProvStarts)
	if err != nil {
		return err
	}
	cnProvEndsOff, _, err := writeU32Slice(buf, cnProvEnds)
	if err != nil {
		return err
	}
	cnProvLblsOff, _, err := writeU32Slice(buf, cnProvLbls)
	if err != nil {
		return err
	}

	cnCityStartsOff, cnCityEntriesCnt, err := writeU32Slice(buf, cnCityStarts)
	if err != nil {
		return err
	}
	cnCityEndsOff, _, err := writeU32Slice(buf, cnCityEnds)
	if err != nil {
		return err
	}
	cnCityLblsOff, _, err := writeU32Slice(buf, cnCityLbls)
	if err != nil {
		return err
	}

	providerStartsOff, providerEntriesCnt, err := writeU32Slice(buf, providerStarts)
	if err != nil {
		return err
	}
	providerEndsOff, _, err := writeU32Slice(buf, providerEnds)
	if err != nil {
		return err
	}
	providerLblsOff, _, err := writeU32Slice(buf, providerLbls)
	if err != nil {
		return err
	}

	// Fill header.
	out := buf.Bytes()
	copy(out[0:4], []byte(magicV4))
	binary.LittleEndian.PutUint16(out[4:6], version2)
	binary.LittleEndian.PutUint16(out[6:8], 0)
	binary.LittleEndian.PutUint64(out[8:16], uint64(time.Now().Unix()))
	binary.LittleEndian.PutUint32(out[16:20], stringsOff)
	binary.LittleEndian.PutUint32(out[20:24], stringsSize)
	binary.LittleEndian.PutUint32(out[24:28], secOff)
	binary.LittleEndian.PutUint32(out[28:32], secSize)
	// remaining header bytes reserved

	// Fill section header.
	sec := out[secOff : secOff+secSize]
	putU32 := func(off int, v uint32) { binary.LittleEndian.PutUint32(sec[off:off+4], v) }
	putU32(0, countryLabelsOff)
	putU32(4, countryLabelsCnt)
	putU32(8, cnLabelsOff)
	putU32(12, cnLabelsCnt)
	putU32(16, providerLabelsOff)
	putU32(20, providerLabelsCnt)

	// Each table: startsOff, endsOff, labelsOff, count
	putU32(24, countryStartsOff)
	putU32(28, countryEndsOff)
	putU32(32, countryLblsOff)
	putU32(36, countryEntriesCnt)

	putU32(40, cnProvStartsOff)
	putU32(44, cnProvEndsOff)
	putU32(48, cnProvLblsOff)
	putU32(52, cnProvEntriesCnt)

	putU32(56, cnCityStartsOff)
	putU32(60, cnCityEndsOff)
	putU32(64, cnCityLblsOff)
	putU32(68, cnCityEntriesCnt)

	putU32(72, providerStartsOff)
	putU32(76, providerEndsOff)
	putU32(80, providerLblsOff)
	putU32(84, providerEntriesCnt)

	if err := os.WriteFile(outPath, out, 0o644); err != nil {
		return err
	}
	return nil
}

func splitEntries(entries []entry) (starts, ends, labels []uint32) {
	starts = make([]uint32, len(entries))
	ends = make([]uint32, len(entries))
	labels = make([]uint32, len(entries))
	for i, e := range entries {
		starts[i] = e.Start
		ends[i] = e.End
		labels[i] = e.Label
	}
	return
}

func writeU32Slice(buf *bytes.Buffer, s []uint32) (off uint32, count uint32, err error) {
	off = uint32(buf.Len())
	count = uint32(len(s))
	if len(s) == 0 {
		return off, 0, nil
	}
	return off, count, binary.Write(buf, binary.LittleEndian, s)
}

func writeFixed[T any](buf *bytes.Buffer, s []T) (off uint32, count uint32, err error) {
	off = uint32(buf.Len())
	count = uint32(len(s))
	if len(s) == 0 {
		return off, 0, nil
	}
	return off, count, binary.Write(buf, binary.LittleEndian, s)
}

type ipRange struct{ start, end uint32 }

func readCIDRFileAsRanges(path string) ([]ipRange, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []ipRange
	r := bufio.NewScanner(f)
	for r.Scan() {
		line := strings.TrimSpace(r.Text())
		if line == "" {
			continue
		}
		_, n, err := net.ParseCIDR(line)
		if err != nil {
			return nil, fmt.Errorf("%s: parse CIDR %q: %w", path, line, err)
		}
		v4 := n.IP.To4()
		if v4 == nil {
			continue
		}
		maskSize, _ := n.Mask.Size()
		start := uint32(v4[0])<<24 | uint32(v4[1])<<16 | uint32(v4[2])<<8 | uint32(v4[3])
		size := uint32(1) << (32 - uint32(maskSize))
		end := start + size - 1
		out = append(out, ipRange{start: start, end: end})
	}
	if err := r.Err(); err != nil {
		return nil, err
	}

	// Normalize by merging overlaps/adjacent ranges.
	sort.Slice(out, func(i, j int) bool { return out[i].start < out[j].start })
	merged := out[:0]
	for _, cur := range out {
		if len(merged) == 0 {
			merged = append(merged, cur)
			continue
		}
		last := &merged[len(merged)-1]
		if cur.start <= last.end+1 {
			if cur.end > last.end {
				last.end = cur.end
			}
			continue
		}
		merged = append(merged, cur)
	}
	return merged, nil
}

func validateNoOverlapDifferentLabel(entries []entry) error {
	for i := 1; i < len(entries); i++ {
		prev := entries[i-1]
		cur := entries[i]
		if cur.Start <= prev.End {
			if cur.Label != prev.Label {
				return fmt.Errorf("overlap %d-%d (label %d) with %d-%d (label %d)", prev.Start, prev.End, prev.Label, cur.Start, cur.End, cur.Label)
			}
			// Same label overlap should have been merged by input merge; tolerate.
		}
	}
	return nil
}

// --- string interner ---

type stringInterner struct {
	idx map[string]uint32
	arr []string
}

func newStringInterner() *stringInterner {
	return &stringInterner{idx: make(map[string]uint32)}
}

func (s *stringInterner) intern(v string) uint32 {
	if i, ok := s.idx[v]; ok {
		return i
	}
	i := uint32(len(s.arr))
	s.idx[v] = i
	s.arr = append(s.arr, v)
	return i
}

func (s *stringInterner) encode() []byte {
	buf := &bytes.Buffer{}
	_ = binary.Write(buf, binary.LittleEndian, uint32(len(s.arr)))
	for _, str := range s.arr {
		b := []byte(str)
		_ = binary.Write(buf, binary.LittleEndian, uint16(len(b)))
		_, _ = io.Copy(buf, bytes.NewReader(b))
	}
	return buf.Bytes()
}
