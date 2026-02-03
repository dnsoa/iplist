package iplist

import (
	"errors"
	"net/netip"
	"os"
)

// DB is an opened IP list database.
//
// The current implementation supports IPv4 only.
//
// Use Open to load a built database file.
// Use the cmd/iplist build command to generate a database file from a data/ directory.
type DB struct {
	raw  *os.File
	data []byte
	v4   *v4DB
}

// Result is the lookup result for a single IP.
// Fields may be empty when the category has no match.
//
// City is based on CN admin code data (province/city level).
// Provider covers both ISP and cloud vendors.
type Result struct {
	IP netip.Addr

	CountryCode string
	CountryName string

	CNProvinceCode string
	CNProvinceName string

	CNCityCode string
	CNCityName string

	ProviderKey  string // e.g. aliyun, chinatelecom
	ProviderName string
	ProviderKind ProviderKind
}

// ResultIDs is a low-level lookup result that only contains label IDs.
// It avoids decoding strings on the hot path.
//
// ID values are indices into the underlying label tables. When a field has no match,
// it will be set to IDNone.
type ResultIDs struct {
	IP netip.Addr

	CountryID    uint32
	CNProvinceID uint32
	CNCityID     uint32

	ProviderID   uint32
	ProviderKind ProviderKind
}

const IDNone uint32 = ^uint32(0)

type ProviderKind uint8

const (
	ProviderKindUnknown ProviderKind = 0
	ProviderKindISP     ProviderKind = 1
	ProviderKindCloud   ProviderKind = 2
)

var (
	ErrInvalidDB      = errors.New("iplist: invalid db")
	ErrUnsupportedIP  = errors.New("iplist: unsupported ip (ipv4 only)")
	ErrInvalidIP      = errors.New("iplist: invalid ip")
	ErrNilResult      = errors.New("iplist: nil result")
	ErrUnknownVendor  = errors.New("iplist: unknown provider")
	ErrUnknownCountry = errors.New("iplist: unknown country")
	ErrUnknownCity    = errors.New("iplist: unknown cn city")
)

// Open opens an existing database file built by cmd/iplist build.
func Open(path string) (*DB, error) {
	return open(path)
}

// Close releases underlying resources.
func (db *DB) Close() error {
	if db == nil {
		return nil
	}
	return db.close()
}

// Lookup finds the country/city/provider information for a given IP.
func (db *DB) Lookup(ip string) (Result, bool, error) {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return Result{}, false, ErrInvalidIP
	}
	return db.LookupAddr(addr)
}

// LookupInto is like Lookup but writes the result into dst to avoid copying
// a large Result value on return.
func (db *DB) LookupInto(ip string, dst *Result) (bool, error) {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return false, ErrInvalidIP
	}
	return db.LookupAddrInto(addr, dst)
}

// LookupIDs is like Lookup but returns label IDs only.
func (db *DB) LookupIDs(ip string) (ResultIDs, bool, error) {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return ResultIDs{}, false, ErrInvalidIP
	}
	return db.LookupAddrIDs(addr)
}

// LookupIDsInto is like LookupIDs but writes into dst.
func (db *DB) LookupIDsInto(ip string, dst *ResultIDs) (bool, error) {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return false, ErrInvalidIP
	}
	return db.LookupAddrIDsInto(addr, dst)
}

// LookupAddr is like Lookup but takes a parsed address.
func (db *DB) LookupAddr(addr netip.Addr) (Result, bool, error) {
	if db == nil || db.v4 == nil {
		return Result{}, false, ErrInvalidDB
	}
	if !addr.Is4() {
		return Result{}, false, ErrUnsupportedIP
	}
	return db.v4.lookup(addr)
}

// LookupAddrIDs is like LookupAddr but returns label IDs only.
func (db *DB) LookupAddrIDs(addr netip.Addr) (ResultIDs, bool, error) {
	var out ResultIDs
	matched, err := db.LookupAddrIDsInto(addr, &out)
	if err != nil {
		return ResultIDs{}, false, err
	}
	return out, matched, nil
}

// LookupAddrIDsInto is like LookupAddrInto but avoids decoding strings.
func (db *DB) LookupAddrIDsInto(addr netip.Addr, dst *ResultIDs) (bool, error) {
	if db == nil || db.v4 == nil {
		return false, ErrInvalidDB
	}
	if dst == nil {
		return false, ErrNilResult
	}
	if !addr.Is4() {
		clearResultIDs(dst)
		return false, ErrUnsupportedIP
	}
	clearResultIDs(dst)
	dst.IP = addr
	matched, err := db.v4.lookupIDsInto(addr, dst)
	if err != nil {
		clearResultIDs(dst)
		return false, err
	}
	return matched, nil
}

// LookupIPv4Uint32Into looks up an IPv4 address represented as a uint32.
// The input must be in network-byte order layout: a.b.c.d => a<<24|b<<16|c<<8|d.
func (db *DB) LookupIPv4Uint32Into(ip uint32, dst *Result) (bool, error) {
	if db == nil || db.v4 == nil {
		return false, ErrInvalidDB
	}
	if dst == nil {
		return false, ErrNilResult
	}
	clearResult(dst)
	return db.v4.lookupIntoU32(ip, dst)
}

// LookupIPv4Uint32IDsInto is like LookupAddrIDsInto but takes uint32 IPv4 directly.
func (db *DB) LookupIPv4Uint32IDsInto(ip uint32, dst *ResultIDs) (bool, error) {
	if db == nil || db.v4 == nil {
		return false, ErrInvalidDB
	}
	if dst == nil {
		return false, ErrNilResult
	}
	clearResultIDs(dst)
	return db.v4.lookupIDsIntoU32(ip, dst)
}

// LookupAddrProviderID looks up provider label ID only.
// It avoids decoding strings and avoids country/city lookups.
func (db *DB) LookupAddrProviderID(addr netip.Addr) (providerID uint32, kind ProviderKind, ok bool, err error) {
	if db == nil || db.v4 == nil {
		return IDNone, ProviderKindUnknown, false, ErrInvalidDB
	}
	if !addr.Is4() {
		return IDNone, ProviderKindUnknown, false, ErrUnsupportedIP
	}
	ip4 := addr.As4()
	ip := uint32(ip4[0])<<24 | uint32(ip4[1])<<16 | uint32(ip4[2])<<8 | uint32(ip4[3])
	providerID, kind, ok = db.v4.lookupProviderIDU32(ip)
	return providerID, kind, ok, nil
}

// LookupIPv4Uint32ProviderID looks up provider label ID only.
// The input must be in network-byte order layout: a.b.c.d => a<<24|b<<16|c<<8|d.
func (db *DB) LookupIPv4Uint32ProviderID(ip uint32) (providerID uint32, kind ProviderKind, ok bool, err error) {
	if db == nil || db.v4 == nil {
		return IDNone, ProviderKindUnknown, false, ErrInvalidDB
	}
	providerID, kind, ok = db.v4.lookupProviderIDU32(ip)
	return providerID, kind, ok, nil
}

// LookupAddrInto is like LookupAddr but writes the result into dst.
// This avoids returning a large Result by value (which shows up as runtime.duffcopy
// in CPU profiles for tight loops).
func (db *DB) LookupAddrInto(addr netip.Addr, dst *Result) (bool, error) {
	if db == nil || db.v4 == nil {
		return false, ErrInvalidDB
	}
	if dst == nil {
		return false, ErrNilResult
	}
	if !addr.Is4() {
		clearResult(dst)
		return false, ErrUnsupportedIP
	}
	// Avoid copying a large Result value (runtime.duffcopy hotspot).
	clearResult(dst)
	dst.IP = addr
	matched, err := db.v4.lookupInto(addr, dst)
	if err != nil {
		clearResult(dst)
		return false, err
	}
	return matched, nil
}

func clearResult(dst *Result) {
	dst.IP = netip.Addr{}
	dst.CountryCode = ""
	dst.CountryName = ""
	dst.CNProvinceCode = ""
	dst.CNProvinceName = ""
	dst.CNCityCode = ""
	dst.CNCityName = ""
	dst.ProviderKey = ""
	dst.ProviderName = ""
	dst.ProviderKind = ProviderKindUnknown
}

func clearResultIDs(dst *ResultIDs) {
	dst.IP = netip.Addr{}
	dst.CountryID = IDNone
	dst.CNProvinceID = IDNone
	dst.CNCityID = IDNone
	dst.ProviderID = IDNone
	dst.ProviderKind = ProviderKindUnknown
}

// Decode helpers (cold path)

func (db *DB) CountryByID(id uint32) (code, name string, ok bool) {
	if db == nil || db.v4 == nil {
		return "", "", false
	}
	if id == IDNone {
		return "", "", false
	}
	code, name = db.v4.countryLabel(id)
	return code, name, code != ""
}

func (db *DB) CNByID(id uint32) (code, name string, ok bool) {
	if db == nil || db.v4 == nil {
		return "", "", false
	}
	if id == IDNone {
		return "", "", false
	}
	code, name = db.v4.cnLabel(id)
	return code, name, code != ""
}

func (db *DB) ProviderByID(id uint32) (key, name string, kind ProviderKind, ok bool) {
	if db == nil || db.v4 == nil {
		return "", "", ProviderKindUnknown, false
	}
	if id == IDNone {
		return "", "", ProviderKindUnknown, false
	}
	key, name, kind = db.v4.providerLabel(id)
	return key, name, kind, key != ""
}

// CloudIPs returns all CIDRs for a given cloud vendor (e.g. "aliyun").
// Output lines are normalized CIDR strings.
func (db *DB) CloudIPs(vendor string) ([]string, error) {
	if db == nil || db.v4 == nil {
		return nil, ErrInvalidDB
	}
	return db.v4.providerCIDRs(vendor, ProviderKindCloud)
}

// ProviderIPs returns all CIDRs for a provider (ISP or cloud).
// It also returns the resolved kind.
func (db *DB) ProviderIPs(provider string) ([]string, ProviderKind, error) {
	if db == nil || db.v4 == nil {
		return nil, ProviderKindUnknown, ErrInvalidDB
	}
	return db.v4.providerCIDRsAny(provider)
}









































































