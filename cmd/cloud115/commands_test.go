package main

import (
	"strings"
	"testing"
)

func TestRootCommandName(t *testing.T) {
	if rootCmd.Use != "cloud115" {
		t.Errorf("rootCmd.Use = %q, want %q", rootCmd.Use, "cloud115")
	}
}

func TestCommandsRegistered(t *testing.T) {
	expected := []string{
		"auth", "ls", "cache", "serve", "sync", "dedup",
		"get", "rename", "strm", "doctor", "find",
		"mkdir", "mv", "rm", "stat",
	}
	for _, name := range expected {
		t.Run(name, func(t *testing.T) {
			cmd, _, err := rootCmd.Find([]string{name})
			if err != nil {
				t.Fatalf("command %q not found: %v", name, err)
			}
			if cmd == rootCmd {
				t.Fatalf("command %q not registered (returned root)", name)
			}
			// Verify Use field starts with the expected name.
			if !strings.HasPrefix(cmd.Use, name) {
				t.Errorf("cmd.Use = %q, want prefix %q", cmd.Use, name)
			}
		})
	}
}

func TestCacheSubcommands(t *testing.T) {
	cacheCmd, _, err := rootCmd.Find([]string{"cache"})
	if err != nil || cacheCmd == rootCmd {
		t.Fatal("cache command not found")
	}

	sub, _, err := cacheCmd.Find([]string{"status"})
	if err != nil || sub == cacheCmd {
		t.Error("cache status subcommand not found")
	}

	sub2, _, err := cacheCmd.Find([]string{"clear"})
	if err != nil || sub2 == cacheCmd {
		t.Error("cache clear subcommand not found")
	}
}

func TestAuthFlags(t *testing.T) {
	cmd, _, _ := rootCmd.Find([]string{"auth"})
	if cmd == rootCmd {
		t.Fatal("auth command not found")
	}

	flags := []string{"check", "renew", "force", "app", "get-qr", "wait-qr"}
	for _, f := range flags {
		if cmd.Flags().Lookup(f) == nil {
			t.Errorf("auth flag --%s not registered", f)
		}
	}
}

func TestLsFlags(t *testing.T) {
	cmd, _, _ := rootCmd.Find([]string{"ls"})
	if cmd == rootCmd {
		t.Fatal("ls command not found")
	}

	if cmd.Flags().Lookup("long") == nil {
		t.Error("ls flag --long not registered")
	}
	if cmd.Flags().Lookup("recursive") == nil {
		t.Error("ls flag --recursive not registered")
	}
	if cmd.Flags().Lookup("depth") == nil {
		t.Error("ls flag --depth not registered")
	}
}

func TestSyncFlags(t *testing.T) {
	cmd, _, _ := rootCmd.Find([]string{"sync"})
	if cmd == rootCmd {
		t.Fatal("sync command not found")
	}

	if cmd.Flags().Lookup("deep") == nil {
		t.Error("sync flag --deep not registered")
	}
	if cmd.Flags().Lookup("depth") == nil {
		t.Error("sync flag --depth not registered")
	}
}

func TestDedupFlags(t *testing.T) {
	cmd, _, _ := rootCmd.Find([]string{"dedup"})
	if cmd == rootCmd {
		t.Fatal("dedup command not found")
	}

	if cmd.Flags().Lookup("execute") == nil {
		t.Error("dedup flag --execute not registered")
	}
}

func TestRenameFlags(t *testing.T) {
	cmd, _, _ := rootCmd.Find([]string{"rename"})
	if cmd == rootCmd {
		t.Fatal("rename command not found")
	}

	if cmd.Flags().Lookup("batch") == nil {
		t.Error("rename flag --batch not registered")
	}
}

func TestRmFlags(t *testing.T) {
	cmd, _, _ := rootCmd.Find([]string{"rm"})
	if cmd == rootCmd {
		t.Fatal("rm command not found")
	}

	if cmd.Flags().Lookup("recursive") == nil {
		t.Error("rm flag --recursive not registered")
	}
}

func TestMkdirFlags(t *testing.T) {
	cmd, _, _ := rootCmd.Find([]string{"mkdir"})
	if cmd == rootCmd {
		t.Fatal("mkdir command not found")
	}

	if cmd.Flags().Lookup("parents") == nil {
		t.Error("mkdir flag --parents not registered")
	}
}

func TestServeFlags(t *testing.T) {
	cmd, _, _ := rootCmd.Find([]string{"serve"})
	if cmd == rootCmd {
		t.Fatal("serve command not found")
	}

	if cmd.Flags().Lookup("host") == nil {
		t.Error("serve flag --host not registered")
	}
	if cmd.Flags().Lookup("port") == nil {
		t.Error("serve flag --port not registered")
	}
}

func TestStrmFlags(t *testing.T) {
	cmd, _, _ := rootCmd.Find([]string{"strm"})
	if cmd == rootCmd {
		t.Fatal("strm command not found")
	}

	if cmd.Flags().Lookup("output") == nil {
		t.Error("strm flag --output not registered")
	}
	if cmd.Flags().Lookup("host") == nil {
		t.Error("strm flag --host not registered")
	}
	if cmd.Flags().Lookup("port") == nil {
		t.Error("strm flag --port not registered")
	}
}

func TestRootVerboseFlag(t *testing.T) {
	if rootCmd.PersistentFlags().Lookup("verbose") == nil {
		t.Error("root persistent flag --verbose not registered")
	}
}

func TestAuthCommandShort(t *testing.T) {
	cmd, _, _ := rootCmd.Find([]string{"auth"})
	if cmd.Short == "" {
		t.Error("auth command should have a Short description")
	}
}

func TestLsCommandArgs(t *testing.T) {
	cmd, _, _ := rootCmd.Find([]string{"ls"})
	if cmd.Args == nil {
		t.Error("ls command should have Args validator")
	}
}

func TestSyncCommandArgs(t *testing.T) {
	cmd, _, _ := rootCmd.Find([]string{"sync"})
	if cmd.Args == nil {
		t.Error("sync command should have Args validator")
	}
}

func TestDedupCommandArgs(t *testing.T) {
	cmd, _, _ := rootCmd.Find([]string{"dedup"})
	if cmd.Args == nil {
		t.Error("dedup command should have Args validator")
	}
}

func TestDoctorCommandShort(t *testing.T) {
	cmd, _, _ := rootCmd.Find([]string{"doctor"})
	if cmd.Short == "" {
		t.Error("doctor command should have a Short description")
	}
}

func TestFindCommandArgs(t *testing.T) {
	cmd, _, _ := rootCmd.Find([]string{"find"})
	if cmd.Args == nil {
		t.Error("find command should have Args validator")
	}
}
