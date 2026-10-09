package service

const (
	ApplicationName = "CServer Manager"
	Version         = "0.1.0-dev"
)

// ApplicationInfo contains public application metadata.
type ApplicationInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// GetApplicationInfo returns the current application metadata.
func GetApplicationInfo() ApplicationInfo {
	return ApplicationInfo{
		Name:    ApplicationName,
		Version: Version,
	}
}
