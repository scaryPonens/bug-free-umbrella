//go:build legacy_webconsole_emulator
// +build legacy_webconsole_emulator

package webconsole

import (
	"fmt"
	"strings"
)

type ParsedCommand struct {
	Name     string
	Flags    map[string]string
	Position []string
	Raw      string
}

func ParseCommand(line string) (ParsedCommand, error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return ParsedCommand{}, fmt.Errorf("empty command")
	}
	tokens := strings.Fields(line)
	if len(tokens) == 0 {
		return ParsedCommand{}, fmt.Errorf("empty command")
	}

	parsed := ParsedCommand{
		Name:     strings.ToLower(tokens[0]),
		Flags:    make(map[string]string),
		Position: make([]string, 0),
		Raw:      line,
	}

	for i := 1; i < len(tokens); i++ {
		tok := strings.TrimSpace(tokens[i])
		if strings.HasPrefix(tok, "--") {
			withoutPrefix := strings.TrimPrefix(tok, "--")
			if withoutPrefix == "" {
				continue
			}
			if strings.Contains(withoutPrefix, "=") {
				parts := strings.SplitN(withoutPrefix, "=", 2)
				parsed.Flags[strings.ToLower(parts[0])] = strings.TrimSpace(parts[1])
				continue
			}
			key := strings.ToLower(withoutPrefix)
			value := "true"
			if i+1 < len(tokens) && !strings.HasPrefix(tokens[i+1], "--") {
				i++
				value = strings.TrimSpace(tokens[i])
			}
			parsed.Flags[key] = value
			continue
		}
		parsed.Position = append(parsed.Position, tok)
	}

	return parsed, nil
}
