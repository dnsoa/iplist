package iplist

import (
	"sort"
)

func (v *v4DB) providerCIDRsAny(provider string) ([]string, ProviderKind, error) {
	idx, ok := v.providerByKey[provider]
	if !ok {
		return nil, ProviderKindUnknown, ErrUnknownVendor
	}
	key, _, kind := v.providerLabel(idx)
	cidrs, err := v.providerCIDRs(key, kind)
	return cidrs, kind, err
}

func (v *v4DB) providerCIDRs(provider string, want ProviderKind) ([]string, error) {
	idx, ok := v.providerByKey[provider]
	if !ok {
		return nil, ErrUnknownVendor
	}
	_, _, kind := v.providerLabel(idx)
	if want != ProviderKindUnknown && kind != want {
		return nil, ErrUnknownVendor
	}

	// Collect ranges for this provider label.
	rangesIdx := make([]int, 0, 1024)
	for i, label := range v.provider.labels {
		if label == idx {
			rangesIdx = append(rangesIdx, i)
		}
	}
	if len(rangesIdx) == 0 {
		return []string{}, nil
	}
	sort.Slice(rangesIdx, func(i, j int) bool { return v.provider.starts[rangesIdx[i]] < v.provider.starts[rangesIdx[j]] })

	out := make([]string, 0, len(rangesIdx))
	for _, i := range rangesIdx {
		ps, err := rangeToCIDRs(v.provider.starts[i], v.provider.ends[i])
		if err != nil {
			return nil, err
		}
		for _, p := range ps {
			out = append(out, p.String())
		}
	}
	return out, nil
}
