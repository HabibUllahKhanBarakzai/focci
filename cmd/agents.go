package cmd

import "fmt"

// agentTarget records which agents a command should act on.
type agentTarget struct {
	claude bool
	codex  bool
}

func parseAgentFlag(value string) (agentTarget, error) {
	switch value {
	case "claude":
		return agentTarget{claude: true}, nil
	case "codex":
		return agentTarget{codex: true}, nil
	case "all":
		return agentTarget{claude: true, codex: true}, nil
	default:
		return agentTarget{}, fmt.Errorf("invalid value %q for --agent (expected claude, codex, or all)", value)
	}
}
