package common

// ValidPassword accepts printable ASCII, 12–72 bytes, with English letters and
// digits. Symbols are allowed. The byte cap respects bcrypt's maximum input length.
func ValidPassword(password string) bool {
	if len(password) < 12 || len(password) > 72 {
		return false
	}
	var letter, digit bool
	for _, c := range password {
		switch {
		case c >= 'A' && c <= 'Z':
			letter = true
		case c >= 'a' && c <= 'z':
			letter = true
		case c >= '0' && c <= '9':
			digit = true
		case c >= 33 && c <= 126:
			continue
		default:
			return false
		}
	}
	return letter && digit
}
