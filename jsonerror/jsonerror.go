package jsonerror

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type SeleniumError struct {
	Name   string
	Err    error
	Status int
}

func (se *SeleniumError) Error() string {
	return fmt.Errorf("%s: %v", se.Name, se.Err).Error()
}

func newSeleniumError(name string, err error, status int) *SeleniumError {
	return &SeleniumError{
		Name:   name,
		Err:    err,
		Status: status,
	}
}

func InvalidArgument(err error) *SeleniumError {
	return newSeleniumError("invalid argument", err, http.StatusBadRequest)
}

func InvalidSessionID(err error) *SeleniumError {
	return newSeleniumError("invalid session id", err, http.StatusNotFound)
}

func SessionNotCreated(err error) *SeleniumError {
	return newSeleniumError("session not created", err, http.StatusInternalServerError)
}

func UnknownError(err error) *SeleniumError {
	return newSeleniumError("unknown error", err, http.StatusInternalServerError)
}

func (se *SeleniumError) Encode(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(se.Status)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"value": map[string]string{
			"error":   se.Name,
			"message": se.Err.Error(),
		},
	})
}
