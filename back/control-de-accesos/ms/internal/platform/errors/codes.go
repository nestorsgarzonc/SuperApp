package errors

const (
	ErrCodeValidationFailed ErrorCode = "VAL_001"
	ErrCodeAuthFailed       ErrorCode = "AUTH_001"
	ErrCodePermissionDenied ErrorCode = "PERM_001"
	ErrCodeResourceNotFound ErrorCode = "RES_001"
	ErrCodeInternalError    ErrorCode = "SYS_001"
	ErrCodeBadRequest       ErrorCode = "SYS_002"
)
