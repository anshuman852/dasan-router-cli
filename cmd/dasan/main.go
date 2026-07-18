package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/anshuman852/dasan/internal/cli"
	"github.com/anshuman852/dasan/internal/client"
	"github.com/anshuman852/dasan/internal/exporter"
)

// Set by GoReleaser via ldflags at build time.
var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func envDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	var host, username, password string
	var verbose bool

	rootCmd := &cobra.Command{
		Use:   "dasan",
		Short: "CLI for the Dasan/Airtel H660GM-A GPON router",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Skip login for non-router subcommands (check os.Args since
			// args only contains remaining args after the command path).
			if len(os.Args) > 1 {
				switch os.Args[1] {
				case "help", "version", "completion":
					return nil
				}
			}
			// Create the client
			c := client.NewDasanClient(host)
			c.SetVerbose(verbose)

			// Prompt for credentials if not provided
			user := username
			if user == "" {
				user = envDefault("DASAN_USER", "")
			}
			pass := password
			if pass == "" {
				pass = envDefault("DASAN_PASS", "")
			}

			// For serve command, also check DASAN_USERNAME/DASAN_PASSWORD
			if cmd.Name() == "serve" {
				if user == "" {
					user = envDefault("DASAN_USERNAME", "")
				}
				if pass == "" {
					pass = envDefault("DASAN_PASSWORD", "")
				}
			}

			if user == "" || pass == "" {
				return fmt.Errorf("username and password are required. Set DASAN_USER/DASAN_PASS or use --user/--password")
			}

			if err := c.Login(user, pass); err != nil {
				return fmt.Errorf("login failed: %w", err)
			}

			cli.DasanClient = c
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	rootCmd.PersistentFlags().StringVar(&host, "host", envDefault("DASAN_HOST", "192.168.1.1"), "Router IP/hostname")
	rootCmd.PersistentFlags().StringVarP(&username, "user", "u", "", "Router login username (env: DASAN_USER)")
	rootCmd.PersistentFlags().StringVarP(&password, "password", "p", "", "Router login password (env: DASAN_PASS)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Print HTTP requests to stderr")

	// ---- Subcommands ----

	// status
	rootCmd.AddCommand(cli.NewStatusCmd())

	// wifi
	rootCmd.AddCommand(cli.NewWifiCmd())

	// firewall
	rootCmd.AddCommand(cli.NewFirewallCmd())

	// maintenance
	rootCmd.AddCommand(cli.NewMaintenanceCmd())

	// advanced
	rootCmd.AddCommand(cli.NewAdvancedCmd())

	// reboot
	rootCmd.AddCommand(newRebootCmd())

	// raw
	rootCmd.AddCommand(newRawCmd())

	// serve (exporter)
	rootCmd.AddCommand(newServeCmd())

	// version
	rootCmd.AddCommand(newVersionCmd())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func newRebootCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "reboot",
		Short: "Reboot the router",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !yes {
				fmt.Printf("Reboot the router at %s? This will drop your connection. [y/N]: ", cli.DasanClient.GetHost())
				var response string
				fmt.Scanln(&response)
				if response != "y" && response != "Y" && response != "yes" {
					fmt.Println("Aborted.")
					return nil
				}
			}
			_, err := cli.DasanClient.Cmd("Reboot", map[string]any{"rebootReason": "reboot"})
			if err != nil {
				return err
			}
			fmt.Println("Reboot command sent. The router is restarting.")
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip confirmation")
	return cmd
}

func newRawCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "raw <get|post|delete> <object> [--page <page>] [--data <json>]",
		Short: "Escape hatch: call any objs/cmd endpoint directly",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			method := args[0]
			objs := args[1]

			page, _ := cmd.Flags().GetString("page")
			dataStr, _ := cmd.Flags().GetString("data")
			namespace, _ := cmd.Flags().GetString("namespace")

			switch method {
			case "get":
				result, err := cli.DasanClient.GetNS(objs, page, namespace)
				if err != nil {
					return err
				}
				out, _ := json.MarshalIndent(result, "", "  ")
				fmt.Println(string(out))
			case "post":
				var data any
				if dataStr != "" {
					json.Unmarshal([]byte(dataStr), &data)
				}
				return cli.DasanClient.Post(objs, page, data)
			case "delete":
				var data any
				if dataStr != "" {
					json.Unmarshal([]byte(dataStr), &data)
				}
				return cli.DasanClient.Delete(objs, page, data)
			default:
				return fmt.Errorf("unknown method: %s (use get, post, or delete)", method)
			}
			return nil
		},
	}
	cmd.Flags().StringP("page", "", "", "page id for permission check")
	cmd.Flags().StringP("data", "d", "", "JSON body for post/delete")
	cmd.Flags().StringP("namespace", "n", "tr98", "tr98, sys, or bin")
	return cmd
}

func newServeCmd() *cobra.Command {
	var port, interval int
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the Prometheus metrics exporter",
		RunE: func(cmd *cobra.Command, args []string) error {
			exporter.Serve(cli.DasanClient, port, interval)
			return nil
		},
	}
	cmd.Flags().IntVar(&port, "port", 9800, "Exporter HTTP listen port")
	cmd.Flags().IntVar(&interval, "interval", 60, "Scrape interval in seconds")
	return cmd
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("dasan %s (commit %s, built %s)\n", version, commit, date)
		},
	}
}
