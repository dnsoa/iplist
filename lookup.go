package iplist

import (
	"net/netip"
	"unsafe"
)

func (v *v4DB) lookup(addr netip.Addr) (Result, bool, error) {
	res := Result{IP: addr}
	matched, err := v.lookupInto(addr, &res)
	if err != nil {
		return Result{}, false, err
	}
	return res, matched, nil
}

func (v *v4DB) lookupInto(addr netip.Addr, dst *Result) (bool, error) {
	ip4 := addr.As4()
	ip := uint32(ip4[0])<<24 | uint32(ip4[1])<<16 | uint32(ip4[2])<<8 | uint32(ip4[3])
	return v.lookupIntoU32(ip, dst)
}

func (v *v4DB) lookupIntoU32(ip uint32, dst *Result) (bool, error) {

	matched := false

	if label, ok := v.country.lookup(ip); ok {
		code, name := v.countryLabel(label)
		dst.CountryCode = code
		dst.CountryName = name
		matched = true
	}

	// CN city data (prefer city-level match).
	if label, ok := v.cnCity.lookup(ip); ok {
		code, name := v.cnLabel(label)
		dst.CNCityCode = code
		dst.CNCityName = name
		matched = true
	} else if label, ok := v.cnProv.lookup(ip); ok {
		code, name := v.cnLabel(label)
		dst.CNProvinceCode = code
		dst.CNProvinceName = name
		matched = true
	}

	if label, ok := v.provider.lookup(ip); ok {
		key, name, kind := v.providerLabel(label)
		dst.ProviderKey = key
		dst.ProviderName = name
		dst.ProviderKind = kind
		matched = true
	}

	return matched, nil
}

func (v *v4DB) lookupIDsInto(addr netip.Addr, dst *ResultIDs) (bool, error) {
	ip4 := addr.As4()
	ip := uint32(ip4[0])<<24 | uint32(ip4[1])<<16 | uint32(ip4[2])<<8 | uint32(ip4[3])
	return v.lookupIDsIntoU32(ip, dst)
}

func (v *v4DB) lookupIDsIntoU32(ip uint32, dst *ResultIDs) (bool, error) {

	matched := false

	if label, ok := v.country.lookup(ip); ok {
		dst.CountryID = label
		matched = true
	}

	if label, ok := v.cnCity.lookup(ip); ok {
		dst.CNCityID = label
		matched = true
	} else if label, ok := v.cnProv.lookup(ip); ok {
		dst.CNProvinceID = label
		matched = true
	}

	if label, ok := v.provider.lookup(ip); ok {
		dst.ProviderID = label
		if label < uint32(len(v.providerLabels)) {
			dst.ProviderKind = ProviderKind(v.providerLabels[label].Kind)
		}
		matched = true
	}

	return matched, nil
}

func (v *v4DB) lookupProviderIDU32(ip uint32) (providerID uint32, kind ProviderKind, ok bool) {
	label, ok := v.provider.lookup(ip)
	if !ok {
		return IDNone, ProviderKindUnknown, false
	}
	providerID = label
	if label < uint32(len(v.providerLabels)) {
		kind = ProviderKind(v.providerLabels[label].Kind)
	}
	return providerID, kind, true
}

func (t v4Table) lookup(ip uint32) (uint32, bool) {
	if len(t.starts) == 0 {
		return 0, false
	}
	baseS := unsafe.Pointer(&t.starts[0])
	baseL := unsafe.Pointer(&t.labels[0])
	baseE := unsafe.Pointer(&t.ends[0])

	lo := 0
	hi := len(t.starts)
	if t.bucketLo16 != nil && t.bucketHi16 != nil {
		p := int(ip >> 16)
		lo = int(t.bucketLo16[p])
		hi = int(t.bucketHi16[p])
		if lo < 0 {
			lo = 0
		}
		if hi > len(t.starts) {
			hi = len(t.starts)
		}
		if lo >= hi {
			return 0, false
		}
	}

	// If the narrowed window is small, linear scan can outperform binary search
	// due to fewer branches and better locality.
	const linearThreshold = 32
	if hi-lo <= linearThreshold {
		for idx := hi - 1; idx >= lo; idx-- {
			start := *(*uint32)(unsafe.Add(baseS, uintptr(idx)<<2))
			if start > ip {
				continue
			}
			label := *(*uint32)(unsafe.Add(baseL, uintptr(idx)<<2))
			if t.dense {
				if label == labelNone {
					return 0, false
				}
				return label, true
			}
			end := *(*uint32)(unsafe.Add(baseE, uintptr(idx)<<2))
			if ip > end {
				return 0, false
			}
			return label, true
		}
		return 0, false
	}

	i, j := lo, hi
	for i < j {
		h := (i + j) >> 1
		if *(*uint32)(unsafe.Add(baseS, uintptr(h)<<2)) > ip {
			j = h
		} else {
			i = h + 1
		}
	}
	idx := i - 1
	if idx < lo {
		return 0, false
	}
	label := *(*uint32)(unsafe.Add(baseL, uintptr(idx)<<2))
	if t.dense {
		if label == labelNone {
			return 0, false
		}
		return label, true
	}
	end := *(*uint32)(unsafe.Add(baseE, uintptr(idx)<<2))
	if ip > end {
		return 0, false
	}
	return label, true
}
