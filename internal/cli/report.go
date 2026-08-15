package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/wujunwei/ccusage-go/internal/core"
	"github.com/wujunwei/ccusage-go/internal/report"
)

// Exit codes for scripts: 0 ok, 2 configuration error, 3 network failure,
// 4 server rejection.
const (
	reportExitConfig = 2
	reportExitNet    = 3
	reportExitServer = 4
)

func newReportCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Report today's token usage to the leaderboard server",
		Long: "Aggregate today's usage across all detected agents (local timezone),\n" +
			"then upload the hourly report snapshot. Re-running replaces the day's data\n" +
			"for this device (latest report wins).",
		RunE:         runReport,
		SilenceUsage: true,
	}
	flags := cmd.Flags()
	flags.String("server", "", "leaderboard server address (flag > CCUSAGE_REPORT_SERVER > ccusage.json reportServer)")
	flags.String("token", "", "user report token (flag > CCUSAGE_REPORT_TOKEN > ccusage.json reportToken)")
	flags.String("timezone", "", "timezone for the day/hour split (default: system local)")
	flags.String("since", "", "backfill mode: report every day from YYYY-MM-DD to today in one request")
	flags.String("device-file", "", "device identity file (default: CCUSAGE_DEVICE_FILE or <config>/ccusage/device.json)")
	flags.Bool("dry-run", false, "build and print the snapshot without sending")
	flags.Bool("install-timer", false, "install an hourly crontab entry running ccusage report (idempotent)")
	flags.Bool("uninstall-timer", false, "remove the crontab entry installed by --install-timer")
	flags.Bool("quiet", false, "print only the summary line")
	return cmd
}

// ReportError carries the report command's exit code through cobra's error
// path; cmd/ccusage maps it onto os.Exit.
type ReportError struct {
	ExitCode int
	Err      error
}

func (e *ReportError) Error() string { return e.Err.Error() }

func runReport(cmd *cobra.Command, args []string) error {
	flags := cmd.Flags()
	if install, _ := flags.GetBool("install-timer"); install {
		return runTimerInstall()
	}
	if uninstall, _ := flags.GetBool("uninstall-timer"); uninstall {
		return runTimerUninstall()
	}

	flagServer, _ := flags.GetString("server")
	flagToken, _ := flags.GetString("token")
	dryRun, _ := flags.GetBool("dry-run")
	quiet, _ := flags.GetBool("quiet")

	devicePath, _ := flags.GetString("device-file")
	if devicePath == "" {
		var err error
		devicePath, err = report.DefaultDevicePath()
		if err != nil {
			return &ReportError{reportExitConfig, fmt.Errorf("cannot locate device file: %w", err)}
		}
	}
	device, err := report.LoadDevice(devicePath)
	if err != nil {
		return &ReportError{reportExitConfig, fmt.Errorf("cannot create device identity at %s: %w", devicePath, err)}
	}

	var tzFlag *string
	if v, _ := flags.GetString("timezone"); v != "" {
		tzFlag = &v
	}
	shared := &core.SharedArgs{Mode: core.ModeDisplay, Offline: true, Timezone: tzFlag}

	if since, _ := flags.GetString("since"); since != "" {
		return runBackfill(cmd, shared, device, since, flagServer, flagToken, dryRun, quiet)
	}

	snapshot := report.BuildSnapshot(shared, device.DeviceID, device.Label, time.Now())

	if dryRun {
		pretty, _ := json.MarshalIndent(snapshot, "", "  ")
		fmt.Fprintln(cmd.OutOrStdout(), string(pretty))
		return nil
	}

	server, token, err := report.ResolveEndpoint(os.Args[1:], flagServer, flagToken)
	if err != nil {
		return &ReportError{reportExitConfig, adviseConfig(err)}
	}
	result, err := report.Send(server, token, snapshot, 15*time.Second)
	if err != nil {
		return sendError(err)
	}

	out := cmd.OutOrStdout()
	if quiet {
		fmt.Fprintf(out, "report accepted: %s tokens on %s (%d devices)\n",
			result.DayTokens, result.DayDate, result.DeviceCount)
		return nil
	}
	fmt.Fprintf(out, "✓ 上报成功 %s\n", server)
	fmt.Fprintf(out, "  设备: %s (%s)\n", device.Label, device.DeviceID)
	fmt.Fprintf(out, "  日期: %s (%s, %d 个小时×工具×模型聚合)\n",
		snapshot.Date, snapshot.Timezone, len(snapshot.Hours))
	fmt.Fprintf(out, "  当日 token: %s\n", result.DayTokens)
	fmt.Fprintf(out, "  已绑定设备: %d/3\n", result.DeviceCount)
	return nil
}

// runBackfill reports every day since the given date in one request.
func runBackfill(cmd *cobra.Command, shared *core.SharedArgs, device *report.DeviceFile, since, flagServer, flagToken string, dryRun, quiet bool) error {
	if _, err := time.Parse("2006-01-02", since); err != nil {
		return &ReportError{reportExitConfig, fmt.Errorf("--since must be YYYY-MM-DD (got %q)", since)}
	}
	snapshots := report.BuildBackfill(shared, device.DeviceID, device.Label, since, time.Now())
	if dryRun {
		pretty, _ := json.MarshalIndent(snapshots, "", "  ")
		fmt.Fprintln(cmd.OutOrStdout(), string(pretty))
		return nil
	}
	server, token, err := report.ResolveEndpoint(os.Args[1:], flagServer, flagToken)
	if err != nil {
		return &ReportError{reportExitConfig, adviseConfig(err)}
	}
	result, err := report.SendMany(server, token, snapshots, 120*time.Second)
	if err != nil {
		return sendError(err)
	}
	var total uint64
	for _, s := range snapshots {
		total += s.TotalTokens()
	}
	out := cmd.OutOrStdout()
	if quiet {
		fmt.Fprintf(out, "backfill accepted: %s tokens, %d days\n", result.DayTokens, result.Days)
		return nil
	}
	fmt.Fprintf(out, "✓ 回溯上报成功 %s\n", server)
	fmt.Fprintf(out, "  设备: %s (%s)\n", device.Label, device.DeviceID)
	fmt.Fprintf(out, "  窗口: %s → 今天, 有数据的天: %d\n", since, result.Days)
	fmt.Fprintf(out, "  窗口 token: %s\n", strconv.FormatUint(total, 10))
	fmt.Fprintf(out, "  已绑定设备: %d/3\n", result.DeviceCount)
	return nil
}

// sendError maps client failures onto exit codes and advice.
func sendError(err error) error {
	var clientErr *report.ClientError
	if errors.As(err, &clientErr) {
		if clientErr.Kind == "network" {
			return &ReportError{reportExitNet, clientErr}
		}
		return &ReportError{reportExitServer, adviseServer(clientErr)}
	}
	return &ReportError{reportExitNet, err}
}

func adviseConfig(err error) error {
	switch {
	case errors.Is(err, report.ErrNoServer):
		return fmt.Errorf("%w\n  配置方法任选其一:\n  1) ccusage report --server <地址> --token <token>\n  2) 环境变量 CCUSAGE_REPORT_SERVER / CCUSAGE_REPORT_TOKEN\n  3) ccusage.json 里加 {\"reportServer\": ..., \"reportToken\": ...}", err)
	case errors.Is(err, report.ErrNoToken):
		return fmt.Errorf("%w\n  在网页端 登录 → 设置 → 生成 report token,再配置到客户端。", err)
	}
	return err
}

func adviseServer(err *report.ClientError) error {
	switch err.Code {
	case "invalid_token":
		return fmt.Errorf("%w\n  token 已失效或被吊销:去网页端 设置 页重新生成。", err)
	case "device_limit":
		return fmt.Errorf("%w\n  如需更换设备,联系管理员解绑,或复用已有设备。", err)
	}
	return err
}

func runTimerInstall() error {
	installed, err := report.InstallTimer()
	if err != nil {
		if errors.Is(err, report.ErrNoCrontab) {
			return &ReportError{reportExitConfig, err}
		}
		return &ReportError{reportExitNet, err}
	}
	if installed {
		fmt.Println("✓ 已安装每小时自动上报的 crontab 条目(日志见 /tmp/ccusage-report.log)")
	} else {
		fmt.Println("已存在自动上报条目,无需重复安装")
	}
	return nil
}

func runTimerUninstall() error {
	removed, err := report.UninstallTimer()
	if err != nil {
		return &ReportError{reportExitNet, err}
	}
	if removed {
		fmt.Println("✓ 已移除自动上报条目")
	} else {
		fmt.Println("没有已安装的自动上报条目")
	}
	return nil
}
