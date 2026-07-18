package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/anshuman852/dasan/internal/client"
)

const pfPage = "FirewallSetupPage-PortForwarding"

func newFwPortForwardingCmd() *cobra.Command {
	var wan int
	cmd := &cobra.Command{
		Use:   "port-forwarding",
		Short: "List port forwarding rules for a WAN connection",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl := getClient(cmd)
			objs := fmt.Sprintf("PortForwarding.%d", wan)
			data, err := cl.Get(objs, pfPage)
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
				active := "off"
				if client.GetBool(r, "active") {
					active = "on"
				}
				rows = append(rows, []string{
					Ftoa(client.GetFloat(r, "eid")),
					active,
					client.GetStr(r, "Protocol"),
					fmt.Sprintf("%s-%s", Ftoa(client.GetFloat(r, "StartPort")), Ftoa(client.GetFloat(r, "EndPort"))),
					client.GetStr(r, "LocalIP"),
					fmt.Sprintf("%s-%s", Ftoa(client.GetFloat(r, "LocalSPort")), Ftoa(client.GetFloat(r, "LocalEPort"))),
					client.GetStr(r, "Comment"),
				})
			}
			PrintTable([]string{"eid", "active", "proto", "ext-port", "local-ip", "local-port", "comment"}, rows)
			return nil
		},
	}
	cmd.Flags().IntVar(&wan, "wan", 2, "WAN connection iid")
	return cmd
}

func newFwPortForwardingAddCmd() *cobra.Command {
	var extPort, localPort, extPortEnd int
	var localIP, protocol, comment string
	var wan int
	cmd := &cobra.Command{
		Use:   "port-forwarding-add",
		Short: "Add a port forwarding rule",
		RunE: func(cmd *cobra.Command, args []string) error {
			cl := getClient(cmd)
			objs := fmt.Sprintf("PortForwarding.%d", wan)
			data, err := cl.Get(objs, pfPage)
			if err != nil {
				return err
			}
			arr, err := client.ToArr(data)
			if err != nil {
				return err
			}
			maxEID := 0
			for _, item := range arr {
				r, ok := item.(map[string]any)
				if ok {
					if eid := int(client.GetFloat(r, "eid")); eid > maxEID {
						maxEID = eid
					}
				}
			}
			nextEID := maxEID + 1

			if localPort == 0 {
				localPort = extPort
			}
			if extPortEnd == 0 {
				extPortEnd = extPort
			}
			localEnd := localPort + (extPortEnd - extPort)

			entry := map[string]any{
				"iid": wan, "eid": nextEID, "active": true,
				"Protocol": protocol, "StartPort": extPort, "EndPort": extPortEnd,
				"LocalIP": localIP, "LocalSPort": localPort, "LocalEPort": localEnd,
				"Comment": comment,
			}
			return cl.Post(objs, pfPage, []any{entry})
		},
	}
	cmd.Flags().IntVar(&extPort, "ext-port", 0, "external (start) port")
	cmd.Flags().StringVar(&localIP, "local-ip", "", "internal device IP")
	cmd.Flags().IntVar(&localPort, "local-port", 0, "internal port (defaults to ext-port)")
	cmd.Flags().IntVar(&extPortEnd, "ext-port-end", 0, "external end port for a range")
	cmd.Flags().StringVar(&protocol, "protocol", "TCP/UDP", "TCP, UDP, TCP/UDP, or GRE")
	cmd.Flags().StringVar(&comment, "comment", "", "optional label")
	cmd.Flags().IntVar(&wan, "wan", 2, "WAN connection iid")
	cmd.MarkFlagRequired("ext-port")
	cmd.MarkFlagRequired("local-ip")
	return cmd
}

func newFwPortForwardingDeleteCmd() *cobra.Command {
	var wan int
	cmd := &cobra.Command{
		Use:   "port-forwarding-delete <eid>",
		Short: "Delete a port forwarding rule",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl := getClient(cmd)
			eid := atoi(args[0])
			objs := fmt.Sprintf("PortForwarding.%d", wan)
			data, err := cl.Get(objs, pfPage)
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
				if ok && int(client.GetFloat(r, "eid")) == eid {
					target = r
					break
				}
			}
			if target == nil {
				return fmt.Errorf("no rule with eid=%d on WAN iid=%d", eid, wan)
			}
			return cl.Delete(objs, pfPage, []any{target})
		},
	}
	cmd.Flags().IntVar(&wan, "wan", 2, "WAN connection iid")
	return cmd
}
