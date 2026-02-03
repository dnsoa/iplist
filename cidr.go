package iplist

import (
	"fmt"
	"math/bits"
	"net/netip"
)

func rangeToCIDRs(start, end uint32) ([]netip.Prefix, error) {
	if start > end {
		return nil, nil
	}
	out := make([]netip.Prefix, 0, 8)
	cur := start
	for cur <= end {
		// Largest power-of-two block starting at cur.
		maxSize := cur & -cur
		if maxSize == 0 {
			maxSize = 1 << 31
		}
		maxLen := 32 - bits.TrailingZeros32(maxSize)
		remaining := end - cur + 1
		for maxLen < 32 {
			blockSize := uint32(1) << (32 - maxLen)
			if blockSize <= remaining {
				break
			}
			maxLen++
		}
		addr := netip.AddrFrom4([4]byte{byte(cur >> 24), byte(cur >> 16), byte(cur >> 8), byte(cur)})
		p := netip.PrefixFrom(addr, maxLen)
		out = append(out, p)
		step := uint32(1) << (32 - maxLen)
		if step == 0 {
			return nil, fmt.Errorf("rangeToCIDRs overflow")
		}
		next := cur + step
		if next <= cur {
			break
		}
		cur = next
		if cur == 0 {
			break
		}
	}
	return out, nil
}
