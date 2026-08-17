package cli

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/wujunwei928/token-usage/internal/server"
)

// newServerCommand builds the `server` group hosting the leaderboard server
// lifecycle (ADR 0011). Bare `token-usage server` prints help: nothing
// listens until `server serve` is spelled out.
func newServerCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "server",
		Short: "Run and manage the token leaderboard server",
		Long: "Serve the token leaderboard from this single binary: report ingest API\n" +
			"plus SSR web pages, backed by SQLite. serve listens on 127.0.0.1 by default;\n" +
			"pass --addr 0.0.0.0:8787 when deploying on a remote host (behind a TLS proxy).",
		SilenceUsage: true,
	}
	cmd.AddCommand(
		newServerServeCommand(),
		newServerAddUserCommand(),
		newServerAddTokenCommand(),
		newServerDumpPricingCommand(),
	)
	return cmd
}

func newServerServeCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the leaderboard server (HTTP ingest + web pages)",
		RunE:  runServerServe,
	}
	flags := cmd.Flags()
	flags.String("addr", "127.0.0.1:8787", "listen address (loopback by default; set 0.0.0.0:8787 when deploying)")
	addDBFlag(cmd)
	flags.String("pricing", "", "model-prices.json override path")
	return cmd
}

func runServerServe(cmd *cobra.Command, args []string) error {
	flags := cmd.Flags()
	addr, _ := flags.GetString("addr")
	pricingPath, _ := flags.GetString("pricing")

	store, dbPath, err := openStoreFromFlags(cmd)
	if err != nil {
		return err
	}
	defer store.Close()

	pricing, err := server.LoadPricing(pricingPath)
	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	server.NewAPI(store, log.New(os.Stderr, "[api] ", 0)).Register(mux)
	web, err := server.NewWeb(store, pricing)
	if err != nil {
		return err
	}
	web.Register(mux)

	fmt.Fprintf(os.Stderr, "token-usage server listening on http://%s (db: %s)\n", addr, filepath.Base(dbPath))
	srv := &http.Server{Addr: addr, Handler: mux}
	return srv.ListenAndServe()
}

func newServerAddUserCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add-user",
		Short: "Create a user and mint a one-time report token",
		RunE:  runServerAddUser,
	}
	addDBFlag(cmd)
	flags := cmd.Flags()
	flags.String("name", "", "user name (required)")
	flags.String("city", "", "user city")
	flags.String("password", "", "optional web-login password")
	return cmd
}

func runServerAddUser(cmd *cobra.Command, args []string) error {
	flags := cmd.Flags()
	name, err := requiredString(cmd, "name")
	if err != nil {
		return err
	}
	city, _ := flags.GetString("city")
	password, _ := flags.GetString("password")

	store, _, err := openStoreFromFlags(cmd)
	if err != nil {
		return err
	}
	defer store.Close()

	passwordHash := ""
	if password != "" {
		passwordHash, err = server.HashPassword(password)
		if err != nil {
			return err
		}
	}
	token, err := server.SeedUser(context.Background(), store, name, passwordHash, city)
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "user %q created\n", name)
	fmt.Fprintf(out, "report token (shown once): %s\n", token)
	return nil
}

func newServerAddTokenCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add-token",
		Short: "Mint an extra report token for an existing user",
		RunE:  runServerAddToken,
	}
	addDBFlag(cmd)
	flags := cmd.Flags()
	flags.String("name", "", "existing user name (required)")
	flags.String("label", "ops", "token label")
	return cmd
}

func runServerAddToken(cmd *cobra.Command, args []string) error {
	flags := cmd.Flags()
	name, err := requiredString(cmd, "name")
	if err != nil {
		return err
	}
	label, _ := flags.GetString("label")

	store, _, err := openStoreFromFlags(cmd)
	if err != nil {
		return err
	}
	defer store.Close()

	user, err := store.UserByName(context.Background(), name)
	if err != nil {
		return fmt.Errorf("user %q not found: %w", name, err)
	}
	token := server.GenerateToken()
	if err := store.CreateToken(context.Background(), user.ID, label, server.HashToken(token)); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "token for %q (shown once): %s\n", name, token)
	return nil
}

func newServerDumpPricingCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dump-pricing",
		Short: "Export the effective model price table as an editable JSON file",
		RunE:  runServerDumpPricing,
	}
	flags := cmd.Flags()
	flags.String("out", "model-prices.json", "output path")
	flags.String("pricing", "", "model-prices.json override path")
	return cmd
}

func runServerDumpPricing(cmd *cobra.Command, args []string) error {
	flags := cmd.Flags()
	outPath, _ := flags.GetString("out")
	pricingPath, _ := flags.GetString("pricing")
	return server.DumpPricing(pricingPath, outPath)
}

// addDBFlag declares the SQLite path flag shared by the store-backed server
// subcommands (serve, add-user, add-token).
func addDBFlag(cmd *cobra.Command) {
	cmd.Flags().String("db", "leaderboard.db", "SQLite database path")
}

// openStoreFromFlags opens the store selected by --db; the resolved path
// comes back alongside because serve's startup banner prints its base name.
func openStoreFromFlags(cmd *cobra.Command) (*server.Store, string, error) {
	dbPath, _ := cmd.Flags().GetString("db")
	store, err := server.OpenStore(dbPath)
	return store, dbPath, err
}

// requiredString reads a string flag, mapping the empty value to a
// "--<name> is required" error.
func requiredString(cmd *cobra.Command, name string) (string, error) {
	v, _ := cmd.Flags().GetString(name)
	if v == "" {
		return "", fmt.Errorf("--%s is required", name)
	}
	return v, nil
}
