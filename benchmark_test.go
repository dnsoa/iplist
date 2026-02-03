package iplist

import (
	"net/netip"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

var (
	benchOnce sync.Once
	benchDB   *DB
	benchErr  error
)

func openBenchDB(b *testing.B) *DB {
	b.Helper()

	benchOnce.Do(func() {
		// Prefer a prebuilt DB if user provides it.
		if p := os.Getenv("IPLIST_DB"); p != "" {
			benchDB, benchErr = Open(p)
			return
		}

		// Build a temporary DB from the repository's data/ directory.
		tmp := b.TempDir()
		out := filepath.Join(tmp, "iplist.db")
		benchErr = Build("data", out)
		if benchErr != nil {
			return
		}
		benchDB, benchErr = Open(out)
	})

	if benchErr != nil {
		b.Fatalf("open bench db: %v", benchErr)
	}
	if benchDB == nil {
		b.Fatalf("open bench db: nil db")
	}
	return benchDB
}

func BenchmarkLookup_8_160_0_3(b *testing.B) {
	db := openBenchDB(b)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _, err := db.Lookup("8.160.0.3")
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLookupAddr_8_160_0_3(b *testing.B) {
	db := openBenchDB(b)
	addr := netip.MustParseAddr("8.160.0.3")

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _, err := db.LookupAddr(addr)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLookupAddrInto_8_160_0_3(b *testing.B) {
	db := openBenchDB(b)
	addr := netip.MustParseAddr("8.160.0.3")
	var dst Result

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := db.LookupAddrInto(addr, &dst)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLookupAddrIDsInto_8_160_0_3(b *testing.B) {
	db := openBenchDB(b)
	addr := netip.MustParseAddr("8.160.0.3")
	var dst ResultIDs

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := db.LookupAddrIDsInto(addr, &dst)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLookupIPv4Uint32IDsInto_8_160_0_3(b *testing.B) {
	db := openBenchDB(b)
	addr := netip.MustParseAddr("8.160.0.3")
	ip4 := addr.As4()
	ip := uint32(ip4[0])<<24 | uint32(ip4[1])<<16 | uint32(ip4[2])<<8 | uint32(ip4[3])
	var dst ResultIDs

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := db.LookupIPv4Uint32IDsInto(ip, &dst)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLookupAddrProviderID_8_160_0_3(b *testing.B) {
	db := openBenchDB(b)
	addr := netip.MustParseAddr("8.160.0.3")

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _, _, err := db.LookupAddrProviderID(addr)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLookupIPv4Uint32ProviderID_8_160_0_3(b *testing.B) {
	db := openBenchDB(b)
	addr := netip.MustParseAddr("8.160.0.3")
	ip4 := addr.As4()
	ip := uint32(ip4[0])<<24 | uint32(ip4[1])<<16 | uint32(ip4[2])<<8 | uint32(ip4[3])

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _, _, err := db.LookupIPv4Uint32ProviderID(ip)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTableLookupOnly_8_160_0_3(b *testing.B) {
	db := openBenchDB(b)
	addr := netip.MustParseAddr("8.160.0.3")
	ip4 := addr.As4()
	ip := uint32(ip4[0])<<24 | uint32(ip4[1])<<16 | uint32(ip4[2])<<8 | uint32(ip4[3])
	t := db.v4.provider

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = t.lookup(ip)
	}
}
