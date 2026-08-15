// Command server runs the ccusage token-leaderboard server: one binary
// serving the report ingest API and the SSR web pages, backed by SQLite.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/wujunwei928/token-usage/internal/server"
)

func main() {
	log.SetFlags(0)
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "server: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) > 0 {
		switch args[0] {
		case "add-user":
			return addUser(args[1:])
		case "add-token":
			return addToken(args[1:])
		case "dump-pricing":
			return dumpPricing(args[1:])
		case "serve":
			args = args[1:]
		default:
			return fmt.Errorf("unknown subcommand %q (want serve|add-user|add-token|dump-pricing)", args[0])
		}
	}
	return serve(args)
}

func serve(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := fs.String("addr", "127.0.0.1:8787", "listen address")
	dbPath := fs.String("db", "leaderboard.db", "SQLite database path")
	pricingPath := fs.String("pricing", "", "model-prices.json override path")
	fs.Parse(args)

	store, err := server.OpenStore(*dbPath)
	if err != nil {
		return err
	}
	defer store.Close()

	pricing, err := server.LoadPricing(*pricingPath)
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

	fmt.Fprintf(os.Stderr, "token-leaderboard server listening on http://%s (db: %s)\n", *addr, filepath.Base(*dbPath))
	srv := &http.Server{Addr: *addr, Handler: mux}
	return srv.ListenAndServe()
}

func addUser(args []string) error {
	fs := flag.NewFlagSet("add-user", flag.ExitOnError)
	dbPath := fs.String("db", "leaderboard.db", "SQLite database path")
	name := fs.String("name", "", "user name (required)")
	city := fs.String("city", "", "user city")
	password := fs.String("password", "", "optional web-login password")
	fs.Parse(args)
	if *name == "" {
		return fmt.Errorf("--name is required")
	}

	store, err := server.OpenStore(*dbPath)
	if err != nil {
		return err
	}
	defer store.Close()

	passwordHash := ""
	if *password != "" {
		passwordHash, err = server.HashPassword(*password)
		if err != nil {
			return err
		}
	}
	token, err := server.SeedUser(context.Background(), store, *name, passwordHash, *city)
	if err != nil {
		return err
	}
	fmt.Printf("user %q created\nreport token (shown once): %s\n", *name, token)
	return nil
}

func dumpPricing(args []string) error {
	fs := flag.NewFlagSet("dump-pricing", flag.ExitOnError)
	out := fs.String("out", "model-prices.json", "output path")
	pricingPath := fs.String("pricing", "", "model-prices.json override path")
	fs.Parse(args)
	return server.DumpPricing(*pricingPath, *out)
}

func addToken(args []string) error {
	fs := flag.NewFlagSet("add-token", flag.ExitOnError)
	dbPath := fs.String("db", "leaderboard.db", "SQLite database path")
	name := fs.String("name", "", "existing user name (required)")
	label := fs.String("label", "ops", "token label")
	fs.Parse(args)
	if *name == "" {
		return fmt.Errorf("--name is required")
	}

	store, err := server.OpenStore(*dbPath)
	if err != nil {
		return err
	}
	defer store.Close()

	user, err := store.UserByName(context.Background(), *name)
	if err != nil {
		return fmt.Errorf("user %q not found", *name)
	}
	token := server.GenerateToken()
	if err := store.CreateToken(context.Background(), user.ID, *label, server.HashToken(token)); err != nil {
		return err
	}
	fmt.Printf("token for %q (shown once): %s\n", *name, token)
	return nil
}
