package core

import (
	"fmt"
	"os"
	"os/exec"
)

// PrintJSONOrJQ emits the value as pretty JSON, or pipes it through jq when a
// filter is set, propagating jq's output and exit status.
func PrintJSONOrJQ(v J, jq *string, noCost bool) error {
	if noCost {
		StripCostJ(&v)
	}
	if jq != nil && *jq != "" {
		child := exec.Command("jq", *jq)
		child.Stdout = os.Stdout
		child.Stderr = os.Stderr
		stdin, err := child.StdinPipe()
		if err != nil {
			return &CLIError{fmt.Sprintf("failed to run jq: %v", err)}
		}
		if err := child.Start(); err != nil {
			return &CLIError{fmt.Sprintf("failed to run jq: %v", err)}
		}
		fmt.Fprintln(stdin, SerializeJ(v))
		stdin.Close()
		if err := child.Wait(); err != nil {
			return &CLIError{"jq failed"}
		}
		return nil
	}
	fmt.Println(SerializeJ(v))
	return nil
}
