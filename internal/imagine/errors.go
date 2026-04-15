package imagine

import "fmt"

type InvalidParamsError struct {
	Message string
}

func (e InvalidParamsError) Error() string {
	return e.Message
}

func invalidParams(format string, args ...any) error {
	return InvalidParamsError{Message: fmt.Sprintf(format, args...)}
}
