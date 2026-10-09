package core

// RuntimeInspectionState explicitly separates configuration evidence from runtime state.
type RuntimeInspectionState string

const RuntimeNotChecked RuntimeInspectionState = "not_checked"

// OverlayFSMetadata describes paths found on disk or declared in a mount unit.
// Directory existence and mount activation are different facts.
type OverlayFSMetadata struct {
	LowerDirectories []string               `json:"lower_directories,omitempty"`
	UpperDirectory   string                 `json:"upper_directory,omitempty"`
	WorkDirectory    string                 `json:"work_directory,omitempty"`
	MergedDirectory  string                 `json:"merged_directory,omitempty"`
	Source           string                 `json:"source"`
	RuntimeState     RuntimeInspectionState `json:"runtime_state"`
}

// SystemdMountMetadata describes a recognized overlay .mount configuration.
// It does not report whether the unit is enabled, active or loaded.
type SystemdMountMetadata struct {
	UnitName     string                 `json:"unit_name"`
	UnitFile     string                 `json:"unit_file"`
	Where        string                 `json:"where"`
	RuntimeState RuntimeInspectionState `json:"runtime_state"`
}

// InfrastructureMetadata contains optional read-only configuration evidence.
// It may be exposed on a discovered server without implying runtime activity.
type InfrastructureMetadata struct {
	OverlayFS *OverlayFSMetadata    `json:"overlayfs,omitempty"`
	Systemd   *SystemdMountMetadata `json:"systemd,omitempty"`
	Warnings  []DiscoveryWarning    `json:"warnings,omitempty"`
}
