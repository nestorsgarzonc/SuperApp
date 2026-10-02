package permissions

type Presenter struct {
	ID          string
	Code        string
	Description string
}

type ListPresenter struct {
	Permissions []Presenter
}

type GetRequester struct {
	PermissionID string
}

type ListRequester struct{}

type UpdateRequester struct {
	PermissionID string
	Code         *string
	Description  *string
}

type DeleteRequester struct {
	PermissionID string
}

type DeletePresenter struct{}
