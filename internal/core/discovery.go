package core

// DiscoveryStatus describes filesystem configuration, not runtime health.
type DiscoveryStatus string

const (
	DiscoveryComplete   DiscoveryStatus = "complete"
	DiscoveryIncomplete DiscoveryStatus = "incomplete"
)

// DiscoveryWarning contains a safe, non-secret diagnostic.
type DiscoveryWarning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// DiscoveredServer describes a CS2 installation candidate found on disk.
// Infrastructure is optional configuration evidence, not runtime state.
type DiscoveredServer struct {
	ID              string                  `json:"id"`
	Name            string                  `json:"name"`
	Directory       string                  `json:"directory"`
	Status          DiscoveryStatus         `json:"status"`
	ComposeFile     string                  `json:"compose_file,omitempty"`
	EnvFile         string                  `json:"env_file,omitempty"`
	MergedDirectory string                  `json:"merged_directory,omitempty"`
	Port            *int                    `json:"port,omitempty"`
	Evidence        []string                `json:"evidence"`
	Warnings        []DiscoveryWarning      `json:"warnings,omitempty"`
	Infrastructure  *InfrastructureMetadata `json:"infrastructure,omitempty"`
}
