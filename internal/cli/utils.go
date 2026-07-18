// Package cli implements the dasan CLI subcommands.
package cli

import (
	"fmt"
	"os"
	"text/tabwriter"
)

// PrintTable prints a formatted table with headers and rows.
func PrintTable(headers []string, rows [][]string) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)

	// Print header
	for i, h := range headers {
		if i > 0 {
			fmt.Fprint(w, "\t")
		}
		fmt.Fprint(w, h)
	}
	fmt.Fprintln(w)

	// Print separator
	for i := range headers {
		if i > 0 {
			fmt.Fprint(w, "\t")
		}
		fmt.Fprint(w, "---")
	}
	fmt.Fprintln(w)

	// Print rows
	for _, row := range rows {
		for i, cell := range row {
			if i > 0 {
				fmt.Fprint(w, "\t")
			}
			fmt.Fprint(w, cell)
		}
		fmt.Fprintln(w)
	}
	w.Flush()
}

// PrintKeyValue prints a simple key-value pair table.
func PrintKeyValue(rows [][]string) {
	PrintTable([]string{"Field", "Value"}, rows)
}

// MaskPassword returns a masked version of a password string.
func MaskPassword(pw string) string {
	if pw == "" {
		return "-"
	}
	n := len(pw)
	if n > 8 {
		n = 8
	}
	masked := make([]byte, n)
	for i := range masked {
		masked[i] = '*'
	}
	return string(masked)
}

// BoolStr returns "yes" or "no" for a bool.
func BoolStr(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// StatusStr returns "up" or "down" for a bool.
func StatusStr(b bool) string {
	if b {
		return "up"
	}
	return "down"
}

// OnOff returns "on" or "off" for a bool.
func OnOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

// Ftoa formats a float64 as a string without decimal if it's a whole number.
func Ftoa(f float64) string {
	if f == float64(int64(f)) {
		return fmt.Sprintf("%.0f", f)
	}
	return fmt.Sprintf("%.2f", f)
}
