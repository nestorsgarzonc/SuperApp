package errors

const (
	ErrCodeValidationFailed ErrorCode = "VAL_001"
	ErrCodeAuthFailed       ErrorCode = "AUTH_001"
	ErrCodePermissionDenied ErrorCode = "PERM_001"
	ErrCodeResourceNotFound ErrorCode = "RES_001"
	ErrCodeConflict         ErrorCode = "RES_002"
	ErrCodeQRUnavailable    ErrorCode = "QR_001"
	ErrCodePayloadTooLarge  ErrorCode = "SYS_003"
	ErrCodeInternalError    ErrorCode = "SYS_001"
	ErrCodeBadRequest       ErrorCode = "SYS_002"
)
