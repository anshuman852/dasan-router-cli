package cli

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/anshuman852/dasan/internal/client"
)

// NewAdvancedCmd creates the `dasan advanced` command tree.
func NewAdvancedCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "advanced",
		Short: "Advanced setup: WAN detail, ARP, DDNS, static routing",
	}

	cmd.AddCommand(
		newAdvWanConnectionsCmd(),
		newAdvArpCmd(),
		newAdvArpSetTimeoutCmd(),
		newAdvDdnsCmd(),
		newAdvDdnsSetCmd(),
		newAdvStaticRoutingCmd(),
		newAdvStaticRoutingAddCmd(),
		newAdvStaticRoutingDeleteCmd(),
		newAdvDhcpReservationsCmd(),
		newAdvDhcpReservationAddCmd(),
		newAdvDhcpReservationDeleteCmd(),
	)

	return cmd
}

const arpStatusPage = "StatusPage-ARP"
const arpSettingPage = "AdvancedSetupPage-ARP"
const ddnsPage = "AdvancedSetupPage-DDNS"
const staticRoutingPage = "AdvancedSetupPage-StaticRouting"
const lanSetupPage = "AdvancedSetupPage-LANSetup"

func newAdvWanConnectionsCmd() *cobra.Command {
	var showPassword bool
	cmd := &cobra.Command{
		Use:   "wan-connections",
		Short: "Detailed WAN connection view: WANObject slots plus IP/PPPoE connection data",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl := getClient(cmd)

			// WANObjects
			data, err := cl.Get("WANObject", "")
			if err != nil {
				return err
			}
			arr, err := client.ToArr(data)
			if err != nil {
				return err
			}
			rows := [][]string{}
			for _, item := range arr {
				o, ok := item.(map[string]any)
				if !ok {
					continue
				}
				rows = append(rows, []string{
					Ftoa(client.GetFloat(o, "iid")),
					valOrDash(client.GetStr(o, "ServiceList")),
					client.GetStr(o, "ConnectionType"),
					fmt.Sprintf("%v", o["Active"]),
					fmt.Sprintf("%v", o["IsDefault"]),
				})
			}
			PrintTable([]string{"iid", "service", "type", "active", "default"}, rows)

			// WANIPConnections
			ipData, err := cl.Get("WANIPConnection", "")
			if err != nil {
				return err
			}
			ipArr, err := client.ToArr(ipData)
			if err != nil {
				return err
			}
			rows = [][]string{}
			for _, item := range ipArr {
				c, ok := item.(map[string]any)
				if !ok {
					continue
				}
				ipv6 := client.GetStr(c, "ip6AddrGlobal")
				if ipv6 == "" {
					ipv6 = client.GetStr(c, "ip6AddrLocal")
				}
				if ipv6 == "" {
					ipv6 = "-"
				}
				rows = append(rows, []string{
					Ftoa(client.GetFloat(c, "iid")),
					valOrDash(client.GetStr(c, "ServiceList")),
					client.GetStr(c, "connectionType"),
					fmt.Sprintf("%v", c["enable"]),
					client.GetStr(c, "connectionStatus"),
					valOrDash(client.GetStr(c, "externalIPAddress")),
					valOrDash(client.GetStr(c, "defaultGateway")),
					Ftoa(client.GetFloat(c, "VLANId")),
					valOrDash(client.GetStr(c, "ip6Mode")),
					ipv6,
				})
			}
			PrintTable([]string{"iid", "service", "type", "enable", "status", "ipv4", "gateway", "vlan", "ipv6-mode", "ipv6-addr"}, rows)

			// WANPPPConnections
			pppData, err := cl.Get("WANPPPConnection", "")
			if err != nil {
				return err
			}
			pppArr, err := client.ToArr(pppData)
			if err != nil {
				return err
			}
			rows = [][]string{}
			for _, item := range pppArr {
				c, ok := item.(map[string]any)
				if !ok {
					continue
				}
				user := client.GetStr(c, "username")
				pw := client.GetStr(c, "password")
				if !showPassword {
					user = MaskPassword(user)
					pw = MaskPassword(pw)
				}
				if user == "" {
					user = "-"
				}
				if pw == "" {
					pw = "-"
				}
				rows = append(rows, []string{
					Ftoa(client.GetFloat(c, "iid")),
					valOrDash(client.GetStr(c, "ServiceList")),
					fmt.Sprintf("%v", c["enable"]),
					client.GetStr(c, "connectionStatus"),
					valOrDash(client.GetStr(c, "externalIPAddress")),
					valOrDash(client.GetStr(c, "defaultGateway")),
					Ftoa(client.GetFloat(c, "VLANId")),
					user, pw,
				})
			}
			PrintTable([]string{"iid", "service", "enable", "status", "ipv4", "gateway", "vlan", "username", "password"}, rows)
			return nil
		},
	}
	cmd.Flags().BoolVar(&showPassword, "show-password", false, "reveal PPPoE username/password")
	return cmd
}

func valOrDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func newAdvArpCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "arp",
		Short: "ARP table (IPv4) and ARP timeout setting",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl := getClient(cmd)
			data, err := cl.Get("ARPStatus", arpStatusPage)
			if err != nil {
				return err
			}
			arr, err := client.ToArr(data)
			if err != nil {
				return err
			}
			rows := [][]string{}
			for _, item := range arr {
				e, ok := item.(map[string]any)
				if !ok {
					continue
				}
				rows = append(rows, []string{
					Ftoa(client.GetFloat(e, "Index")),
					client.GetStr(e, "IP"),
					client.GetStr(e, "MAC"),
					client.GetStr(e, "IntfType"),
				})
			}
			PrintTable([]string{"index", "ip", "mac", "interface"}, rows)

			setting, err := cl.Get("ARPSetting", arpSettingPage)
			if err != nil {
				return nil
			}
			s, err := client.ToObj(setting)
			if err != nil {
				return nil
			}
			fmt.Printf("ARP timeout: %.0f s\n", client.GetFloat(s, "timeout"))
			return nil
		},
	}
}

func newAdvArpSetTimeoutCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "arp-set-timeout <timeout>",
		Short: "Set the ARP cache timeout",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl := getClient(cmd)
			timeout, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid timeout: %w", err)
			}
			return cl.Post("ARPSetting", arpSettingPage, map[string]any{"timeout": timeout})
		},
	}
	return cmd
}

func newAdvDdnsCmd() *cobra.Command {
	var showPassword bool
	cmd := &cobra.Command{
		Use:   "ddns",
		Short: "Current DDNS (dynamic DNS) configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl := getClient(cmd)
			data, err := cl.Get("DDnsCfg", ddnsPage)
			if err != nil {
				return err
			}
			arr, err := client.ToArr(data)
			if err != nil {
				return err
			}
			rows := [][]string{}
			for _, item := range arr {
				e, ok := item.(map[string]any)
				if !ok {
					continue
				}
				pw := client.GetStr(e, "Password")
				if !showPassword {
					pw = MaskPassword(pw)
				}
				if pw == "" {
					pw = "-"
				}
				rows = append(rows, []string{
					Ftoa(client.GetFloat(e, "iid")),
					fmt.Sprintf("%v", e["Enable"]),
					client.GetStr(e, "ProviderName"),
					valOrDash(client.GetStr(e, "MyHostName")),
					valOrDash(client.GetStr(e, "Username")),
					pw,
					client.GetStr(e, "Ifname"),
					client.GetStr(e, "Status"),
				})
			}
			PrintTable([]string{"iid", "enabled", "provider", "hostname", "username", "password", "interface", "status"}, rows)
			return nil
		},
	}
	cmd.Flags().BoolVar(&showPassword, "show-password", false, "reveal DDNS password")
	return cmd
}

func newAdvDdnsSetCmd() *cobra.Command {
	var enable, wildcard bool
	var hasEnable, hasWildcard bool
	var provider, hostname, username, password, ifname string
	cmd := &cobra.Command{
		Use:   "ddns-set",
		Short: "Configure DDNS settings (read-modify-write)",
		Long: `Configure DDNS settings (read-modify-write; only touches fields you pass).

Note: this router's DDnsCfg validation rejects an empty MyHostName outright
and silently drops an empty Username -- once set, those two fields can't be
cleared back to blank through this API.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cl := getClient(cmd)
			data, err := cl.Get("DDnsCfg", ddnsPage)
			if err != nil {
				return err
			}
			arr, err := client.ToArr(data)
			if err != nil {
				return err
			}
			if len(arr) == 0 {
				return fmt.Errorf("no DDNS configuration found")
			}
			target, ok := arr[0].(map[string]any)
			if !ok {
				return fmt.Errorf("unexpected DDNS data format")
			}
			if hasEnable {
				target["Enable"] = enable
			}
			if provider != "" {
				target["ProviderName"] = provider
			}
			if hostname != "" {
				target["MyHostName"] = hostname
			}
			if username != "" {
				target["Username"] = username
			}
			if password != "" {
				target["Password"] = password
			}
			if ifname != "" {
				target["Ifname"] = ifname
			}
			if hasWildcard {
				target["Wildcard"] = wildcard
			}
			return cl.Post("DDnsCfg", ddnsPage, []any{target})
		},
	}
	cmd.Flags().BoolVar(&enable, "enable", false, "turn DDNS updates on/off")
	cmd.Flags().BoolVar(&hasEnable, "enable-set", false, "set enable flag")
	cmd.Flags().StringVar(&provider, "provider", "", "DDNS provider domain")
	cmd.Flags().StringVar(&hostname, "hostname", "", "hostname to keep updated")
	cmd.Flags().StringVar(&username, "username", "", "DDNS account username")
	cmd.Flags().StringVar(&password, "password", "", "DDNS account password")
	cmd.Flags().StringVar(&ifname, "interface", "", "WAN interface DDNS tracks")
	cmd.Flags().BoolVar(&wildcard, "wildcard", false, "enable wildcard DNS support")
	cmd.Flags().BoolVar(&hasWildcard, "wildcard-set", false, "set wildcard flag")
	return cmd
}

func newAdvStaticRoutingCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "static-routing",
		Short: "List configured static routes (IPv4 and IPv6)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl := getClient(cmd)

			// IPv4
			v4Data, err := cl.GetAPI("StaticRoutingObject", staticRoutingPage)
			if err != nil {
				return err
			}
			v4Arr, v4Err := client.ToArr(v4Data)
			if v4Err == nil && len(v4Arr) > 0 {
				rows := [][]string{}
				for _, item := range v4Arr {
					r, ok := item.(map[string]any)
					if !ok {
						continue
					}
					rows = append(rows, []string{
						Ftoa(client.GetFloat(r, "iid")),
						client.GetStr(r, "DestIp"),
						client.GetStr(r, "Netmask"),
						client.GetStr(r, "Gateway"),
						Ftoa(client.GetFloat(r, "Metric")),
						client.GetStr(r, "Interface"),
					})
				}
				PrintTable([]string{"iid", "destination", "netmask", "gateway", "metric", "interface"}, rows)
			} else {
				fmt.Println("(no IPv4 static routes configured)")
			}

			// IPv6
			v6Data, err := cl.GetAPI("StaticRoutingIpv6Object", staticRoutingPage)
			if err != nil {
				return nil
			}
			v6Arr, v6Err := client.ToArr(v6Data)
			if v6Err == nil && len(v6Arr) > 0 {
				rows := [][]string{}
				for _, item := range v6Arr {
					r, ok := item.(map[string]any)
					if !ok {
						continue
					}
					rows = append(rows, []string{
						Ftoa(client.GetFloat(r, "iid")),
						client.GetStr(r, "dstIp"),
						Ftoa(client.GetFloat(r, "prefixLen")),
						client.GetStr(r, "gateway"),
						client.GetStr(r, "intfName"),
					})
				}
				PrintTable([]string{"iid", "destination", "prefix-len", "gateway", "interface"}, rows)
			}
			return nil
		},
	}
}

func newAdvStaticRoutingAddCmd() *cobra.Command {
	var destination, netmask, ifname, gateway string
	var metric int
	cmd := &cobra.Command{
		Use:   "static-routing-add",
		Short: "Add a static IPv4 route",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl := getClient(cmd)
			data, err := cl.GetAPI("StaticRoutingObject", staticRoutingPage)
			if err != nil {
				return err
			}
			arr, err := client.ToArr(data)
			if err != nil {
				return err
			}
			maxIID := 0
			for _, item := range arr {
				r, ok := item.(map[string]any)
				if ok {
					if iid := int(client.GetFloat(r, "iid")); iid > maxIID {
						maxIID = iid
					}
				}
			}
			nextIID := maxIID + 1
			entry := map[string]any{
				"iid": nextIID, "DestIp": destination, "Netmask": netmask,
				"Gateway": gateway, "Interface": ifname, "Metric": metric,
			}
			return cl.PostAPI("StaticRoutingObject", staticRoutingPage, []any{entry})
		},
	}
	cmd.Flags().StringVar(&destination, "destination", "", "destination network IP")
	cmd.Flags().StringVar(&netmask, "netmask", "", "subnet mask")
	cmd.Flags().StringVar(&ifname, "interface", "", "egress interface")
	cmd.Flags().StringVar(&gateway, "gateway", "0.0.0.0", "gateway IP")
	cmd.Flags().IntVar(&metric, "metric", 0, "route metric")
	cmd.MarkFlagRequired("destination")
	cmd.MarkFlagRequired("netmask")
	cmd.MarkFlagRequired("interface")
	return cmd
}

func newAdvStaticRoutingDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "static-routing-delete <iid>",
		Short: "Delete a static IPv4 route",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl := getClient(cmd)
			iid := atoi(args[0])
			data, err := cl.GetAPI("StaticRoutingObject", staticRoutingPage)
			if err != nil {
				return err
			}
			arr, err := client.ToArr(data)
			if err != nil {
				return err
			}
			var target map[string]any
			for _, item := range arr {
				r, ok := item.(map[string]any)
				if ok && int(client.GetFloat(r, "iid")) == iid {
					target = r
					break
				}
			}
			if target == nil {
				return fmt.Errorf("no static route with iid=%d", iid)
			}
			return cl.DeleteAPI("StaticRoutingObject", staticRoutingPage, []any{target})
		},
	}
	return cmd
}

func newAdvDhcpReservationsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "dhcp-reservations",
		Short: "List DHCP static IP reservations, plus the server's pool range",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl := getClient(cmd)

			pool, err := cl.Get("DhcpServerConfiguration", lanSetupPage)
			if err != nil {
				return err
			}
			p, err := client.ToObj(pool)
			if err != nil {
				return err
			}
			fmt.Printf("DHCP pool: %s - %s (lease %.0f s, gateway %s)\n",
				client.GetStr(p, "MinAddress"), client.GetStr(p, "MaxAddress"),
				client.GetFloat(p, "DHCPLeaseTime"), client.GetStr(p, "DefaultGateway"))

			data, err := cl.Get("DHCPStaticLease", lanSetupPage)
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
					Ftoa(client.GetFloat(r, "Index")),
					client.GetStr(r, "IP"),
					client.GetStr(r, "MAC"),
				})
			}
			PrintTable([]string{"index", "ip", "mac"}, rows)
			return nil
		},
	}
}

func newAdvDhcpReservationAddCmd() *cobra.Command {
	var ip, mac string
	cmd := &cobra.Command{
		Use:   "dhcp-reservation-add",
		Short: "Pin a device to a fixed IP via a DHCP static reservation (by MAC address)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl := getClient(cmd)
			data, err := cl.Get("DHCPStaticLease", lanSetupPage)
			if err != nil {
				return err
			}
			arr, err := client.ToArr(data)
			if err != nil {
				return err
			}
			macUpper := toUpperMAC(mac)
			for _, item := range arr {
				r, ok := item.(map[string]any)
				if ok {
					if client.GetStr(r, "IP") == ip {
						return fmt.Errorf("%s is already reserved", ip)
					}
					if toUpperMAC(client.GetStr(r, "MAC")) == macUpper {
						return fmt.Errorf("%s already has a reservation", macUpper)
					}
				}
			}
			maxIndex := 0
			for _, item := range arr {
				r, ok := item.(map[string]any)
				if ok {
					if idx := int(client.GetFloat(r, "Index")); idx > maxIndex {
						maxIndex = idx
					}
				}
			}
			nextIndex := maxIndex + 1
			entry := map[string]any{"Index": nextIndex, "IP": ip, "MAC": macUpper}
			fmt.Printf("Note: takes effect on the device's next DHCP renewal.\n")
			return cl.Post("DHCPStaticLease", lanSetupPage, []any{entry})
		},
	}
	cmd.Flags().StringVar(&ip, "ip", "", "IP address to always assign")
	cmd.Flags().StringVar(&mac, "mac", "", "device MAC address")
	cmd.MarkFlagRequired("ip")
	cmd.MarkFlagRequired("mac")
	return cmd
}

func newAdvDhcpReservationDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dhcp-reservation-delete <index>",
		Short: "Remove a DHCP static reservation",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl := getClient(cmd)
			index := atoi(args[0])
			data, err := cl.Get("DHCPStaticLease", lanSetupPage)
			if err != nil {
				return err
			}
			arr, err := client.ToArr(data)
			if err != nil {
				return err
			}
			var target map[string]any
			for _, item := range arr {
				r, ok := item.(map[string]any)
				if ok && int(client.GetFloat(r, "Index")) == index {
					target = r
					break
				}
			}
			if target == nil {
				return fmt.Errorf("no reservation with index=%d", index)
			}
			return cl.Delete("DHCPStaticLease", lanSetupPage, []any{target})
		},
	}
	return cmd
}

func atoi(s string) int {
	v, _ := strconv.Atoi(s)
	return v
}
