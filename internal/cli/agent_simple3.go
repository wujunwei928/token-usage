package cli

import (
	"github.com/spf13/cobra"

	"github.com/wujunwei/ccusage-go/internal/adapter/common"
	"github.com/wujunwei/ccusage-go/internal/adapter/goose"
	"github.com/wujunwei/ccusage-go/internal/adapter/kilo"
	"github.com/wujunwei/ccusage-go/internal/adapter/pi"
	"github.com/wujunwei/ccusage-go/internal/core"
)

func init() {
	registerAgentCommand(newPiCommand)
	registerAgentCommand(newGooseCommand)
	registerAgentCommand(newKiloCommand)
}

func newPiCommand() *cobra.Command {
	var piPath string
	run := func(f *sharedFlags, kind pi.ReportKind) error {
		var customPath *string
		if piPath != "" {
			customPath = &piPath
		}
		entries, err := pi.LoadEntries(pi.LoadOptions{Shared: f.shared, CustomPath: customPath, Pricing: agentPricing(f.shared)})
		if err != nil {
			return err
		}
		entries = pi.FilterEntriesByDate(entries, f.shared)
		rows := pi.SummarizeEntries(entries, kind)
		rows = core.SortSummaries(rows, f.shared.Order, pi.SummaryPeriod)
		if core.WantsJSON(f.shared) {
			return core.PrintJSONOrJQ(pi.ReportFromRows(rows, kind), f.shared.JQ, f.shared.NoCost)
		}
		return core.PrintUsageTable("pi-agent Token Usage Report", kind.FirstColumn(), rows, f.shared, false, nil)
	}
	cmd := newSimpleAgentCommand(simpleAgentConfig{
		Use:         "pi",
		Display:     "pi-agent",
		Short:       "Show pi-agent usage commands",
		About:       "Usage reports for pi.",
		RunDaily:    func(f *sharedFlags) error { return run(f, pi.ReportDaily) },
		RunMonthly:  func(f *sharedFlags) error { return run(f, pi.ReportMonthly) },
		RunSession:  func(f *sharedFlags) error { return run(f, pi.ReportSession) },
	})
	cmd.PersistentFlags().StringVar(&piPath, "pi-path", "", "Path to pi agent sessions directory (default: auto-discovery)")
	return cmd
}

func newGooseCommand() *cobra.Command {
	run := func(f *sharedFlags, kind goose.ReportKind) error {
		entries, err := goose.LoadEntries(f.shared, agentPricing(f.shared))
		if err != nil {
			return err
		}
		entries = common.FilterLoadedEntriesByDate(entries, f.shared)
		rows := goose.SummarizeEntries(entries, kind)
		rows = core.SortSummaries(rows, f.shared.Order, goose.SummaryPeriod)
		if core.WantsJSON(f.shared) {
			return core.PrintJSONOrJQ(goose.ReportFromRows(rows, kind), f.shared.JQ, f.shared.NoCost)
		}
		return goose.PrintTableForAgent("Goose", kind, rows, f.shared)
	}
	return newSimpleAgentCommand(simpleAgentConfig{
		Use:         "goose",
		Display:     "Goose",
		Short:       "Show Goose usage commands",
		About:       "Usage reports for goose.",
		RunDaily:    func(f *sharedFlags) error { return run(f, goose.ReportDaily) },
		RunMonthly:  func(f *sharedFlags) error { return run(f, goose.ReportMonthly) },
		RunSession:  func(f *sharedFlags) error { return run(f, goose.ReportSession) },
	})
}

func newKiloCommand() *cobra.Command {
	run := func(f *sharedFlags, kind kilo.ReportKind) error {
		entries, err := kilo.LoadEntries(f.shared, agentPricing(f.shared))
		if err != nil {
			return err
		}
		entries = kilo.FilterEntriesByDate(entries, f.shared)
		rows := kilo.SummarizeEntries(entries, kind)
		rows = core.SortSummaries(rows, f.shared.Order, kilo.SummaryPeriod)
		if core.WantsJSON(f.shared) {
			return core.PrintJSONOrJQ(kilo.ReportFromRows(rows, kind), f.shared.JQ, f.shared.NoCost)
		}
		return core.PrintUsageTable("Kilo Token Usage Report", kind.FirstColumn(), rows, f.shared, false, nil)
	}
	return newSimpleAgentCommand(simpleAgentConfig{
		Use:         "kilo",
		Display:     "Kilo",
		Short:       "Show Kilo usage commands",
		About:       "Usage reports for kilo.",
		RunDaily:    func(f *sharedFlags) error { return run(f, kilo.ReportDaily) },
		RunMonthly:  func(f *sharedFlags) error { return run(f, kilo.ReportMonthly) },
		RunSession:  func(f *sharedFlags) error { return run(f, kilo.ReportSession) },
	})
}
