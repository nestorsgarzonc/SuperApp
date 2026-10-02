package errors

import (
	"encoding/json"
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

func DecodeJSONLenient(r *http.Request, target interface{}) error {
	if r.Body == nil {
		return NewBadRequestError("Request body is empty")
	}

	if err := json.NewDecoder(r.Body).Decode(target); err != nil {
		return NewBadRequestError("Invalid JSON: " + err.Error())
	}

	return nil
}
