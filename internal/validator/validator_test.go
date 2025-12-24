package validator

import (
	"testing"

	"github.com/zinrai/savalet/internal/models"
)

func TestValidateCommand(t *testing.T) {
	// Setup test command list
	commandList := &models.CommandList{
		Commands: []models.Command{
			{
				Name:        "ls",
				Description: "List directory contents",
				AllowedArgs: []string{"-l", "-a", "/tmp", "/var/log"},
			},
			{
				Name:        "echo",
				Description: "Display text",
				AllowedArgs: []string{"hello", "world"},
			},
			{
				Name:        "uptime",
				Description: "Show system uptime",
				AllowedArgs: []string{}, // No arguments allowed
			},
		},
	}

	tests := []struct {
		name        string
		cmd         string
		args        []string
		commandList *models.CommandList
		wantErr     bool
		errContains string
	}{
		// Allowed commands and arguments
		{
			name:        "allowed command with allowed args",
			cmd:         "ls",
			args:        []string{"-l", "/tmp"},
			commandList: commandList,
			wantErr:     false,
		},
		{
			name:        "allowed command with single allowed arg",
			cmd:         "echo",
			args:        []string{"hello"},
			commandList: commandList,
			wantErr:     false,
		},
		{
			name:        "allowed command with no args",
			cmd:         "uptime",
			args:        []string{},
			commandList: commandList,
			wantErr:     false,
		},
		{
			name:        "allowed command with empty args slice",
			cmd:         "uptime",
			args:        nil,
			commandList: commandList,
			wantErr:     false,
		},

		// Disallowed commands
		{
			name:        "disallowed command",
			cmd:         "rm",
			args:        []string{"-rf", "/"},
			commandList: commandList,
			wantErr:     true,
			errContains: "command not allowed",
		},
		{
			name:        "empty command name",
			cmd:         "",
			args:        []string{},
			commandList: commandList,
			wantErr:     true,
			errContains: "command not allowed",
		},

		// Disallowed arguments
		{
			name:        "allowed command with disallowed arg",
			cmd:         "ls",
			args:        []string{"-R"}, // -R not in allowed list
			commandList: commandList,
			wantErr:     true,
			errContains: "argument not allowed",
		},
		{
			name:        "allowed command with mixed allowed and disallowed args",
			cmd:         "ls",
			args:        []string{"-l", "-R"}, // -l is allowed, -R is not
			commandList: commandList,
			wantErr:     true,
			errContains: "argument not allowed",
		},
		{
			name:        "allowed command with disallowed path",
			cmd:         "ls",
			args:        []string{"/etc"}, // /etc not in allowed list
			commandList: commandList,
			wantErr:     true,
			errContains: "argument not allowed",
		},
		{
			name:        "command that allows no args but args provided",
			cmd:         "uptime",
			args:        []string{"-p"},
			commandList: commandList,
			wantErr:     true,
			errContains: "argument not allowed",
		},

		// Edge cases
		{
			name:        "nil command list",
			cmd:         "ls",
			args:        []string{"-l"},
			commandList: nil,
			wantErr:     true,
			errContains: "command list is not available",
		},
		{
			name:        "empty command list",
			cmd:         "ls",
			args:        []string{"-l"},
			commandList: &models.CommandList{Commands: []models.Command{}},
			wantErr:     true,
			errContains: "command not allowed",
		},
		{
			name:        "case sensitive command name",
			cmd:         "LS", // uppercase
			args:        []string{"-l"},
			commandList: commandList,
			wantErr:     true,
			errContains: "command not allowed",
		},
		{
			name:        "case sensitive argument",
			cmd:         "echo",
			args:        []string{"Hello"}, // capital H
			commandList: commandList,
			wantErr:     true,
			errContains: "argument not allowed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCommand(tt.cmd, tt.args, tt.commandList)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidateCommand() expected error but got nil")
					return
				}
				if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("ValidateCommand() error = %v, want error containing %v", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("ValidateCommand() unexpected error = %v", err)
				}
			}
		})
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[:len(substr)] == substr ||
		len(s) >= len(substr) && contains(s[1:], substr)
}
