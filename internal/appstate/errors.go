package appstate

import "fmt"

func errMissingConfigStore() error {
	return fmt.Errorf("config store not available")
}
