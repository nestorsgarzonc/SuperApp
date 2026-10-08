package password

import "golang.org/x/crypto/bcrypt"

const bcryptCost = 12

var dummyHash = func() []byte {
	b, err := bcrypt.GenerateFromPassword([]byte("auth-user-not-found-dummy-password"), bcryptCost)
	if err != nil {
		panic(err)
	}
	return b
}()

type Passwords struct{}

func (Passwords) Hash(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	return string(b), err
}
func (Passwords) Verify(encoded, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(encoded), []byte(password))
}

// Burn performs a bcrypt comparison for unknown addresses to reduce timing-based enumeration.
func (Passwords) Burn(password string) {
	_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
}
