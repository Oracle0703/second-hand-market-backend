package auth

const legacyDefaultAdminPassword = "Admin@123456"

func IsSafeAdministratorPassword(password string) bool {
	passwordBytes := len([]byte(password))
	return passwordBytes >= 12 && passwordBytes <= 72 && password != legacyDefaultAdminPassword
}
