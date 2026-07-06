package sdk

import "fmt"

func errorf(format string, args ...any) error {
	return fmt.Errorf("tabibu-ext-sdk: "+format, args...)
}
