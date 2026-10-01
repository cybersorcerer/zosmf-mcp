package setup

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// AskSecret reads one secret. The value is never echoed back by us; a
// terminal that echoes input is the caller's environment, not our doing.
func AskSecret(name string, in io.Reader, out io.Writer) (string, error) {
	_, _ = fmt.Fprintf(out, "\n%s\n  Secret. It goes into the environment file, never into the configuration.\n> ", name)
	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && line == "" {
		return "", fmt.Errorf("input ended before %s was answered", name)
	}
	return strings.TrimSpace(line), nil
}
