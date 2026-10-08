package auth

type LoginRequestDTO struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Device   string `json:"device_name"`
}

type RegisterRequestDTO struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	FullName string `json:"full_name"`
}

type RefreshRequestDTO struct {
	RefreshToken string `json:"refresh_token"`
}

type LogoutRequestDTO struct {
	RefreshToken string `json:"refresh_token"`
}
