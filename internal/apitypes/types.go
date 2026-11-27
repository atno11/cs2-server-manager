// Package apitypes defines the public JSON contracts shared by the daemon and clients.
package apitypes

type Definition struct {
	ID         string   `json:"id"`
	Executable string   `json:"executable"`
	WorkDir    string   `json:"work_dir"`
	Port       int      `json:"port"`
	Args       []string `json:"args"`
}

type Instance struct {
	ID              string `json:"id"`
	Port            int    `json:"port"`
	State           string `json:"state"`
	Error           string `json:"error,omitempty"`
	LastTermination string `json:"last_termination,omitempty"`
}

type Snapshot struct {
	Instances []Instance `json:"instances"`
}
type InstanceChange struct {
	Instance Instance `json:"instance"`
}
type Removed struct {
	ID string `json:"id"`
}
type LogEvent struct {
	ID    string `json:"id"`
	Start uint64 `json:"start"`
	End   uint64 `json:"end"`
	Bytes []byte `json:"bytes"`
}
