package iplist

func docsCountryName(code string) (string, bool) {
	return docsLookupFixedKeys(code, 2, docsCountryKeys, docsCountryVals, docsCountryOff)
}

func docsCNCityName(code string) (string, bool) {
	return docsLookupFixedKeys(code, 6, docsCNCityKeys, docsCNCityVals, docsCNCityOff)
}

func docsLookupFixedKeys(code string, keyLen int, keys, vals string, off []uint32) (string, bool) {
	if keyLen <= 0 || len(code) != keyLen {
		return "", false
	}
	if len(off) < 2 {
		return "", false
	}
	n := len(off) - 1
	if n <= 0 {
		return "", false
	}
	if len(keys) != n*keyLen {
		return "", false
	}
	if len(vals) == 0 {
		return "", false
	}

	lo, hi := 0, n
	for lo < hi {
		mid := (lo + hi) >> 1
		k := keys[mid*keyLen : (mid+1)*keyLen]
		if k < code {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	if lo >= n {
		return "", false
	}
	if keys[lo*keyLen:(lo+1)*keyLen] != code {
		return "", false
	}
	s := off[lo]
	e := off[lo+1]
	if s > e || int(e) > len(vals) {
		return "", false
	}
	if s == e {
		return "", true
	}
	return vals[s:e], true
}
