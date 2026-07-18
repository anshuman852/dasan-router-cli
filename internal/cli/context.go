package cli

import (
	"github.com/spf13/cobra"

	"github.com/anshuman852/dasan/internal/client"
)

// DasanClient is the global CLI client instance, set by main.go before
// executing any subcommand.
var DasanClient *client.DasanClient

// getClient returns the global CLI client instance from the command context.
func getClient(cmd *cobra.Command) *client.DasanClient {
	return DasanClient
}
