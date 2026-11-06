package cmd

import (
	"fmt"
)

// ValidateCommandLine returns an npm-style message for an unknown top-level
// command. An empty string means the line is known and can go to docopt.
func ValidateCommandLine(argv []string) string {
	if len(argv) == 0 || len(argv[0]) == 0 || argv[0][0] == '-' {
		return ""
	}
	if !isKnownCommand(argv[0]) {
		return unknownCommandMessage(argv[0], "commands")
	}
	return ""
}

func isKnownCommand(name string) bool {
	for _, command := range completionCommands {
		if command == name {
			return true
		}
	}
	return false
}

func unknownCommandMessage(command, kind string) string {
	return fmt.Sprintf("Unknown command: %q\n\nTo see a list of supported %s, run:\n  cf --help", command, kind)
}
