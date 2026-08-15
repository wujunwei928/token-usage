// ccusage-go: Go implementation of ccusage, byte-compatible with Rust ccusage v20.0.19.
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/wujunwei/ccusage-go/internal/cli"
	"github.com/wujunwei/ccusage-go/internal/core"
)

func main() {
	core.SetJSONFetcher(cli.FetchJSON)
	if err := cli.Execute(); err != nil {
		var parseErr *cli.ParseError
		if errors.As(err, &parseErr) {
			fmt.Fprintln(os.Stderr, parseErr.Message)
			os.Exit(2)
		}
		var reportErr *cli.ReportError
		if errors.As(err, &reportErr) {
			fmt.Fprintf(os.Stderr, "Error: %v\n", reportErr.Err)
			os.Exit(reportErr.ExitCode)
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
