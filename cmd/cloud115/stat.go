package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var statCmd = &cobra.Command{
	Use:   "stat <path>",
	Short: "显示文件或目录的元数据（JSON 格式）",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path := args[0]

		client, err := getClient()
		if err != nil {
			return err
		}
		defer client.Close()

		entry, err := client.Stat(path)
		if err != nil {
			return fmt.Errorf("路径不存在: %s", path)
		}

		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.SetEscapeHTML(false)
		return enc.Encode(entry)
	},
}

func init() {
	rootCmd.AddCommand(statCmd)
}
