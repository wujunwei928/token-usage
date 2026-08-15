package cli

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/wujunwei928/token-usage/internal/adapter/all"
	"github.com/wujunwei928/token-usage/internal/core"
)

// allFlags holds the unified-report flag set (root and top-level reports).
type allFlags struct {
	shared      *sharedFlags
	allAccepted bool
	sections    string
	byAgent     bool
}

func registerAllFlags(cmd *cobra.Command) *allFlags {
	f := &allFlags{shared: registerSharedFlags(cmd)}
	flags := cmd.Flags()
	flags.BoolVar(&f.allAccepted, "all", false,
		"Accepted for compatibility; all detected supported agents are included by default (default: false)")
	flags.StringVar(&f.sections, "sections", "",
		"Emit multiple unified report sections from one load (daily, weekly, monthly, session)")
	flags.BoolVar(&f.byAgent, "by-agent", false,
		"Include per-agent JSON breakdowns in unified report rows (default: false)")
	return f
}

func newAllReportCommand(kind all.ReportKind) *cobra.Command {
	meta := map[all.ReportKind][2]string{
		all.KindDaily:   {"daily", "Show all detected coding (agent) CLI usage grouped by date"},
		all.KindWeekly:  {"weekly", "Show all detected coding (agent) CLI usage grouped by week"},
		all.KindMonthly: {"monthly", "Show all detected coding (agent) CLI usage grouped by month"},
		all.KindSession: {"session", "Show all detected coding (agent) CLI usage grouped by session"},
	}[kind]
	cmd := &cobra.Command{Use: meta[0], Short: meta[1]}
	f := registerAllFlags(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runAllReport(kind, f)
	}
	return cmd
}

func runAllReport(kind all.ReportKind, f *allFlags) error {
	if err := f.shared.resolve(); err != nil {
		return err
	}
	lastSupported := kind != all.KindSession
	if err := f.shared.validateLast(lastSupported); err != nil {
		return err
	}
	if f.sections != "" && f.shared.shared.Last != nil {
		return parseErr("The --last option cannot be used with --sections.")
	}
	var sections []all.ReportKind
	if f.sections != "" {
		seen := map[all.ReportKind]bool{}
		for _, token := range splitAndTrim(f.sections) {
			if token == "" {
				continue
			}
			sectionKind, ok := all.ParseReportKind(token)
			if !ok {
				return parseErr("Invalid --sections value '%s'. Expected one or more of: daily, weekly, monthly, session.", token)
			}
			if !seen[sectionKind] {
				seen[sectionKind] = true
				sections = append(sections, sectionKind)
			}
		}
		if len(sections) == 0 {
			return parseErr("Invalid --sections value '%s'. Expected one or more of: daily, weekly, monthly, session.", f.sections)
		}
	}

	unit := core.PeriodDay
	switch kind {
	case all.KindWeekly:
		unit = core.PeriodWeek
	case all.KindMonthly:
		unit = core.PeriodMonth
	}
	f.shared.resolveLastSince(unit, core.Sunday)

	specs := all.BuiltInSpecs(f.shared.shared)
	if sections != nil {
		requested := []all.ReportKind{kind}
		for _, candidate := range []all.ReportKind{all.KindDaily, all.KindWeekly, all.KindMonthly, all.KindSession} {
			if candidate != kind && containsKind(sections, candidate) {
				requested = append(requested, candidate)
			}
		}
		return runAllSections(requested, kind, f, specs)
	}

	loadKind := all.KindDaily
	if kind == all.KindSession {
		loadKind = all.KindSession
	}
	base, err := all.LoadBaseRows(loadKind, f.shared.shared, specs)
	if err != nil {
		return err
	}
	rows := all.FinishRows(kind, base.Rows, f.shared.shared)
	if core.WantsJSON(f.shared.shared) {
		return core.PrintJSONOrJQ(all.ReportJSON(rows, kind, f.byAgent), f.shared.shared.JQ, f.shared.shared.NoCost)
	}
	return all.PrintTable(rows, kind, f.shared.shared, base.DetectedAgents)
}

func runAllSections(requested []all.ReportKind, kind all.ReportKind, f *allFlags, specs []all.Spec) error {
	shared := f.shared.shared
	var dailyBase, sessionBase *all.LoadResult
	needsDaily := false
	needsSession := false
	for _, sectionKind := range requested {
		if sectionKind == all.KindSession {
			needsSession = true
		} else {
			needsDaily = true
		}
	}
	if needsDaily {
		base, err := all.LoadBaseRows(all.KindDaily, shared, specs)
		if err != nil {
			return err
		}
		dailyBase = base
	}
	if needsSession {
		base, err := all.LoadBaseRows(all.KindSession, shared, specs)
		if err != nil {
			return err
		}
		sessionBase = base
	}
	detectedFor := func(sectionKind all.ReportKind) []string {
		if sectionKind == all.KindSession {
			if sessionBase != nil {
				return sessionBase.DetectedAgents
			}
			return nil
		}
		if dailyBase != nil {
			return dailyBase.DetectedAgents
		}
		return nil
	}
	type section = all.Section
	var out []section
	for _, sectionKind := range requested {
		base := dailyBase
		if sectionKind == all.KindSession {
			base = sessionBase
		}
		var rows []all.Row
		if base != nil {
			rows = all.FinishRows(sectionKind, append([]all.Row(nil), base.Rows...), shared)
		}
		out = append(out, section{Kind: sectionKind, Rows: rows})
	}
	if core.WantsJSON(shared) {
		return core.PrintJSONOrJQ(all.SectionsReportJSON(out, kind, f.byAgent), shared.JQ, shared.NoCost)
	}
	for _, s := range out {
		if err := all.PrintTable(s.Rows, s.Kind, shared, detectedFor(s.Kind)); err != nil {
			return err
		}
	}
	return nil
}

func containsKind(list []all.ReportKind, want all.ReportKind) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}

func splitAndTrim(value string) []string {
	var out []string
	for _, item := range strings.Split(value, ",") {
		out = append(out, strings.TrimSpace(item))
	}
	return out
}
