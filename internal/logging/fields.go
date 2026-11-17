package logging

// Reserved context keys. These spellings are part of the logging contract;
// callers must use Fields rather than inventing aliases such as device_sn.
const (
	KeyDevice = "device"
	KeyPoint  = "point"
	KeySeq    = "seq"
	KeyLink   = "link"
	KeyReason = "reason"
)

// Fields carries the reserved context keys. Seq is a pointer so that a
// legitimate sequence number of zero is still emitted.
type Fields struct {
	Device string
	Point  string
	Link   string
	Reason string
	Seq    *uint64
}

// Attrs renders the non-empty fields as alternating key/value pairs suitable
// for Logger.Info/Warn/Error.
func (f Fields) Attrs() []any {
	attrs := make([]any, 0, 10)
	if f.Device != "" {
		attrs = append(attrs, KeyDevice, f.Device)
	}
	if f.Point != "" {
		attrs = append(attrs, KeyPoint, f.Point)
	}
	if f.Seq != nil {
		attrs = append(attrs, KeySeq, *f.Seq)
	}
	if f.Link != "" {
		attrs = append(attrs, KeyLink, f.Link)
	}
	if f.Reason != "" {
		attrs = append(attrs, KeyReason, f.Reason)
	}
	return attrs
}
