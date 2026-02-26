package runtimeapi

type usecaseError struct {
	Status  int
	Code    string
	Message string
	Details any
}

func (e *usecaseError) Error() string {
	if e == nil {
		return ""
	}
	return e.Code + ": " + e.Message
}
