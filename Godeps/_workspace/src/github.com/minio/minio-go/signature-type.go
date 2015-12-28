package minio

// SignatureType is type of Authorization requested for a given HTTP request.
type SignatureType int

// Different types of supported signatures - default is Latest i.e SignatureV4.
const (
	Latest SignatureType = iota
	SignatureV4
	SignatureV2
)

// isV2 - is signature SignatureV2?
func (s SignatureType) isV2() bool {
	return s == SignatureV2
}

// isV4 - is signature SignatureV4?
func (s SignatureType) isV4() bool {
	return s == SignatureV4 || s == Latest
}
