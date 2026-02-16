package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/cipher-shad0w/gogchat/internal/api"
	"github.com/cipher-shad0w/gogchat/internal/output"
)

// NewReadStateCmd creates the top-level "readstate" command with get-space,
// update-space, and get-thread subcommands.
func NewReadStateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "readstate",
		Short: "Manage read state for spaces and threads",
		Long:  "Get and update read state for Google Chat spaces and threads.",
	}

	cmd.AddCommand(
		newReadStateGetSpaceCmd(),
		newReadStateUpdateSpaceCmd(),
		newReadStateGetThreadCmd(),
	)

	return cmd
}

// newReadStateGetSpaceCmd creates the "readstate get-space" subcommand.
func newReadStateGetSpaceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get-space READSTATE",
		Short: "Get the read state of a space",
		Long:  "Retrieve the read state of a space for the calling user. READSTATE is the full resource name (users/{user}/spaces/{space}/spaceReadState).",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newAPIClient()
			if err != nil {
				return err
			}
			formatter := getFormatter()
			svc := api.NewReadStateService(client)

			name := args[0]

			raw, err := svc.GetSpaceReadState(cmd.Context(), name)
			if err != nil {
				return fmt.Errorf("getting space read state: %w", err)
			}

			if formatter.IsJSON() {
				return formatter.PrintRaw(raw)
			}

			var state struct {
				Name         string `json:"name"`
				LastReadTime string `json:"lastReadTime"`
			}
			if err := json.Unmarshal(raw, &state); err != nil {
				return fmt.Errorf("parsing response: %w", err)
			}

			fmt.Printf("Name:           %s\n", state.Name)
			fmt.Printf("Last Read Time: %s\n", output.FormatTime(state.LastReadTime))

			return nil
		},
	}

	return cmd
}

// newReadStateUpdateSpaceCmd creates the "readstate update-space" subcommand.
func newReadStateUpdateSpaceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update-space READSTATE",
		Short: "Update the read state of a space",
		Long:  "Update the read state of a space for the calling user. READSTATE is the full resource name (users/{user}/spaces/{space}/spaceReadState).",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newAPIClient()
			if err != nil {
				return err
			}
			formatter := getFormatter()
			svc := api.NewReadStateService(client)

			name := args[0]
			lastReadTime, _ := cmd.Flags().GetString("last-read-time")
			updateMask, _ := cmd.Flags().GetString("update-mask")

			body := map[string]interface{}{
				"lastReadTime": lastReadTime,
			}

			raw, err := svc.UpdateSpaceReadState(cmd.Context(), name, body, updateMask)
			if err != nil {
				return fmt.Errorf("updating space read state: %w", err)
			}

			if formatter.IsJSON() {
				return formatter.PrintRaw(raw)
			}

			var state struct {
				Name         string `json:"name"`
				LastReadTime string `json:"lastReadTime"`
			}
			if err := json.Unmarshal(raw, &state); err != nil {
				return fmt.Errorf("parsing response: %w", err)
			}

			formatter.PrintSuccess("Space read state updated.")
			fmt.Printf("Name:           %s\n", state.Name)
			fmt.Printf("Last Read Time: %s\n", output.FormatTime(state.LastReadTime))

			return nil
		},
	}

	cmd.Flags().String("last-read-time", "", "Last read time in RFC3339 format (required)")
	_ = cmd.MarkFlagRequired("last-read-time")
	cmd.Flags().String("update-mask", "lastReadTime", "Fields to update (comma-separated)")

	return cmd
}

// newReadStateGetThreadCmd creates the "readstate get-thread" subcommand.
func newReadStateGetThreadCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get-thread THREADREADSTATE",
		Short: "Get the read state of a thread",
		Long:  "Retrieve the read state of a thread for the calling user. THREADREADSTATE is the full resource name (users/{user}/spaces/{space}/threads/{thread}/threadReadState).",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newAPIClient()
			if err != nil {
				return err
			}
			formatter := getFormatter()
			svc := api.NewReadStateService(client)

			name := args[0]

			raw, err := svc.GetThreadReadState(cmd.Context(), name)
			if err != nil {
				return fmt.Errorf("getting thread read state: %w", err)
			}

			if formatter.IsJSON() {
				return formatter.PrintRaw(raw)
			}

			var state struct {
				Name         string `json:"name"`
				LastReadTime string `json:"lastReadTime"`
			}
			if err := json.Unmarshal(raw, &state); err != nil {
				return fmt.Errorf("parsing response: %w", err)
			}

			fmt.Printf("Name:           %s\n", state.Name)
			fmt.Printf("Last Read Time: %s\n", output.FormatTime(state.LastReadTime))

			return nil
		},
	}

	return cmd
}
