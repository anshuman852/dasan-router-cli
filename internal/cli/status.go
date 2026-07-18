package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/anshuman852/dasan/internal/client"
)

// NewStatusCmd creates the `dasan status` command tree.
func NewStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Device status, WAN/LAN, and connected clients",
	}

	cmd.AddCommand(
		newStatusInfoCmd(),
		newStatusWanCmd(),
		newStatusLanCmd(),
		newStatusClientsCmd(),
	)

	return cmd
}

func newStatusInfoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "info",
		Short: "Device info, uptime, CPU/mem/temp, PON optical stats",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl := getClient(cmd)
			info, err := cl.Get("DeviceInfo", "")
			if err != nil {
				return err
			}
			pon, err := cl.Get("PonPortStatus", "")
			if err != nil {
				return err
			}

			obj, _ := client.ToObj(info)
			ponObj, _ := client.ToObj(pon)

			fmt.Printf("Model:         %s %s\n", client.GetStr(obj, "manufacturer"), client.GetStr(obj, "modelName"))
			fmt.Printf("Serial:        %s\n", client.GetStr(obj, "serialNumber"))
			fmt.Printf("HW/SW ver:     %s / %s\n", client.GetStr(obj, "hardwareVersion"), client.GetStr(obj, "softwareVersion"))
			uptime := client.GetFloat(obj, "UpTime")
			fmt.Printf("Uptime:        %.0fs (%dh%dm)\n", uptime, int(uptime)/3600, int(uptime)%3600/60)
			fmt.Printf("CPU / Mem:     %.0f%% / %.0f%%\n", client.GetFloat(obj, "CPU_Load"), client.GetFloat(obj, "MemoryUsage"))
			fmt.Printf("Temperature:   %.0f C\n", client.GetFloat(obj, "Temperature"))
			fmt.Printf("MAC:           %s\n", client.GetStr(obj, "MACAddress"))
			fmt.Println()
			fmt.Printf("PON:           %s link %s (uptime %.0fs), OLT %s\n",
				client.GetStr(ponObj, "ponMode"), client.GetStr(ponObj, "ponLinkState"),
				client.GetFloat(ponObj, "ponLinkUptime"), client.GetStr(ponObj, "oltType"))
			fmt.Printf("Optical:       Rx %s dBm / Tx %s dBm, temp %s C, FEC %s\n",
				client.GetStr(ponObj, "ponRxPower"), client.GetStr(ponObj, "ponTxPower"),
				client.GetStr(ponObj, "ponTemp"), client.GetStr(ponObj, "fecStatus"))
			return nil
		},
	}
}

func newStatusWanCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "wan",
		Short: "WAN connection summary (internet/voice VLANs, IPs, status)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl := getClient(cmd)
			data, err := cl.Get("WANIPConnection", "")
			if err != nil {
				return err
			}
			arr, err := client.ToArr(data)
			if err != nil {
				return err
			}
			rows := [][]string{}
			for _, item := range arr {
				c, ok := item.(map[string]any)
				if !ok {
					continue
				}
				if !client.GetBool(c, "enable") && client.GetStr(c, "connectionType") == "Unconfigured" {
					continue
				}
				ip := client.GetStr(c, "externalIPAddress")
				if ip == "" {
					ip = "-"
				}
				gw := client.GetStr(c, "defaultGateway")
				if gw == "" {
					gw = "-"
				}
				rows = append(rows, []string{
					Ftoa(client.GetFloat(c, "iid")),
					client.GetStr(c, "ServiceList"),
					client.GetStr(c, "connectionType"),
					client.GetStr(c, "connectionStatus"),
					ip, gw,
					Ftoa(client.GetFloat(c, "VLANId")),
				})
			}
			PrintTable([]string{"iid", "service", "type", "status", "ip", "gateway", "vlan"}, rows)
			return nil
		},
	}
}

func newStatusLanCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "lan",
		Short: "LAN port link status",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl := getClient(cmd)
			data, err := cl.Get("LANPortStatus", "")
			if err != nil {
				return err
			}
			arr, err := client.ToArr(data)
			if err != nil {
				return err
			}
			rows := [][]string{}
			for _, item := range arr {
				p, ok := item.(map[string]any)
				if !ok {
					continue
				}
				rows = append(rows, []string{
					Ftoa(client.GetFloat(p, "iid")),
					client.GetStr(p, "Admin"),
					client.GetStr(p, "Status"),
					client.GetStr(p, "Mode"),
				})
			}
			PrintTable([]string{"port", "admin", "status", "mode"}, rows)
			return nil
		},
	}
}

func newStatusClientsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "clients",
		Short: "DHCP leases / connected devices",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl := getClient(cmd)
			data, err := cl.Get("DhcpLease", "")
			if err != nil {
				return err
			}
			arr, err := client.ToArr(data)
			if err != nil {
				return err
			}
			rows := [][]string{}
			for _, item := range arr {
				l, ok := item.(map[string]any)
				if !ok {
					continue
				}
				rows = append(rows, []string{
					client.GetStr(l, "ClientName"),
					client.GetStr(l, "IP"),
					client.GetStr(l, "MAC"),
					client.GetStr(l, "ExpireTime"),
				})
			}
			PrintTable([]string{"name", "ip", "mac", "lease-expires"}, rows)
			return nil
		},
	}
}
