package protocol

// EncryptionLevel is the encryption level
// Default value is Unencrypted
type EncryptionLevel int

const (
	// EncryptionUnspecified is a not specified encryption level
	EncryptionUnspecified EncryptionLevel = iota
	// EncryptionUnencrypted is not encrypted
	EncryptionUnencrypted
	// EncryptionSecure is encrypted, but not forward secure
	EncryptionSecure
	// EncryptionForwardSecure is forward secure
	// 前向安全代表了握手之后的加密状态
	EncryptionForwardSecure
)

func (e EncryptionLevel) String() string {
	switch e {
	case EncryptionUnencrypted:
		return "unencrypted"
	case EncryptionSecure:
		return "encrypted (not forward-secure)"
	case EncryptionForwardSecure:
		return "forward-secure"
	}
	return "unknown"
}
