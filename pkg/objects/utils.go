package objects

import "fmt"

func ResourceIDToFullName(prefix string, resourceID string) string {
	return fmt.Sprintf(prefix+"/%s", resourceID)
}
