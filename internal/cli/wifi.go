package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/anshuman852/dasan/internal/client"
)

// NewWifiCmd creates the `dasan wifi` command tree.
func NewWifiCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "wifi",
		Short: "WiFi SSIDs (2.4GHz/5GHz radios)",
	}

	cmd.AddCommand(
		newWifiListCmd(),
		newWifiSetCmd(),
		newWifiMacfilterListCmd(),
		newWifiMacfilterAddCmd(),
		newWifiMacfilterDeleteCmd(),
		newWifiScheduleShowCmd(),
		newWifiScheduleSetCmd(),
		newWifiMeshStatusCmd(),
		newWifiMeshBssCmd(),
		newWifiMeshTopoCmd(),
		newWifiMeshOnboardCmd(),
		newWifiMeshBackhaulSteerCmd(),
	)

	return cmd
}

func newWifiListCmd() *cobra.Command {
	var showPassword bool
	var band string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Show all configured SSIDs",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl := getClient(cmd)
			type bandInfo struct{ obj, label string }
			bands := []bandInfo{{"WLANConfiguration", "2.4GHz"}}
			if band != "2.4ghz" {
				bands = append(bands, bandInfo{"WLAN11acConfiguration", "5GHz"})
			}
			if band == "5ghz" {
				bands = []bandInfo{{"WLAN11acConfiguration", "5GHz"}}
			}

			rows := [][]string{}
			for _, b := range bands {
				data, err := cl.Get(b.obj, "")
				if err != nil {
					continue // 5GHz may 403 on some hardware
				}
				arr, err := client.ToArr(data)
				if err != nil {
					continue
				}
				for _, item := range arr {
					s, ok := item.(map[string]any)
					if !ok {
						continue
					}
					pw := client.GetStr(s, "KeyPassphrase")
					masked := MaskPassword(pw)
					if showPassword {
						masked = pw
					}
					radioStatus := "down"
					if client.GetBool(s, "RadioEnabled") {
						radioStatus = "up"
					}
					visibility := "hidden"
					if client.GetBool(s, "SSIDAdvertisementEnabled") {
						visibility = "visible"
					}
					rows = append(rows, []string{
						b.label,
						Ftoa(client.GetFloat(s, "iid")),
						client.GetStr(s, "SSID"),
						radioStatus,
						client.GetStr(s, "Security"),
						masked,
						visibility,
					})
				}
			}
			PrintTable([]string{"band", "iid", "ssid", "radio", "security", "password", "broadcast"}, rows)
			return nil
		},
	}
	cmd.Flags().BoolVar(&showPassword, "show-password", false, "reveal WiFi passphrases")
	cmd.Flags().StringVar(&band, "band", "all", "show all, 2.4ghz, or 5ghz")
	return cmd
}

func newWifiSetCmd() *cobra.Command {
	var ssid, password, radio, band string
	cmd := &cobra.Command{
		Use:   "set <iid>",
		Short: "Update an SSID's name/password/radio state (read-modify-write)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl := getClient(cmd)
			iid := args[0]

			// Map band to object name
			obj := bandObjects[band]
			if obj == "" {
				obj = "WLANConfiguration"
			}

			data, err := cl.Get(obj, "")
			if err != nil {
				return err
			}
			arr, err := client.ToArr(data)
			if err != nil {
				return err
			}
			var target map[string]any
			for _, item := range arr {
				s, ok := item.(map[string]any)
				if ok && Ftoa(client.GetFloat(s, "iid")) == iid {
					target = s
					break
				}
			}
			if target == nil {
				return fmt.Errorf("no WLAN with iid=%s on band %s (object %s)", iid, band, obj)
			}

			if ssid != "" {
				target["SSID"] = ssid
			}
			if password != "" {
				target["KeyPassphrase"] = password
			}
			switch radio {
			case "on":
				target["RadioEnabled"] = true
			case "off":
				target["RadioEnabled"] = false
			}

			return cl.Post(obj, "", []any{target})
		},
	}
	cmd.Flags().StringVar(&ssid, "ssid", "", "new SSID name")
	cmd.Flags().StringVar(&password, "key", "", "new WiFi passphrase")
	cmd.Flags().StringVar(&radio, "radio", "", "turn radio on or off")
	cmd.Flags().StringVar(&band, "band", "2.4ghz", "WiFi band (2.4ghz or 5ghz)")
	return cmd
}

// ---------------------------------------------------------------------------
// MAC filter
// ---------------------------------------------------------------------------

var bandObjects = map[string]string{
	"2.4ghz": "WLANConfiguration",
	"5ghz":   "WLAN11acConfiguration",
}

const macFilterPage = "WifiSetupPage-MacFilter"

func bandObj(band string) (string, error) {
	obj, ok := bandObjects[band]
	if !ok {
		return "", fmt.Errorf("band must be 2.4ghz or 5ghz")
	}
	return obj, nil
}

func toUpperMAC(s string) string {
	return strings.ToUpper(s)
}

func newWifiMacfilterListCmd() *cobra.Command {
	var band string
	cmd := &cobra.Command{
		Use:   "macfilter-list",
		Short: "Show MAC filter policy and address list for each WLAN interface",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl := getClient(cmd)
			obj, err := bandObj(band)
			if err != nil {
				return err
			}
			data, err := cl.Get(obj, macFilterPage)
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
				macs := []string{}
				if ml, ok := e["MacAcl"].([]any); ok {
					for _, m := range ml {
						macs = append(macs, fmt.Sprintf("%v", m))
					}
				}
				policy := client.GetStr(e, "MacAclPolicy")
				if label, ok := map[float64]string{0: "disabled", 1: "allow-list", 2: "deny-list"}[client.GetFloat(e, "MacAclPolicy")]; ok {
					policy = label
				}
				rows = append(rows, []string{
					Ftoa(client.GetFloat(e, "iid")),
					client.GetStr(e, "SSID"),
					policy,
					fmt.Sprintf("%d", len(macs)),
					strings.Join(macs, ", "),
				})
			}
			PrintTable([]string{"iid", "ssid", "policy", "count", "mac addresses"}, rows)
			return nil
		},
	}
	cmd.Flags().StringVar(&band, "band", "2.4ghz", "2.4ghz or 5ghz")
	return cmd
}

func newWifiMacfilterAddCmd() *cobra.Command {
	var iid int
	var mac, band string
	cmd := &cobra.Command{
		Use:   "macfilter-add",
		Short: "Add a MAC address to an interface's filter list (max 8 entries)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl := getClient(cmd)
			obj, err := bandObj(band)
			if err != nil {
				return err
			}
			data, err := cl.Get(obj, macFilterPage)
			if err != nil {
				return err
			}
			arr, err := client.ToArr(data)
			if err != nil {
				return err
			}
			var target map[string]any
			for _, item := range arr {
				e, ok := item.(map[string]any)
				if ok && int(client.GetFloat(e, "iid")) == iid {
					target = e
					break
				}
			}
			if target == nil {
				return fmt.Errorf("no WLAN with iid=%d", iid)
			}

			upperMAC := toUpperMAC(mac)
			macs := []string{}
			if ml, ok := target["MacAcl"].([]any); ok {
				for _, m := range ml {
					macs = append(macs, fmt.Sprintf("%v", m))
				}
			}

			// Check if MAC already present (case-insensitive)
			for _, m := range macs {
				if toUpperMAC(m) == upperMAC {
					return fmt.Errorf("%s is already in the filter list", upperMAC)
				}
			}
			if len(macs) >= 8 {
				return fmt.Errorf("filter list is full (max 8 entries)")
			}
			macs = append(macs, upperMAC)
			target["MacAcl"] = macs

			if err := cl.Post(obj, macFilterPage, target); err != nil {
				return err
			}

			// Verify persistence (silent failure check)
			after, err := cl.Get(obj, macFilterPage)
			if err != nil {
				return nil
			}
			arr2, _ := client.ToArr(after)
			var found bool
			for _, item := range arr2 {
				e, ok := item.(map[string]any)
				if ok && int(client.GetFloat(e, "iid")) == iid {
					if ml, ok := e["MacAcl"].([]any); ok {
						for _, m := range ml {
							if toUpperMAC(fmt.Sprintf("%v", m)) == upperMAC {
								found = true
								break
							}
						}
					}
				}
			}
			if !found {
				fmt.Fprintf(cmd.ErrOrStderr(), "Warning: the router accepted the request but did not persist it (known firmware quirk) -- verify in the web UI.\n")
				return nil
			}
			fmt.Printf("Added %s to iid=%d (%s) filter list\n", upperMAC, iid, band)
			return nil
		},
	}
	cmd.Flags().IntVar(&iid, "iid", 0, "WLAN interface iid")
	cmd.Flags().StringVar(&mac, "mac", "", "MAC address to add")
	cmd.Flags().StringVar(&band, "band", "2.4ghz", "2.4ghz or 5ghz")
	cmd.MarkFlagRequired("iid")
	cmd.MarkFlagRequired("mac")
	return cmd
}

func newWifiMacfilterDeleteCmd() *cobra.Command {
	var iid int
	var mac, band string
	cmd := &cobra.Command{
		Use:   "macfilter-delete",
		Short: "Remove a MAC address from an interface's filter list",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl := getClient(cmd)
			obj, err := bandObj(band)
			if err != nil {
				return err
			}
			data, err := cl.Get(obj, macFilterPage)
			if err != nil {
				return err
			}
			arr, err := client.ToArr(data)
			if err != nil {
				return err
			}
			var target map[string]any
			for _, item := range arr {
				e, ok := item.(map[string]any)
				if ok && int(client.GetFloat(e, "iid")) == iid {
					target = e
					break
				}
			}
			if target == nil {
				return fmt.Errorf("no WLAN with iid=%d", iid)
			}

			upperMAC := toUpperMAC(mac)
			macs := []string{}
			if ml, ok := target["MacAcl"].([]any); ok {
				for _, m := range ml {
					macs = append(macs, fmt.Sprintf("%v", m))
				}
			}

			found := false
			filtered := []string{}
			for _, m := range macs {
				if toUpperMAC(m) == upperMAC {
					found = true
				} else {
					filtered = append(filtered, m)
				}
			}
			if !found {
				return fmt.Errorf("%s is not in the filter list", upperMAC)
			}
			target["MacAcl"] = filtered
			return cl.Post(obj, macFilterPage, target)
		},
	}
	cmd.Flags().IntVar(&iid, "iid", 0, "WLAN interface iid")
	cmd.Flags().StringVar(&mac, "mac", "", "MAC address to remove")
	cmd.Flags().StringVar(&band, "band", "2.4ghz", "2.4ghz or 5ghz")
	cmd.MarkFlagRequired("iid")
	cmd.MarkFlagRequired("mac")
	return cmd
}

// ---------------------------------------------------------------------------
// WiFi schedule
// ---------------------------------------------------------------------------

const schedulePage = "WifiSetupPage-WifiSchedule"

func newWifiScheduleShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "schedule-show",
		Short: "Show the WiFi auto-refresh schedule",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl := getClient(cmd)
			data, err := cl.Get("WifiRefreshObj", schedulePage)
			if err != nil {
				return err
			}
			cfg, err := client.ToObj(data)
			if err != nil {
				return err
			}

			days := "-"
			if s := client.GetStr(cfg, "Schedule"); s != "" {
				days = s
			}
			rows := [][]string{
				{"enabled", fmt.Sprintf("%v", cfg["Enable"])},
				{"days", days},
				{"time", fmt.Sprintf("%02d:%02d", int(client.GetFloat(cfg, "Hour")), int(client.GetFloat(cfg, "Minute")))},
				{"threshold enabled", fmt.Sprintf("%v", cfg["Enable_ucount"])},
				{"unassociated-client threshold", fmt.Sprintf("%v", cfg["Ucount_threshold"])},
			}
			PrintKeyValue(rows)
			return nil
		},
	}
}

func newWifiScheduleSetCmd() *cobra.Command {
	var enable bool
	var hasEnable bool
	var days string
	var hour, minute int
	var hasHour, hasMinute bool
	var thresholdEnable bool
	var hasThresholdEnable bool
	var threshold int
	var hasThreshold bool
	cmd := &cobra.Command{
		Use:   "schedule-set",
		Short: "Update the WiFi auto-refresh schedule (read-modify-write)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl := getClient(cmd)
			data, err := cl.Get("WifiRefreshObj", schedulePage)
			if err != nil {
				return err
			}
			cfg, err := client.ToObj(data)
			if err != nil {
				return err
			}

			if hasEnable {
				cfg["Enable"] = enable
			}
			if days != "" {
				cfg["Schedule"] = days
			}
			if hasHour {
				cfg["Hour"] = hour
			}
			if hasMinute {
				cfg["Minute"] = minute
			}
			if hasThresholdEnable {
				cfg["Enable_ucount"] = thresholdEnable
			}
			if hasThreshold {
				cfg["Ucount_threshold"] = threshold
			}
			return cl.Post("WifiRefreshObj", schedulePage, cfg)
		},
	}
	cmd.Flags().BoolVar(&enable, "enable", false, "turn the scheduled refresh on/off")
	cmd.Flags().BoolVar(&hasEnable, "enable-set", false, "set enable flag")
	cmd.Flags().StringVar(&days, "days", "", "comma-separated days, e.g. mon,wed,fri")
	cmd.Flags().IntVar(&hour, "hour", 0, "hour (0-23)")
	cmd.Flags().BoolVar(&hasHour, "hour-set", false, "set hour")
	cmd.Flags().IntVar(&minute, "minute", 0, "minute (0-59)")
	cmd.Flags().BoolVar(&hasMinute, "minute-set", false, "set minute")
	cmd.Flags().BoolVar(&thresholdEnable, "enable-threshold", false, "enable unassociated client threshold")
	cmd.Flags().BoolVar(&hasThresholdEnable, "enable-threshold-set", false, "set enable threshold")
	cmd.Flags().IntVar(&threshold, "threshold", 0, "unassociated client count that triggers a refresh")
	cmd.Flags().BoolVar(&hasThreshold, "threshold-set", false, "set threshold")
	return cmd
}

// ---------------------------------------------------------------------------
// Mesh
// ---------------------------------------------------------------------------

const meshSettingPage = "WifiMeshPage-Setting"
const meshBssPage = "WifiMeshPage-BSSConfiguration"
const meshTopoPage = "WifiMeshPage-Status"
const meshActionPage = "WifiMeshPage-Action"

func newWifiMeshStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mesh-status",
		Short: "Show mesh configuration/mode (WifiMeshCfg). Requires mesh-capable hardware.",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl := getClient(cmd)
			data, err := cl.Get("WifiMeshCfg", meshSettingPage)
			if err != nil {
				return err
			}
			obj, err := client.ToObj(data)
			if err != nil {
				return err
			}
			rows := [][]string{}
			for k, v := range obj {
				rows = append(rows, []string{k, fmt.Sprintf("%v", v)})
			}
			PrintKeyValue(rows)
			return nil
		},
	}
}

func newWifiMeshBssCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mesh-bss",
		Short: "List mesh BSS (SSID) configuration per band. Requires mesh-capable hardware.",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl := getClient(cmd)
			data, err := cl.Get("WifiMeshBssCfg", meshBssPage)
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
					Ftoa(client.GetFloat(e, "iid")),
					client.GetStr(e, "SSID"),
					client.GetStr(e, "auth"),
					client.GetStr(e, "encryption"),
				})
			}
			PrintTable([]string{"iid", "ssid", "auth", "encryption"}, rows)
			return nil
		},
	}
}

func newWifiMeshTopoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mesh-topo",
		Short: "Show mesh topology nodes. Requires mesh-capable hardware.",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl := getClient(cmd)
			data, err := cl.Get("WifiMeshTopo", meshTopoPage)
			if err != nil {
				return err
			}
			obj, err := client.ToObj(data)
			if err != nil {
				fmt.Printf("%v\n", data)
				return nil
			}
			nodes, ok := obj["nodes"].([]any)
			if !ok {
				fmt.Println("(no nodes)")
				return nil
			}
			rows := [][]string{}
			for _, item := range nodes {
				n, ok := item.(map[string]any)
				if !ok {
					continue
				}
				rows = append(rows, []string{
					Ftoa(client.GetFloat(n, "id")),
					client.GetStr(n, "type"),
					client.GetStr(n, "Mac"),
					client.GetStr(n, "title"),
				})
			}
			PrintTable([]string{"id", "type", "mac", "info"}, rows)
			return nil
		},
	}
}

func newWifiMeshOnboardCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mesh-onboard",
		Short: "Trigger mesh onboarding mode. Requires mesh-capable hardware.",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl := getClient(cmd)
			return cl.Post("WifiMeshAct", meshActionPage, map[string]any{
				"action": "TriggerOnboarding",
			})
		},
	}
}

func newWifiMeshBackhaulSteerCmd() *cobra.Command {
	var staMac, targetBssid string
	cmd := &cobra.Command{
		Use:   "mesh-backhaul-steer",
		Short: "Steer a mesh backhaul station to a target BSSID. Requires mesh-capable hardware.",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl := getClient(cmd)
			return cl.Post("WifiMeshAct", meshActionPage, map[string]any{
				"action":         "BackhaulSteering",
				"backhaulStaMac": staMac,
				"targetBssid":    targetBssid,
			})
		},
	}
	cmd.Flags().StringVar(&staMac, "sta-mac", "", "backhaul station MAC to steer")
	cmd.Flags().StringVar(&targetBssid, "target-bssid", "", "target BSSID to steer the station to")
	cmd.MarkFlagRequired("sta-mac")
	cmd.MarkFlagRequired("target-bssid")
	return cmd
}
