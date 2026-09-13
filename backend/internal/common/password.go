package common

// ValidPassword accepts printable ASCII, 12–72 bytes, with all four character
// classes. The byte cap respects bcrypt's maximum input length.
func ValidPassword(password string) bool {
	if len(password) < 12 || len(password) > 72 {
		return false
	}
	var upper, lower, digit, symbol bool
	for _, c := range password {
		switch {
		case c >= 'A' && c <= 'Z':
			upper = true
		case c >= 'a' && c <= 'z':
			lower = true
		case c >= '0' && c <= '9':
			digit = true
		case c >= 33 && c <= 126:
			symbol = true
		default:
			return false
		}
	}
	return upper && lower && digit && symbol
}
