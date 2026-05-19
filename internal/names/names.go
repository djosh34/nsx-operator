package names

import (
	"errors"
	"net/url"
	"strings"
)

type NSXGroupLogicalID struct {
	NetworkCloudFQDN string
	GroupID          string
}

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

func NSXGroupName(id NSXGroupLogicalID) string {
	cloud := NormalizeNetworkCloudFQDN(id.NetworkCloudFQDN)
	cloud = strings.ReplaceAll(cloud, ":", "-")
	groupID := strings.TrimSpace(id.GroupID)
	return cloud + "--" + groupID
}

func ParseNSXGroupName(value string) (NSXGroupLogicalID, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return NSXGroupLogicalID{}, errors.New("nsx group name is empty")
	}
	if strings.Count(trimmed, "--") != 1 {
		return NSXGroupLogicalID{}, errors.New("nsx group name must contain exactly one -- separator")
	}
	parts := strings.Split(trimmed, "--")
	cloud := strings.TrimSpace(parts[0])
	groupID := strings.TrimSpace(parts[1])
	if cloud == "" {
		return NSXGroupLogicalID{}, errors.New("nsx group name cloud segment is empty")
	}
	if groupID == "" {
		return NSXGroupLogicalID{}, errors.New("nsx group name group segment is empty")
	}
	return NSXGroupLogicalID{
		NetworkCloudFQDN: NormalizeNetworkCloudFQDN(restorePort(cloud)),
		GroupID:          groupID,
	}, nil
}

func restorePort(cloud string) string {
	portSeparator := strings.LastIndex(cloud, "-")
	if portSeparator <= 0 || portSeparator == len(cloud)-1 {
		return cloud
	}
	port := cloud[portSeparator+1:]
	for _, char := range port {
		if char < '0' || char > '9' {
			return cloud
		}
	}
	return cloud[:portSeparator] + ":" + port
}
