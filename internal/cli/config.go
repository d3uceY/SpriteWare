package cli

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/d3uceY/spriteware/internal/config"
)

// setting is one writeable key: how to read it, and how to parse a new value.
type setting struct {
	get func(config.Config) string
	set func(*config.Config, string) error
}

// settings is the whole key surface. One map beats reflection over JSON tags: the
// parsing and the error messages are written here, where they can be read.
var settings = map[string]setting{
	"asepritePath": {
		get: func(c config.Config) string { return c.AsepritePath },
		set: func(c *config.Config, v string) error { c.AsepritePath = strings.TrimSpace(v); return nil },
	},
	"asepritePathConfirmed": boolKey(func(c *config.Config) *bool { return &c.AsepritePathConfirmed }),
	"mode": {
		get: func(c config.Config) string { return string(c.Mode) },
		set: func(c *config.Config, v string) error {
			switch config.Mode(v) {
			case config.ModeHeadless, config.ModeLive:
				c.Mode = config.Mode(v)
				return nil
			}
			return fmt.Errorf("mode is headless or live, not %q", v)
		},
	},
	"port":       portKey(func(c *config.Config) *int { return &c.Port }),
	"bridgePort": portKey(func(c *config.Config) *int { return &c.BridgePort }),
	"transport": {
		get: func(c config.Config) string { return string(c.Transport) },
		set: func(c *config.Config, v string) error {
			switch config.Transport(v) {
			case config.TransportStreamableHTTP, config.TransportSSE:
				c.Transport = config.Transport(v)
				return nil
			}
			return fmt.Errorf("transport is %s or %s, not %q", config.TransportStreamableHTTP, config.TransportSSE, v)
		},
	},
	"projectPath": {
		get: func(c config.Config) string { return c.ProjectPath },
		set: func(c *config.Config, v string) error { c.ProjectPath = strings.TrimSpace(v); return nil },
	},
	"startMcpOnLaunch": boolKey(func(c *config.Config) *bool { return &c.StartMCPOnLaunch }),
	"allowLua":         boolKey(func(c *config.Config) *bool { return &c.AllowLua }),
}

func boolKey(field func(*config.Config) *bool) setting {
	return setting{
		get: func(c config.Config) string { return strconv.FormatBool(*field(&c)) },
		set: func(c *config.Config, v string) error {
			b, err := strconv.ParseBool(v)
			if err != nil {
				return fmt.Errorf("want true or false, not %q", v)
			}
			*field(c) = b
			return nil
		},
	}
}

func portKey(field func(*config.Config) *int) setting {
	return setting{
		get: func(c config.Config) string { return strconv.Itoa(*field(&c)) },
		set: func(c *config.Config, v string) error {
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 || n > 65535 {
				return fmt.Errorf("want a port in 1..65535, not %q", v)
			}
			*field(c) = n
			return nil
		},
	}
}

func settingKeys() []string {
	keys := make([]string, 0, len(settings))
	for key := range settings {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func lookupSetting(key string) (setting, error) {
	s, ok := settings[key]
	if !ok {
		return setting{}, fmt.Errorf("no setting named %q; try one of: %s", key, strings.Join(settingKeys(), ", "))
	}
	return s, nil
}

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Read and write the settings file",
		Long: `Print the effective settings, or one key, or change one.

Flags given to any command win for that run and are never written back; this is
the only command that saves.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return showConfig(cmd) },
	}
	cmd.AddCommand(newConfigGetCmd(), newConfigSetCmd(), newConfigPathCmd(), newConfigResetCmd())
	return cmd
}

func showConfig(cmd *cobra.Command) error {
	store, err := config.Load()
	if err != nil {
		return err
	}
	if jsonOut {
		return printJSON(cmd.OutOrStdout(), store.Get())
	}

	out := cmd.OutOrStdout()
	cfg := store.Get()
	fmt.Fprintf(out, "file  %s\n\n", store.FilePath())
	for _, key := range settingKeys() {
		value := settings[key].get(cfg)
		if value == "" {
			value = "(not set)"
		}
		fmt.Fprintf(out, "%-22s %s\n", key, value)
	}
	return nil
}

func newConfigGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <key>",
		Short: "Print one setting",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := lookupSetting(args[0])
			if err != nil {
				return err
			}
			store, err := config.Load()
			if err != nil {
				return err
			}
			value := s.get(store.Get())
			if jsonOut {
				return printJSON(cmd.OutOrStdout(), map[string]string{args[0]: value})
			}
			fmt.Fprintln(cmd.OutOrStdout(), value)
			return nil
		},
	}
}

func newConfigSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Change one setting",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := lookupSetting(args[0])
			if err != nil {
				return err
			}
			store, err := config.Load()
			if err != nil {
				return err
			}
			// Validate on a copy: a rejected value must leave the file alone.
			next := store.Get()
			if err := s.set(&next, args[1]); err != nil {
				return err
			}
			saved, err := store.Save(next)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s = %s\n", args[0], s.get(saved))
			return nil
		},
	}
}

func newConfigPathCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "Print the settings file location",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := config.Path()
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), path)
			return nil
		},
	}
}

func newConfigResetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reset",
		Short: "Write the defaults back",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			store, err := config.Load()
			if err != nil {
				return err
			}
			if _, err := store.Save(config.Default()); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "defaults written to %s\n", store.FilePath())
			return nil
		},
	}
}
