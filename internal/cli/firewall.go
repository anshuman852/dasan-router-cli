package cli

import "github.com/spf13/cobra"

// NewFirewallCmd creates the `dasan firewall` command tree.
func NewFirewallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "firewall",
		Short: "Firewall/NAT rules: port forwarding, DMZ, filters, UPnP",
	}

	cmd.AddCommand(
		newFwPortForwardingCmd(),
		newFwPortForwardingAddCmd(),
		newFwPortForwardingDeleteCmd(),
		newFwDmzCmd(),
		newFwDmzEnableCmd(),
		newFwDmzDisableCmd(),
		newFwDmzSetHostCmd(),
		newFwPortTriggeringCmd(),
		newFwPortTriggeringAddCmd(),
		newFwPortTriggeringDeleteCmd(),
		newFwUrlFilterCmd(),
		newFwUrlFilterAddCmd(),
		newFwUrlFilterDeleteCmd(),
		newFwParentalControlCmd(),
		newFwParentalControlAddCmd(),
		newFwParentalControlDeleteCmd(),
		newFwUpnpCmd(),
		newFwUpnpEnableCmd(),
		newFwUpnpDisableCmd(),
		newFwMacAntiSpoofingCmd(),
		newFwMacAntiSpoofingTableCmd(),
		newFwIpAntiSpoofingCmd(),
		newFwIpAntiSpoofingTableCmd(),
	)

	return cmd
}
