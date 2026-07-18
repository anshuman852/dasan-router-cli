package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/anshuman852/dasan/internal/client"
)

// ---------------------------------------------------------------------------
// Port triggering
// ---------------------------------------------------------------------------

const ptPage = "FirewallSetupPage-PortTriggering"

func newFwPortTriggeringCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "port-triggering",
		Short: "List port triggering rules",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl := getClient(cmd)
			data, err := cl.Get("PortTriggering", ptPage)
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
					Ftoa(client.GetFloat(r, "iid")),
					client.GetStr(r, "Name"),
					client.GetStr(r, "TProtocol"),
					fmt.Sprintf("%s-%s", Ftoa(client.GetFloat(r, "TSPort")), Ftoa(client.GetFloat(r, "TEPort"))),
					client.GetStr(r, "OProtocol"),
					fmt.Sprintf("%s-%s", Ftoa(client.GetFloat(r, "OSPort")), Ftoa(client.GetFloat(r, "OEPort"))),
				})
			}
			PrintTable([]string{"iid", "name", "trigger-proto", "trigger-port", "open-proto", "open-port"}, rows)
			return nil
		},
	}
}

func newFwPortTriggeringAddCmd() *cobra.Command {
	var name, triggerProtocol, openProtocol string
	var triggerPort, triggerPortEnd, openPort, openPortEnd int
	cmd := &cobra.Command{
		Use:   "port-triggering-add",
		Short: "Add a port triggering rule",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl := getClient(cmd)
			data, err := cl.Get("PortTriggering", ptPage)
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
			if triggerPortEnd == 0 {
				triggerPortEnd = triggerPort
			}
			if openPortEnd == 0 {
				openPortEnd = openPort
			}
			entry := map[string]any{
				"iid": nextIID, "Name": name,
				"TProtocol": triggerProtocol, "TSPort": triggerPort, "TEPort": triggerPortEnd,
				"OProtocol": openProtocol, "OSPort": openPort, "OEPort": openPortEnd,
			}
			return cl.Post("PortTriggering", ptPage, []any{entry})
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "rule label")
	cmd.Flags().IntVar(&triggerPort, "trigger-port", 0, "trigger (outbound) start port")
	cmd.Flags().IntVar(&triggerPortEnd, "trigger-port-end", 0, "trigger end port")
	cmd.Flags().StringVar(&triggerProtocol, "trigger-protocol", "TCP", "TCP, UDP, or TCP/UDP")
	cmd.Flags().IntVar(&openPort, "open-port", 0, "opened (inbound) start port")
	cmd.Flags().IntVar(&openPortEnd, "open-port-end", 0, "opened end port")
	cmd.Flags().StringVar(&openProtocol, "open-protocol", "TCP", "TCP, UDP, or TCP/UDP")
	cmd.MarkFlagRequired("name")
	cmd.MarkFlagRequired("trigger-port")
	cmd.MarkFlagRequired("open-port")
	return cmd
}

func newFwPortTriggeringDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "port-triggering-delete <iid>",
		Short: "Delete a port triggering rule",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl := getClient(cmd)
			iid := atoi(args[0])
			data, err := cl.Get("PortTriggering", ptPage)
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
				return fmt.Errorf("no port triggering rule with iid=%d", iid)
			}
			return cl.Delete("PortTriggering", ptPage, []any{target})
		},
	}
}

// ---------------------------------------------------------------------------
// URL filter
// ---------------------------------------------------------------------------

const ufPage = "FirewallSetupPage-UrlFilter"

func newFwUrlFilterCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "url-filter",
		Short: "List URL filter rules",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl := getClient(cmd)
			data, err := cl.Get("URLFilterObject", ufPage)
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
				if client.GetBool(r, "Activate") {
					state = "active"
				}
				rows = append(rows, []string{
					Ftoa(client.GetFloat(r, "Index")),
					client.GetStr(r, "URL"),
					state,
				})
			}
			PrintTable([]string{"index", "url", "state"}, rows)
			return nil
		},
	}
}

func newFwUrlFilterAddCmd() *cobra.Command {
	var url string
	var active bool
	cmd := &cobra.Command{
		Use:   "url-filter-add",
		Short: "Add a URL filter rule",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl := getClient(cmd)
			data, err := cl.Get("URLFilterObject", ufPage)
			if err != nil {
				return err
			}
			arr, err := client.ToArr(data)
			if err != nil {
				return err
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
			entry := map[string]any{"Index": nextIndex, "URL": url, "Activate": active}
			return cl.Post("URLFilterObject", ufPage, []any{entry})
		},
	}
	cmd.Flags().StringVar(&url, "url", "", "URL or keyword to block")
	cmd.Flags().BoolVar(&active, "active", true, "activate the rule immediately")
	cmd.MarkFlagRequired("url")
	return cmd
}

func newFwUrlFilterDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "url-filter-delete <index>",
		Short: "Delete a URL filter rule",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl := getClient(cmd)
			index := atoi(args[0])
			data, err := cl.Get("URLFilterObject", ufPage)
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
				return fmt.Errorf("no URL filter rule with index=%d", index)
			}
			return cl.Delete("URLFilterObject", ufPage, []any{target})
		},
	}
}

// ---------------------------------------------------------------------------
// Parental control
// ---------------------------------------------------------------------------

const pcPage = "FirewallSetupPage-ParentalControl"

var pcDays = []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}

func newFwParentalControlCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "parental-control",
		Short: "Show the weekly parental control blocking schedule",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl := getClient(cmd)
			data, err := cl.Get("ParentalControlObj", pcPage)
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
				day := Ftoa(client.GetFloat(r, "iid"))
				if idx := int(client.GetFloat(r, "iid")); idx >= 0 && idx < len(pcDays) {
					day = pcDays[idx]
				}
				rows = append(rows, []string{
					day,
					OnOff(client.GetBool(r, "Enable")),
					client.GetStr(r, "StartTime"),
					client.GetStr(r, "EndTime"),
				})
			}
			PrintTable([]string{"day", "enabled", "start", "end"}, rows)
			return nil
		},
	}
}

func newFwParentalControlAddCmd() *cobra.Command {
	var day int
	var start, end string
	cmd := &cobra.Command{
		Use:   "parental-control-add",
		Short: "Enable the blocking window for a day of the week",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl := getClient(cmd)
			data, err := cl.Get("ParentalControlObj", pcPage)
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
				if ok && int(client.GetFloat(r, "iid")) == day {
					target = r
					break
				}
			}
			if target == nil {
				return fmt.Errorf("no parental control schedule entry for day iid=%d", day)
			}
			target["Enable"] = true
			target["StartTime"] = start
			target["EndTime"] = end
			return cl.Post("ParentalControlObj", pcPage, arr)
		},
	}
	cmd.Flags().IntVar(&day, "day", 0, "day index, 0=Mon..6=Sun")
	cmd.Flags().StringVar(&start, "start", "", "start time, HH:MM")
	cmd.Flags().StringVar(&end, "end", "", "end time, HH:MM")
	cmd.MarkFlagRequired("start")
	cmd.MarkFlagRequired("end")
	return cmd
}

func newFwParentalControlDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "parental-control-delete <day>",
		Short: "Disable the blocking window for a day of the week",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl := getClient(cmd)
			day := atoi(args[0])
			data, err := cl.Get("ParentalControlObj", pcPage)
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
				if ok && int(client.GetFloat(r, "iid")) == day {
					target = r
					break
				}
			}
			if target == nil {
				return fmt.Errorf("no parental control schedule entry for day iid=%d", day)
			}
			target["Enable"] = false
			return cl.Post("ParentalControlObj", pcPage, arr)
		},
	}
}
