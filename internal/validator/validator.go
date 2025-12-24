package validator

import (
	"fmt"

	"github.com/zinrai/savalet/internal/models"
)

// ValidateCommand verifies if the command and arguments are allowed
func ValidateCommand(cmd string, args []string, commandList *models.CommandList) error {
	// Check if command list is nil
	if commandList == nil {
		return fmt.Errorf("command list is not available")
	}

	// Check if command exists in the allowed list
	command := commandList.FindCommand(cmd)
	if command == nil {
		return fmt.Errorf("command not allowed")
	}

	// Check if all arguments are allowed
	for _, arg := range args {
		if !command.IsArgAllowed(arg) {
			return fmt.Errorf("argument not allowed")
		}
	}

	return nil
}
