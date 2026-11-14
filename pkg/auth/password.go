package auth

import "golang.org/x/crypto/bcrypt"

// bcryptCost is the bcrypt work factor. 10 is the library default and a
// reasonable latency/security tradeoff for an admin login.
const bcryptCost = 10

// HashPassword returns a bcrypt digest of password.
func HashPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// CheckPassword reports whether password matches hash.
func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
