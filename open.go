package iplist

import (
	"encoding/binary"
	"os"
	"syscall"
	"unsafe"
)

var (
	nativeLittleEndian bool
)

func init() {
	// Database file is encoded in little-endian. Most Go targets are little-endian.
	// On big-endian targets we byte-swap fixed-width tables once on open.
	var i uint32 = 0x01020304
	nativeLittleEndian = *(*byte)(unsafe.Pointer(&i)) == 0x04
}

type v4Table struct {
	starts     []uint32
	ends       []uint32
	labels     []uint32
	dense      bool
	bucketLo16 []uint32 // len 65536; first i with ends[i]   >= (p<<16)
	bucketHi16 []uint32 // len 65536; first i with starts[i] >= ((p+1)<<16)
}

func (t *v4Table) detectDense() {
	const maxU32 = ^uint32(0)
	if len(t.starts) == 0 {
		t.dense = false
		return
	}
	if len(t.starts) != len(t.ends) || len(t.starts) != len(t.labels) {
		t.dense = false
		return
	}
	if t.starts[0] != 0 {
		t.dense = false
		return
	}
	for i := 0; i < len(t.starts)-1; i++ {
		e := t.ends[i]
		if e == maxU32 {
			t.dense = false
			return
		}
		if e+1 != t.starts[i+1] {
			t.dense = false
			return
		}
	}
	t.dense = t.ends[len(t.ends)-1] == maxU32
}

func (t *v4Table) buildBuckets16() {
	if len(t.starts) == 0 {
		t.bucketLo16 = nil
		t.bucketHi16 = nil
		return
	}
	const size = 1 << 16
	startGE := make([]uint32, size+1)
	endGE := make([]uint32, size+1)

	// Build startGE: first index with start >= prefixStart.
	{
		i := 0
		n := len(t.starts)
		for p := 0; p <= size; p++ {
			key := uint32(p) << 16
			for i < n && t.starts[i] < key {
				i++
			}
			startGE[p] = uint32(i)
		}
	}
	// Build endGE: first index with end >= prefixStart.
	{
		i := 0
		n := len(t.ends)
		for p := 0; p <= size; p++ {
			key := uint32(p) << 16
			for i < n && t.ends[i] < key {
				i++
			}
			endGE[p] = uint32(i)
		}
	}

	lo := make([]uint32, size)
	hi := make([]uint32, size)
	for p := 0; p < size; p++ {
		lo[p] = endGE[p]
		hi[p] = startGE[p+1]
	}
	// Keep only the compact bucket boundaries.
	t.bucketLo16 = lo
	t.bucketHi16 = hi
}

type v4DB struct {
	stringsData  []byte
	stringsStart []uint32
	stringsEnd   []uint32

	countryLabels  []label2
	cnLabels       []label2
	providerLabels []providerLabel

	country  v4Table
	cnProv   v4Table
	cnCity   v4Table
	provider v4Table

	providerByKey     map[string]uint32
	providerKindByKey map[string]ProviderKind
}

func open(path string) (*DB, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	st, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}

	size := st.Size()
	if size < headerSize {
		_ = f.Close()
		return nil, ErrInvalidDB
	}

	data, err := mmapFile(f, int(size))
	if err != nil {
		_ = f.Close()
		return nil, err
	}

	db := &DB{raw: f, data: data}
	v4, err := parseV4(data)
	if err != nil {
		_ = db.close()
		return nil, err
	}
	db.v4 = v4
	return db, nil
}

func (db *DB) close() error {
	var firstErr error
	hasErr := false
	if len(db.data) > 0 {
		if err := syscall.Munmap(db.data); err != nil {
			if !hasErr {
				firstErr = err
				hasErr = true
			}
		}
		db.data = nil
	}
	if db.raw != nil {
		if err := db.raw.Close(); err != nil {
			if !hasErr {
				firstErr = err
				hasErr = true
			}
		}
		db.raw = nil
	}
	if !hasErr {
		return nil
	}
	return firstErr
}

func mmapFile(f *os.File, size int) ([]byte, error) {
	// syscall.Mmap is still fine on Linux; keep the implementation dependency-free.
	data, err := syscall.Mmap(int(f.Fd()), 0, size, syscall.PROT_READ, syscall.MAP_SHARED)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func parseV4(b []byte) (*v4DB, error) {
	if len(b) < headerSize {
		return nil, ErrInvalidDB
	}
	if string(b[0:4]) != magicV4 {
		return nil, ErrInvalidDB
	}
	ver := binary.LittleEndian.Uint16(b[4:6])
	if ver != version2 {
		return nil, ErrInvalidDB
	}

	stringsOff := int(binary.LittleEndian.Uint32(b[16:20]))
	stringsSize := int(binary.LittleEndian.Uint32(b[20:24]))
	if stringsOff <= 0 || stringsSize <= 0 || stringsOff+stringsSize > len(b) {
		return nil, ErrInvalidDB
	}
	stringsData, stringsStart, stringsEnd, err := parseStringsTable(b[stringsOff : stringsOff+stringsSize])
	if err != nil {
		return nil, err
	}

	v := &v4DB{stringsData: stringsData, stringsStart: stringsStart, stringsEnd: stringsEnd}
	return parseV4v2(v, b)
}

func parseV4v2(v *v4DB, b []byte) (*v4DB, error) {
	secOff := int(binary.LittleEndian.Uint32(b[24:28]))
	secSize := int(binary.LittleEndian.Uint32(b[28:32]))
	if secOff <= 0 || secSize <= 0 || secOff+secSize > len(b) {
		return nil, ErrInvalidDB
	}
	sec := b[secOff : secOff+secSize]
	readU32 := func(off int) int { return int(binary.LittleEndian.Uint32(sec[off : off+4])) }
	readCnt := func(off int) int { return int(binary.LittleEndian.Uint32(sec[off : off+4])) }

	countryLabelsOff := readU32(0)
	countryLabelsCnt := readCnt(4)
	cnLabelsOff := readU32(8)
	cnLabelsCnt := readCnt(12)
	providerLabelsOff := readU32(16)
	providerLabelsCnt := readCnt(20)

	// tables
	countryStartsOff := readU32(24)
	countryEndsOff := readU32(28)
	countryLblsOff := readU32(32)
	countryCnt := readCnt(36)

	cnProvStartsOff := readU32(40)
	cnProvEndsOff := readU32(44)
	cnProvLblsOff := readU32(48)
	cnProvCnt := readCnt(52)

	cnCityStartsOff := readU32(56)
	cnCityEndsOff := readU32(60)
	cnCityLblsOff := readU32(64)
	cnCityCnt := readCnt(68)

	providerStartsOff := readU32(72)
	providerEndsOff := readU32(76)
	providerLblsOff := readU32(80)
	providerCnt := readCnt(84)

	// The DB is stored in little-endian. If the host is big-endian, convert all
	// fixed-width tables in-place so we can use unsafe.Slice cleanly.
	if !nativeLittleEndian {
		if err := swapFixedTablesLEToHost(b,
			countryLabelsOff, countryLabelsCnt,
			cnLabelsOff, cnLabelsCnt,
			providerLabelsOff, providerLabelsCnt,
			countryStartsOff, countryEndsOff, countryLblsOff, countryCnt,
			cnProvStartsOff, cnProvEndsOff, cnProvLblsOff, cnProvCnt,
			cnCityStartsOff, cnCityEndsOff, cnCityLblsOff, cnCityCnt,
			providerStartsOff, providerEndsOff, providerLblsOff, providerCnt,
		); err != nil {
			return nil, err
		}
	}

	var err error
	v.countryLabels, err = sliceFixed[label2](b, countryLabelsOff, countryLabelsCnt)
	if err != nil {
		return nil, err
	}
	v.cnLabels, err = sliceFixed[label2](b, cnLabelsOff, cnLabelsCnt)
	if err != nil {
		return nil, err
	}
	v.providerLabels, err = sliceFixed[providerLabel](b, providerLabelsOff, providerLabelsCnt)
	if err != nil {
		return nil, err
	}

	v.country.starts, err = sliceFixed[uint32](b, countryStartsOff, countryCnt)
	if err != nil {
		return nil, err
	}
	v.country.ends, err = sliceFixed[uint32](b, countryEndsOff, countryCnt)
	if err != nil {
		return nil, err
	}
	v.country.labels, err = sliceFixed[uint32](b, countryLblsOff, countryCnt)
	if err != nil {
		return nil, err
	}

	v.cnProv.starts, err = sliceFixed[uint32](b, cnProvStartsOff, cnProvCnt)
	if err != nil {
		return nil, err
	}
	v.cnProv.ends, err = sliceFixed[uint32](b, cnProvEndsOff, cnProvCnt)
	if err != nil {
		return nil, err
	}
	v.cnProv.labels, err = sliceFixed[uint32](b, cnProvLblsOff, cnProvCnt)
	if err != nil {
		return nil, err
	}

	v.cnCity.starts, err = sliceFixed[uint32](b, cnCityStartsOff, cnCityCnt)
	if err != nil {
		return nil, err
	}
	v.cnCity.ends, err = sliceFixed[uint32](b, cnCityEndsOff, cnCityCnt)
	if err != nil {
		return nil, err
	}
	v.cnCity.labels, err = sliceFixed[uint32](b, cnCityLblsOff, cnCityCnt)
	if err != nil {
		return nil, err
	}

	v.provider.starts, err = sliceFixed[uint32](b, providerStartsOff, providerCnt)
	if err != nil {
		return nil, err
	}
	v.provider.ends, err = sliceFixed[uint32](b, providerEndsOff, providerCnt)
	if err != nil {
		return nil, err
	}
	v.provider.labels, err = sliceFixed[uint32](b, providerLblsOff, providerCnt)
	if err != nil {
		return nil, err
	}

	if len(v.country.starts) != len(v.country.ends) || len(v.country.starts) != len(v.country.labels) {
		return nil, ErrInvalidDB
	}
	if len(v.cnProv.starts) != len(v.cnProv.ends) || len(v.cnProv.starts) != len(v.cnProv.labels) {
		return nil, ErrInvalidDB
	}
	if len(v.cnCity.starts) != len(v.cnCity.ends) || len(v.cnCity.starts) != len(v.cnCity.labels) {
		return nil, ErrInvalidDB
	}
	if len(v.provider.starts) != len(v.provider.ends) || len(v.provider.starts) != len(v.provider.labels) {
		return nil, ErrInvalidDB
	}
	v.country.detectDense()
	v.cnProv.detectDense()
	v.cnCity.detectDense()
	v.provider.detectDense()
	v.country.buildBuckets16()
	v.cnProv.buildBuckets16()
	v.cnCity.buildBuckets16()
	v.provider.buildBuckets16()

	v.providerByKey = make(map[string]uint32, len(v.providerLabels))
	v.providerKindByKey = make(map[string]ProviderKind, len(v.providerLabels))
	for i, pl := range v.providerLabels {
		key := v.str(pl.Key)
		if key == "" {
			return nil, ErrInvalidDB
		}
		v.providerByKey[key] = uint32(i)
		v.providerKindByKey[key] = ProviderKind(pl.Kind)
	}

	return v, nil
}

func swapFixedTablesLEToHost(
	b []byte,
	countryLabelsOff, countryLabelsCnt int,
	cnLabelsOff, cnLabelsCnt int,
	providerLabelsOff, providerLabelsCnt int,
	countryStartsOff, countryEndsOff, countryLblsOff, countryCnt int,
	cnProvStartsOff, cnProvEndsOff, cnProvLblsOff, cnProvCnt int,
	cnCityStartsOff, cnCityEndsOff, cnCityLblsOff, cnCityCnt int,
	providerStartsOff, providerEndsOff, providerLblsOff, providerCnt int,
) error {
	// label2: 2x u32 per entry
	if err := swapU32Words(b, countryLabelsOff, countryLabelsCnt*2); err != nil {
		return err
	}
	if err := swapU32Words(b, cnLabelsOff, cnLabelsCnt*2); err != nil {
		return err
	}
	// providerLabel: 3x u32 per entry
	if err := swapU32Words(b, providerLabelsOff, providerLabelsCnt*3); err != nil {
		return err
	}

	// tables: uint32 arrays
	if err := swapU32Words(b, countryStartsOff, countryCnt); err != nil {
		return err
	}
	if err := swapU32Words(b, countryEndsOff, countryCnt); err != nil {
		return err
	}
	if err := swapU32Words(b, countryLblsOff, countryCnt); err != nil {
		return err
	}

	if err := swapU32Words(b, cnProvStartsOff, cnProvCnt); err != nil {
		return err
	}
	if err := swapU32Words(b, cnProvEndsOff, cnProvCnt); err != nil {
		return err
	}
	if err := swapU32Words(b, cnProvLblsOff, cnProvCnt); err != nil {
		return err
	}

	if err := swapU32Words(b, cnCityStartsOff, cnCityCnt); err != nil {
		return err
	}
	if err := swapU32Words(b, cnCityEndsOff, cnCityCnt); err != nil {
		return err
	}
	if err := swapU32Words(b, cnCityLblsOff, cnCityCnt); err != nil {
		return err
	}

	if err := swapU32Words(b, providerStartsOff, providerCnt); err != nil {
		return err
	}
	if err := swapU32Words(b, providerEndsOff, providerCnt); err != nil {
		return err
	}
	if err := swapU32Words(b, providerLblsOff, providerCnt); err != nil {
		return err
	}

	return nil
}

func swapU32Words(b []byte, off int, count int) error {
	if off < 0 || count < 0 {
		return ErrInvalidDB
	}
	end := off + count*4
	if off == 0 && count == 0 {
		return nil
	}
	if off <= 0 || end > len(b) {
		return ErrInvalidDB
	}
	for i := off; i < end; i += 4 {
		b[i+0], b[i+3] = b[i+3], b[i+0]
		b[i+1], b[i+2] = b[i+2], b[i+1]
	}
	return nil
}

func parseStringsTable(b []byte) (data []byte, start []uint32, end []uint32, err error) {
	if len(b) < 4 {
		return nil, nil, nil, ErrInvalidDB
	}
	count := int(binary.LittleEndian.Uint32(b[0:4]))
	pos := 4
	start = make([]uint32, count)
	end = make([]uint32, count)
	for i := 0; i < count; i++ {
		if pos+2 > len(b) {
			return nil, nil, nil, ErrInvalidDB
		}
		l := int(binary.LittleEndian.Uint16(b[pos : pos+2]))
		pos += 2
		if l < 0 || pos+l > len(b) {
			return nil, nil, nil, ErrInvalidDB
		}
		start[i] = uint32(pos)
		end[i] = uint32(pos + l)
		pos += l
	}
	return b, start, end, nil
}

func sliceFixed[T any](b []byte, off int, count int) ([]T, error) {
	if off < 0 || count < 0 {
		return nil, ErrInvalidDB
	}
	sz := int(unsafe.Sizeof(*new(T)))
	need := off + sz*count
	if off == 0 && count == 0 {
		return nil, nil
	}
	if off <= 0 || need > len(b) {
		return nil, ErrInvalidDB
	}
	p := unsafe.Pointer(&b[off])
	return unsafe.Slice((*T)(p), count), nil
}

func (v *v4DB) str(i uint32) string {
	if i >= uint32(len(v.stringsStart)) {
		return ""
	}
	lo := v.stringsStart[i]
	hi := v.stringsEnd[i]
	if lo > hi || hi > uint32(len(v.stringsData)) {
		return ""
	}
	bs := v.stringsData[lo:hi]
	if len(bs) == 0 {
		return ""
	}
	return unsafe.String(unsafe.SliceData(bs), len(bs))
}

func (v *v4DB) countryLabel(idx uint32) (code, name string) {
	if idx >= uint32(len(v.countryLabels)) {
		return "", ""
	}
	l := v.countryLabels[idx]
	code = v.str(l.Code)
	name = v.str(l.Name)
	return
}

func (v *v4DB) cnLabel(idx uint32) (code, name string) {
	if idx >= uint32(len(v.cnLabels)) {
		return "", ""
	}
	l := v.cnLabels[idx]
	code = v.str(l.Code)
	name = v.str(l.Name)
	return
}

func (v *v4DB) providerLabel(idx uint32) (key, name string, kind ProviderKind) {
	if idx >= uint32(len(v.providerLabels)) {
		return "", "", ProviderKindUnknown
	}
	l := v.providerLabels[idx]
	key = v.str(l.Key)
	name = v.str(l.Name)
	kind = ProviderKind(l.Kind)
	return
}
