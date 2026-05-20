// Package names derives stable Kubernetes object names for NSX resources.
package names

import (
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"strings"
)

const (
	maxDNS1123SubdomainLength = 253
	truncatedNameHashLength   = 16
)

// NSXGroupLogicalID identifies an NSX group within a network cloud.
type NSXGroupLogicalID struct {
	NetworkCloudFQDN string
	GroupID          string
}

// NormalizeNetworkCloudFQDN normalizes a configured NSX manager host value.
func NormalizeNetworkCloudFQDN(value string) string {
	trimmed := strings.TrimSpace(value)
	trimmed = strings.TrimRight(trimmed, "/")
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return strings.ToLower(trimmed)
	}
	if parsed.Host != "" {
		return strings.ToLower(parsed.Host)
	}
	return strings.ToLower(trimmed)
}

// NSXGroupName returns the Kubernetes metadata name for an NSX group.
func NSXGroupName(id NSXGroupLogicalID) string {
	cloud := NormalizeNetworkCloudFQDN(id.NetworkCloudFQDN)
	cloud = strings.ReplaceAll(cloud, ":", "-")
	groupID := strings.TrimSpace(id.GroupID)
	candidate := cloud + "--" + groupID
	return kubernetesMetadataName(candidate)
}

func kubernetesMetadataName(value string) string {
	var builder strings.Builder
	lastWasSeparator := false
	for _, char := range strings.ToLower(value) {
		if isDNS1123SubdomainChar(char) {
			if (char == '-' || char == '.') && (builder.Len() == 0 || lastWasSeparator) {
				continue
			}
			builder.WriteRune(char)
			lastWasSeparator = char == '-' || char == '.'
			continue
		}
		if builder.Len() == 0 || lastWasSeparator {
			continue
		}
		builder.WriteByte('-')
		lastWasSeparator = true
	}
	safe := strings.Trim(builder.String(), "-.")
	if safe == "" {
		return "nsx-group-" + hashSuffix(value)
	}
	if len(safe) > maxDNS1123SubdomainLength {
		suffix := hashSuffix(value)
		prefixLength := maxDNS1123SubdomainLength - len(suffix) - 1
		safe = strings.Trim(safe[:prefixLength], "-.") + "-" + suffix
	}
	return safe
}

func hashSuffix(value string) string {
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:])[:truncatedNameHashLength]
}

func isDNS1123SubdomainChar(char rune) bool {
	return char == '-' || char == '.' || char >= '0' && char <= '9' || char >= 'a' && char <= 'z'
}
