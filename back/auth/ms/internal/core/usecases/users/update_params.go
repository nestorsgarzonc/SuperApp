package users

type UpdateParams struct {
	ID       string
	Email    *string
	FullName *string
	Status   *string
}
