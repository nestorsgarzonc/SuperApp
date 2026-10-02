package errors

import (
	"encoding/json"
	stderrors "errors"
	"log/slog"
	"net/http"
)

type ErrorResponse struct {
	Success bool         `json:"success"`
	Error   ErrorPayload `json:"error"`
}

type ErrorPayload struct {
	Code    ErrorCode              `json:"code"`
	Message string                 `json:"message"`
	Details map[string]interface{} `json:"details,omitempty"`
}

type SuccessResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
}

func RespondError(w http.ResponseWriter, logger *slog.Logger, err error) {
	appErr := Wrap(err)
	if appErr == nil {
		return
	}

	// Internal errors must never leak the wrapped cause (DB/driver strings,
	// constraint/table/column names, SQL fragments) to the client. Log the
	// real cause server-side and return a generic message.
	clientMessage := appErr.Error()
	if appErr.Type() == TypeInternal {
		if logger != nil {
			logger.Error("Internal error",
				"error", err,
				"code", appErr.Code(),
			)
		}
		clientMessage = "An unexpected error occurred"
	}

	response := ErrorResponse{
		Success: false,
		Error: ErrorPayload{
			Code:    appErr.Code(),
			Message: clientMessage,
			Details: appErr.Details(),
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(appErr.HTTPStatus())

	if err := json.NewEncoder(w).Encode(response); err != nil && logger != nil {
		logger.Error("Failed to encode error response", "error", err)
	}
}

func RespondSuccess(w http.ResponseWriter, data interface{}, statusCode int) {
	response := SuccessResponse{
		Success: true,
		Data:    data,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(response)
}

func RespondCreated(w http.ResponseWriter, data interface{}) {
	RespondSuccess(w, data, http.StatusCreated)
}

func RespondOK(w http.ResponseWriter, data interface{}) {
	RespondSuccess(w, data, http.StatusOK)
}

// MaxRequestBodyBytes caps every JSON request body. Without a cap a single
// client could make the server read and decode an arbitrarily large payload.
const MaxRequestBodyBytes int64 = 1 << 20 // 1 MiB

func DecodeJSONLenient(w http.ResponseWriter, r *http.Request, target interface{}) error {
	if r.Body == nil {
		return NewBadRequestError("Request body is empty")
	}
	r.Body = http.MaxBytesReader(w, r.Body, MaxRequestBodyBytes)

	if err := json.NewDecoder(r.Body).Decode(target); err != nil {
		var tooBig *http.MaxBytesError
		if stderrors.As(err, &tooBig) {
			return NewPayloadTooLargeError(MaxRequestBodyBytes)
		}
		return NewBadRequestError("Invalid JSON: " + err.Error())
	}

	return nil
}
