package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "media115",
	Short: "Media management CLI for 115 cloud drive",
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(scanCmd)
	rootCmd.AddCommand(scrapeCmd)
	rootCmd.AddCommand(scrapeFixCmd)
	rootCmd.AddCommand(organizeCmd)
	rootCmd.AddCommand(doctorCmd)
}
