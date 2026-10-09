package core

import "testing"

func TestServerValidate(t *testing.T) {
	tests := []struct {
		name    string
		server  Server
		wantErr bool
	}{
		{
			name: "valid server",
			server: Server{
				ID:        "aim",
				Name:      "CS2 AIM",
				Directory: "/servers/aim",
			},
		},
		{
			name: "missing server ID",
			server: Server{
				Name:      "CS2 AIM",
				Directory: "/servers/aim",
			},
			wantErr: true,
		},
		{
			name: "missing server name",
			server: Server{
				ID:        "aim",
				Directory: "/servers/aim",
			},
			wantErr: true,
		},
		{
			name: "missing server directory",
			server: Server{
				ID:   "aim",
				Name: "CS2 AIM",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.server.Validate()

			if (err != nil) != tt.wantErr {
				t.Fatalf(
					"Validate() error = %v, wantErr = %v",
					err,
					tt.wantErr,
				)
			}
		})
	}
}
