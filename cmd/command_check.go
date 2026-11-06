package cmd

import (
	"fmt"
	"strings"
)

// ValidateCommandLine returns an npm-style message for an unknown top-level
// command or an unsupported "cf timer" subcommand. An empty string means the
// line is known and can go to docopt.
func ValidateCommandLine(argv []string) string {
	if len(argv) == 0 || strings.HasPrefix(argv[0], "-") {
		return ""
	}
	command := argv[0]
	if command == "counttime" || command == "countTime" {
		command = "timer"
	}
	if !isKnownCommand(command) {
		return unknownCommandMessage(command, "commands")
	}
	if command != "timer" {
		return ""
	}
	if len(argv) == 1 {
		return "A timer command is required: start or end.\n\nTo see the supported timer commands, run:\n  cf --help"
	}
	if strings.HasPrefix(argv[1], "-") {
		return ""
	}
	switch normalizeTimerCommand(argv[1]) {
	case "start", "end":
		return ""
	}
	return unknownCommandMessage(argv[1], "timer commands")
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
