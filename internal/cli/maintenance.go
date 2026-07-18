package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/anshuman852/dasan/internal/client"
)

// NewMaintenanceCmd creates the `dasan maintenance` command tree.
func NewMaintenanceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "maintenance",
		Short: "Administration, logs, NTP, backup, firmware, and misc maintenance pages",
	}

	cmd.AddCommand(
		newMaintAdminCmd(),
		newMaintNtpCmd(),
		newMaintFirmwareCmd(),
		newMaintLogsCmd(),
		newMaintBackupCmd(),
		newMaintAutoRebootCmd(),
		newMaintPortMirroringCmd(),
		newMaintSnmpCmd(),
		newMaintSyslogConfigCmd(),
	)

	return cmd
}

func newMaintAdminCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "admin",
		Short: "Web account settings: admin ports, idle timeout, login lockout (read-only)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl := getClient(cmd)
			d, err := cl.GetNS("Administration", "", "sys")
			if err != nil {
				return err
			}
			obj, err := client.ToObj(d)
			if err != nil {
				return err
			}
			fmt.Printf("HTTP port:        %s (enabled: %v)\n", client.GetStr(obj, "WebPort"), obj["HttpEnable"])
			fmt.Printf("HTTPS port:       %s\n", client.GetStr(obj, "HttpsPort"))
			fmt.Printf("Access from WAN:  %v\n", obj["WebOnWan"])
			fmt.Printf("Idle timeout:     %s s\n", client.GetStr(obj, "timeout"))
			fmt.Printf("Max login trials: %s (lockout %s s)\n", client.GetStr(obj, "MaxTrial"), client.GetStr(obj, "BannedTime"))
			return nil
		},
	}
}

func newMaintNtpCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ntp",
		Short: "NTP server configuration and sync status (read-only)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl := getClient(cmd)
			data, err := cl.Get("TimeServer", "MaintainancePage-NTP")
			if err != nil {
				return err
			}
			d, err := client.ToObj(data)
			if err != nil {
				return err
			}
			fmt.Printf("Enabled:      %v\n", d["enable"])
			fmt.Printf("Status:       %s\n", client.GetStr(d, "status"))
			fmt.Printf("Current time: %s (%s)\n", client.GetStr(d, "currentLocalTime"), client.GetStr(d, "localTimeZone"))
			servers := []string{}
			for i := 1; i <= 4; i++ {
				key := fmt.Sprintf("NTPServer%d", i)
				if v, ok := d[key]; ok && fmt.Sprintf("%v", v) != "" {
					servers = append(servers, fmt.Sprintf("%v", v))
				}
			}
			if len(servers) == 0 {
				servers = append(servers, "-")
			}
			fmt.Printf("NTP servers:  %s\n", joinStrings(servers, ", "))
			fmt.Printf("DST:          %v\n", d["daylightSavingsUsed"])
			return nil
		},
	}
}

func newMaintFirmwareCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "firmware",
		Short: "Current firmware/hardware version (read-only)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl := getClient(cmd)
			data, err := cl.Get("DeviceInfo", "")
			if err != nil {
				return err
			}
			d, err := client.ToObj(data)
			if err != nil {
				return err
			}
			fmt.Printf("Manufacturer:     %s\n", client.GetStr(d, "manufacturer"))
			fmt.Printf("Model:            %s\n", client.GetStr(d, "modelName"))
			fmt.Printf("Hardware version: %s\n", client.GetStr(d, "hardwareVersion"))
			fmt.Printf("Software version: %s\n", client.GetStr(d, "softwareVersion"))
			return nil
		},
	}
}

func newMaintLogsCmd() *cobra.Command {
	var lines int
	cmd := &cobra.Command{
		Use:   "logs",
		Short: "Show recent system log entries (read-only)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl := getClient(cmd)
			raw, err := cl.GetRaw("/bin/?objs=SyslogDownload&page=MaintainancePage-SystemLog")
			if err != nil {
				return err
			}
			text := string(raw)
			// Print last N non-empty lines
			allLines := []string{}
			for _, l := range textLines(text) {
				if l != "" {
					allLines = append(allLines, l)
				}
			}
			start := len(allLines) - lines
			if start < 0 {
				start = 0
			}
			for _, l := range allLines[start:] {
				fmt.Println(l)
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&lines, "lines", 50, "number of most recent log lines to show")
	return cmd
}

func newMaintBackupCmd() *cobra.Command {
	var output string
	cmd := &cobra.Command{
		Use:   "backup",
		Short: "Download the current router configuration backup (GET only, no restore)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl := getClient(cmd)
			raw, err := cl.GetRaw("/bin/?objs=BackupConfig&page=MaintainancePage-BackupRestore")
			if err != nil {
				return err
			}
			path := output
			if path == "" {
				path = fmt.Sprintf("backup_%s.bin", time.Now().Format("20060102_150405"))
			}
			if err := os.WriteFile(path, raw, 0644); err != nil {
				return fmt.Errorf("write backup: %w", err)
			}
			fmt.Printf("Saved config backup to %s (%d bytes)\n", path, len(raw))
			return nil
		},
	}
	cmd.Flags().StringVar(&output, "output", "", "output file path")
	return cmd
}

func newMaintAutoRebootCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "auto-reboot",
		Short: "Scheduled auto-reboot configuration (read-only)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl := getClient(cmd)
			data, err := cl.Get("AutoRebootObj", "MaintainancePage-AutoReboot")
			if err != nil {
				return err
			}
			d, err := client.ToObj(data)
			if err != nil {
				return err
			}
			fmt.Printf("Enabled:              %v\n", d["Enable"])
			fmt.Printf("Daily at:             %02d:%02d\n", int(client.GetFloat(d, "Hour")), int(client.GetFloat(d, "Minute")))
			fmt.Printf("Min uptime:           %.0f min\n", client.GetFloat(d, "Uptime"))
			fmt.Printf("Uptime-count reboot:  %v (threshold %.0f)\n", d["Enable_ucount"], client.GetFloat(d, "Ucount_threshold"))
			return nil
		},
	}
}

func newMaintPortMirroringCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "port-mirroring",
		Short: "LAN port mirroring configuration (read-only)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl := getClient(cmd)
			data, err := cl.Get("PortMirroring", "MaintainancePage-PortMirroring")
			if err != nil {
				return err
			}
			d, err := client.ToObj(data)
			if err != nil {
				return err
			}
			fmt.Printf("Enabled: %v\n", d["Enable"])
			ports := []string{}
			for k, v := range d {
				if len(k) > 5 && k[:5] == "Port_" && isTruthy(v) {
					ports = append(ports, k[5:])
				}
			}
			if len(ports) == 0 {
				fmt.Println("Mirrored ports: -")
			} else {
				fmt.Printf("Mirrored ports: %s\n", joinStrings(ports, ", "))
			}
			return nil
		},
	}
}

func newMaintSnmpCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "snmp",
		Short: "SNMP agent configuration (read-only; values are masked)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl := getClient(cmd)
			data, err := cl.Get("SnmpCfg", "MaintainancePage-Snmp")
			if err != nil {
				return err
			}
			d, err := client.ToObj(data)
			if err != nil {
				return err
			}
			mask := func(v string) string {
				if v != "" {
					return "***"
				}
				return "-"
			}
			fmt.Printf("SNMP v1/v2 enabled: %v\n", d["snmpActive"])
			fmt.Printf("Get community:      %s\n", mask(client.GetStr(d, "getCommunity")))
			fmt.Printf("Set community:      %s\n", mask(client.GetStr(d, "setCommunity")))
			fmt.Printf("Trap manager:       %s\n", client.GetStr(d, "trapManagerIPv4"))
			fmt.Printf("sysName/Contact/Loc: %s / %s / %s\n", client.GetStr(d, "sysName"), client.GetStr(d, "sysContact"), client.GetStr(d, "sysLocation"))
			fmt.Printf("SNMPv3 enabled:     %v\n", d["snmpV3Active"])
			if client.GetBool(d, "snmpV3Active") {
				fmt.Printf("SNMPv3 user:        %s\n", client.GetStr(d, "snmpV3UserName"))
				fmt.Printf("SNMPv3 password:    %s\n", mask(client.GetStr(d, "snmpV3Password")))
			}
			return nil
		},
	}
}

func newMaintSyslogConfigCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "syslog-configuration",
		Short: "Remote syslog forwarding configuration (read-only; password is masked)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl := getClient(cmd)
			data, err := cl.Get("RemoteSyslogCfg", "MaintainancePage-SyslogConfiguration")
			if err != nil {
				return err
			}
			d, err := client.ToObj(data)
			if err != nil {
				return err
			}
			fmt.Printf("Enabled:  %v\n", d["isActive"])
			fmt.Printf("Protocol: %s\n", client.GetStr(d, "protocol"))
			host := client.GetStr(d, "host")
			if host == "" {
				host = "-"
			}
			fmt.Printf("Host:     %s\n", host)
			fmt.Printf("Level:    %s\n", client.GetStr(d, "level"))
			if client.GetStr(d, "userName") != "" {
				pw := "-"
				if client.GetStr(d, "passwd") != "" {
					pw = "***"
				}
				fmt.Printf("User:     %s (password: %s)\n", client.GetStr(d, "userName"), pw)
			}
			return nil
		},
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func textLines(s string) []string {
	lines := []string{}
	current := ""
	for _, c := range s {
		if c == '\n' {
			lines = append(lines, current)
			current = ""
		} else {
			current += string(c)
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}

func isTruthy(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return t == "true" || t == "True" || t == "1" || t == "yes"
	case float64:
		return t != 0
	default:
		return false
	}
}

func joinStrings(items []string, sep string) string {
	if len(items) == 0 {
		return "-"
	}
	result := ""
	for i, s := range items {
		if i > 0 {
			result += sep
		}
		result += s
	}
	return result
}
