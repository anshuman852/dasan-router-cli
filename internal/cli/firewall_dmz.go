package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/anshuman852/dasan/internal/client"
)

const dmzPage = "FirewallSetupPage-Dmz"

func dmzRowForWAN(cl *client.DasanClient, wan int) ([]any, string, map[string]any, error) {
	data, err := cl.Get("DmzHostConfig", dmzPage)
	if err != nil {
		return nil, "", nil, err
	}
	arr, err := client.ToArr(data)
	if err != nil {
		return nil, "", nil, err
	}
	intf := fmt.Sprintf("WAN%d", wan)
	var existing map[string]any
	for _, item := range arr {
		r, ok := item.(map[string]any)
		if ok && client.GetStr(r, "intfName") == intf {
			existing = r
			break
		}
	}
	return arr, intf, existing, nil
}

func newFwDmzCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "dmz",
		Short: "Show DMZ host configuration for all WAN interfaces",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl := getClient(cmd)
			data, err := cl.Get("DmzHostConfig", dmzPage)
			if err != nil {
				return err
			}
			arr, err := client.ToArr(data)
			if err != nil {
				return err
			}
			rows := [][]string{}
			for _, item := range arr {
				r, ok := item.(map[string]any)
				if !ok {
					continue
				}
				rows = append(rows, []string{
					client.GetStr(r, "intfName"),
					OnOff(client.GetBool(r, "enable")),
					client.GetStr(r, "IPAddress"),
				})
			}
			PrintTable([]string{"interface", "enabled", "host-ip"}, rows)
			return nil
		},
	}
}

func newFwDmzEnableCmd() *cobra.Command {
	var ip string
	var wan int
	cmd := &cobra.Command{
		Use:   "dmz-enable",
		Short: "Enable DMZ, forwarding all unmatched traffic to the host IP",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl := getClient(cmd)
			_, intf, existing, err := dmzRowForWAN(cl, wan)
			if err != nil {
				return err
			}
			hostIP := ip
			if hostIP == "" && existing != nil {
				hostIP = client.GetStr(existing, "IPAddress")
			}
			if hostIP == "" {
				return fmt.Errorf("no DMZ host IP set for %s yet; pass --ip", intf)
			}
			entry := map[string]any{
				"intfName":  intf,
				"enable":    true,
				"IPAddress": hostIP,
			}
			return cl.Post("DmzHostConfig", dmzPage, []any{entry})
		},
	}
	cmd.Flags().StringVar(&ip, "ip", "", "DMZ host IP")
	cmd.Flags().IntVar(&wan, "wan", 2, "WAN connection iid")
	return cmd
}

func newFwDmzDisableCmd() *cobra.Command {
	var wan int
	cmd := &cobra.Command{
		Use:   "dmz-disable",
		Short: "Disable DMZ for a WAN interface",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl := getClient(cmd)
			_, intf, existing, err := dmzRowForWAN(cl, wan)
			if err != nil {
				return err
			}
			entry := map[string]any{
				"intfName":  intf,
				"enable":    false,
				"IPAddress": "",
			}
			if existing != nil {
				entry["IPAddress"] = client.GetStr(existing, "IPAddress")
			}
			return cl.Post("DmzHostConfig", dmzPage, []any{entry})
		},
	}
	cmd.Flags().IntVar(&wan, "wan", 2, "WAN connection iid")
	return cmd
}

func newFwDmzSetHostCmd() *cobra.Command {
	var wan int
	cmd := &cobra.Command{
		Use:   "dmz-set-host <ip>",
		Short: "Set the DMZ host IP without changing whether DMZ is enabled",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl := getClient(cmd)
			ip := args[0]
			_, intf, existing, err := dmzRowForWAN(cl, wan)
			if err != nil {
				return err
			}
			entry := map[string]any{
				"intfName":  intf,
				"IPAddress": ip,
			}
			if existing != nil {
				entry["enable"] = client.GetBool(existing, "enable")
			} else {
				entry["enable"] = false
			}
			return cl.Post("DmzHostConfig", dmzPage, []any{entry})
		},
	}
	cmd.Flags().IntVar(&wan, "wan", 2, "WAN connection iid")
	return cmd
}
