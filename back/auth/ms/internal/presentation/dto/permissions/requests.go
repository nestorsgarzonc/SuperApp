package permissions

type CreateRequestDTO struct {
	Code        string `json:"code"`
	Description string `json:"description"`
}

type UpdateRequestDTO struct {
	Code        *string `json:"code"`
	Description *string `json:"description"`
}
