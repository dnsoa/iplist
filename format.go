package iplist

const (
	magicV4  = "IPL4"
	version2 = 2
	headerSize = 64
)

const labelNone uint32 = ^uint32(0)

// entry is a lookup record sorted by Start.
// Start/End are inclusive IPv4 integer addresses.
// Label is an index into a label table.
type entry struct {
	Start uint32
	End   uint32
	Label uint32
}

type label2 struct {
	Code uint32
	Name uint32
}

type providerLabel struct {
	Key  uint32
	Name uint32
	Kind uint32
}
