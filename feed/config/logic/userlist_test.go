package logic

import (
	"testing"
)

func TestBlocklistLogicBlockConfig_ValidateAll(t *testing.T) {
	tests := []struct {
		name    string
		config  *BaseLogicBlockConfig
		wantErr bool
	}{
		{
			name: "Success case: listUri is set",
			config: &BaseLogicBlockConfig{
				Options: map[string]interface{}{
					"listUri": "at://did:plc:xxx/app.bsky.graph.list/xxx",
					"allow":   true,
				},
			},

			wantErr: false,
		},
		{
			name: "invalid listUri",
			config: &BaseLogicBlockConfig{
				Options: map[string]interface{}{
					"listUri": "at://did:plc:xxx/app.bsky.graph.follow/xxx",
					"allow":   true,
				},
			},
			wantErr: true,
		},
		{
			name: "Error case: listUri is not set",
			config: &BaseLogicBlockConfig{
				Options: map[string]interface{}{
					"allow": true,
				},
			},

			wantErr: true,
		},
		{
			name: "Error case: listUri is empty string",
			config: &BaseLogicBlockConfig{
				Options: map[string]interface{}{
					"listUri": "",
					"allow":   true,
				},
			},

			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := (&UserListLogicBlockFactory{}).Create(*tt.config)
			if err != nil {
				t.Fatalf("Create() error = %v", err)
			}
			err = cfg.ValidateAll()
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateAll() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
func TestUserListLogicBlockConfig_Validate(t *testing.T) {
	config, err := (&UserListLogicBlockFactory{}).Create(BaseLogicBlockConfig{
		Options: map[string]interface{}{
			"listUri": "at://did:plc:xxx/app.bsky.graph.list/xxx",
			"allow":   true,
		},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	tests := []struct {
		name    string
		key     string
		value   interface{}
		wantErr bool
	}{
		{
			name:    "Success: valid listUri",
			key:     UserListOptionUri,
			value:   "at://did:plc:xxx/app.bsky.graph.list/xxx",
			wantErr: false,
		},
		{
			name:    "Error: invalid listUri",
			key:     UserListOptionUri,
			value:   "invalid_uri",
			wantErr: true,
		},
		{
			name:    "Error: empty listUri",
			key:     UserListOptionUri,
			value:   "",
			wantErr: true,
		},
		{
			name:    "Success: valid allow",
			key:     UserListOptionAllow,
			value:   true,
			wantErr: false,
		},
		{
			name:    "Error: invalid allow",
			key:     UserListOptionAllow,
			value:   "invalid_allow",
			wantErr: true,
		},
		{
			name:    "Error: empty allow",
			key:     UserListOptionAllow,
			value:   "",
			wantErr: true,
		},
		{
			name:    "Success: valid apiBaseURL",
			key:     UserListOptionApiBaseURL,
			value:   "https://example.com",
			wantErr: false,
		},
		{
			name:    "Error: empty apiBaseURL",
			key:     UserListOptionApiBaseURL,
			value:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := config.Validate(tt.key, tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
