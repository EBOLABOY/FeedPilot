package batchexecute

import (
	"crypto/sha1"
	"fmt"
	"strings"
	"time"
)

// extractSAPISID extracts the SAPISID value from cookies
func extractSAPISID(cookies string) string {
	parts := strings.Split(cookies, ";")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "SAPISID=") {
			return strings.TrimPrefix(part, "SAPISID=")
		}
	}
	return ""
}

// generateSAPISIDHASH generates the SAPISIDHASH for Google API authentication
func generateSAPISIDHASH(sapisid string, origin string) string {
	timestamp := time.Now().Unix()
	data := fmt.Sprintf("%d %s %s", timestamp, sapisid, origin)
	hash := sha1.New()
	hash.Write([]byte(data))
	return fmt.Sprintf("SAPISIDHASH %d_%x", timestamp, hash.Sum(nil))
}
