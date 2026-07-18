package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/anshuman852/dasan/internal/client"
)

// ---------------------------------------------------------------------------
// UPnP
// ---------------------------------------------------------------------------

const upnpPage = "FirewallSetupPage-UPnP"

func upnpSet(cl *client.DasanClient, enable bool) error {
	if err := cl.Post("UPnPCfg", upnpPage, map[string]any{"enable": enable}); err != nil {
		return err
	}
	// Verify persistence (silent failure check)
	after, err := cl.Get("UPnPCfg", upnpPage)
	if err != nil {
		return nil
	}
	obj, err := client.ToObj(after)
	if err != nil {
		return nil
	}
	if client.GetBool(obj, "enable") != enable {
		fmt.Fprintf(os.Stderr, "Warning: the router accepted the request but did not change state (known firmware quirk) -- verify in the web UI.\n")
	}
	return nil
}

func newFwUpnpCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "upnp",
		Short: "Show whether UPnP is enabled",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl := getClient(cmd)
			data, err := cl.Get("UPnPCfg", upnpPage)
			if err != nil {
				return err
			}
			obj, err := client.ToObj(data)
			if err != nil {
				return err
			}
			if client.GetBool(obj, "enable") {
				fmt.Println("UPnP: enabled")
			} else {
				fmt.Println("UPnP: disabled")
			}
			return nil
		},
	}
}

func newFwUpnpEnableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "upnp-enable",
		Short: "Enable UPnP (may be silently ignored by firmware)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl := getClient(cmd)
			if err := upnpSet(cl, true); err != nil {
				return err
			}
			fmt.Println("UPnP enable requested")
			return nil
		},
	}
}

func newFwUpnpDisableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "upnp-disable",
		Short: "Disable UPnP (may be silently ignored by firmware)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl := getClient(cmd)
			if err := upnpSet(cl, false); err != nil {
				return err
			}
			fmt.Println("UPnP disable requested")
			return nil
		},
	}
}

// ---------------------------------------------------------------------------
// MAC / IP anti-spoofing
// ---------------------------------------------------------------------------

const masPage = "FirewallSetupPage-MACAntiSpoofing"
const iasPage = "FirewallSetupPage-IPAntiSpoofing"

func newFwMacAntiSpoofingCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mac-anti-spoofing",
		Short: "List configured MAC anti-spoofing rules",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl := getClient(cmd)
			data, err := cl.Get("MacAntiSpoofingCfg", masPage)
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
				state := "inactive"
				if client.GetBool(r, "Active") {
					state = "active"
				}
				rows = append(rows, []string{
					Ftoa(client.GetFloat(r, "Index")),
					client.GetStr(r, "Mac"),
					client.GetStr(r, "PortBind"),
					state,
				})
			}
			PrintTable([]string{"index", "mac", "port-bind", "state"}, rows)
			return nil
		},
	}
}

func newFwMacAntiSpoofingTableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mac-anti-spoofing-table",
		Short: "Show the learned MAC-per-LAN-port binding table",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl := getClient(cmd)
			data, err := cl.Get("MacAntiSpoofingTable", masPage)
			if err != nil {
				return err
			}
			arr, err := client.ToArr(data)
			if err != nil {
				return err
			}
			if len(arr) == 0 {
				fmt.Println("(empty)")
				return nil
			}
			first, ok := arr[0].(map[string]any)
			if !ok {
				return nil
			}
			headers := []string{}
			for k := range first {
				headers = append(headers, k)
			}
			rows := [][]string{}
			for _, item := range arr {
				r, ok := item.(map[string]any)
				if !ok {
					continue
				}
				row := []string{}
				for _, h := range headers {
					row = append(row, fmt.Sprintf("%v", r[h]))
				}
				rows = append(rows, row)
			}
			PrintTable(headers, rows)
			return nil
		},
	}
}

func newFwIpAntiSpoofingCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ip-anti-spoofing",
		Short: "List configured IP anti-spoofing rules",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl := getClient(cmd)
			data, err := cl.Get("IPAntiSpoofingCfg", iasPage)
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
				state := "inactive"
				if client.GetBool(r, "Active") {
					state = "active"
				}
				rows = append(rows, []string{
					Ftoa(client.GetFloat(r, "Index")),
					client.GetStr(r, "IP"),
					client.GetStr(r, "Mask"),
					client.GetStr(r, "PortBind"),
					state,
				})
			}
			PrintTable([]string{"index", "ip", "mask", "port-bind", "state"}, rows)
			return nil
		},
	}
}

func newFwIpAntiSpoofingTableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ip-anti-spoofing-table",
		Short: "Show the learned IP-per-LAN-port binding table",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl := getClient(cmd)
			data, err := cl.Get("IPAntiSpoofingTable", iasPage)
			if err != nil {
				return err
			}
			arr, err := client.ToArr(data)
			if err != nil {
				return err
			}
			if len(arr) == 0 {
				fmt.Println("(empty)")
				return nil
			}
			first, ok := arr[0].(map[string]any)
			if !ok {
				return nil
			}
			headers := []string{}
			for k := range first {
				headers = append(headers, k)
			}
			rows := [][]string{}
			for _, item := range arr {
				r, ok := item.(map[string]any)
				if !ok {
					continue
				}
				row := []string{}
				for _, h := range headers {
					row = append(row, fmt.Sprintf("%v", r[h]))
				}
				rows = append(rows, row)
			}
			PrintTable(headers, rows)
			return nil
		},
	}
}
