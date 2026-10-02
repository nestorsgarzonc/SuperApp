package permissions

type (
	Presenter struct {
		ID          string
		Code        string
		Description string
	}

	ListPresenter struct {
		Permissions []Presenter
	}

	GetRequester struct {
		PermissionID string
	}

	UpdateRequester struct {
		PermissionID string
		Code         *string
		Description  *string
	}

	DeleteRequester struct {
		PermissionID string
	}

	ListRequester struct{}

	DeletePresenter struct{}
)
