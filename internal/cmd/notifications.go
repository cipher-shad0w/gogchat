package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cipher-shad0w/gogchat/internal/api"
)

// NewNotificationsCmd creates the top-level "notifications" command with get
// and update subcommands for space notification settings.
func NewNotificationsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "notifications",
		Short: "Manage space notification settings",
		Long:  "Get and update notification settings for Google Chat spaces.",
	}

	cmd.AddCommand(
		newNotificationsGetCmd(),
		newNotificationsUpdateCmd(),
	)

	return cmd
}

// newNotificationsGetCmd creates the "notifications get" subcommand.
func newNotificationsGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get SETTING",
		Short: "Get notification settings for a space",
		Long:  "Retrieve the notification setting for a space. SETTING is the full resource name (users/{user}/spaces/{space}/spaceNotificationSetting).",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newAPIClient()
			if err != nil {
				return err
			}
			formatter := getFormatter()
			svc := api.NewNotificationsService(client)

			name := args[0]

			raw, err := svc.Get(cmd.Context(), name)
			if err != nil {
				return fmt.Errorf("getting notification settings: %w", err)
			}

			if formatter.IsJSON() {
				return formatter.PrintRaw(raw)
			}

			var setting struct {
				Name                string `json:"name"`
				NotificationSetting string `json:"notificationSetting"`
				MuteSetting         string `json:"muteSetting"`
			}
			if err := json.Unmarshal(raw, &setting); err != nil {
				return fmt.Errorf("parsing response: %w", err)
			}

			fmt.Printf("Name:                  %s\n", setting.Name)
			fmt.Printf("Notification Setting:  %s\n", formatSettingValue(setting.NotificationSetting))
			fmt.Printf("Mute Setting:          %s\n", formatSettingValue(setting.MuteSetting))

			return nil
		},
	}

	return cmd
}

// newNotificationsUpdateCmd creates the "notifications update" subcommand.
func newNotificationsUpdateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update SETTING",
		Short: "Update notification settings for a space",
		Long: `Update the notification setting for a space. SETTING is the full resource name
(users/{user}/spaces/{space}/spaceNotificationSetting).

Provide --notification-setting and/or --mute-setting flags to update. The
update mask is auto-built from the flags that are set, unless --update-mask
is explicitly provided.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newAPIClient()
			if err != nil {
				return err
			}
			formatter := getFormatter()
			svc := api.NewNotificationsService(client)

			name := args[0]
			notificationSetting, _ := cmd.Flags().GetString("notification-setting")
			muteSetting, _ := cmd.Flags().GetString("mute-setting")
			updateMask, _ := cmd.Flags().GetString("update-mask")

			body := map[string]interface{}{}
			var maskParts []string

			if notificationSetting != "" {
				body["notificationSetting"] = notificationSetting
				maskParts = append(maskParts, "notificationSetting")
			}
			if muteSetting != "" {
				body["muteSetting"] = muteSetting
				maskParts = append(maskParts, "muteSetting")
			}

			if len(body) == 0 {
				return fmt.Errorf("at least one of --notification-setting or --mute-setting must be provided")
			}

			// Auto-build update mask from set flags if not explicitly provided.
			if updateMask == "" {
				updateMask = strings.Join(maskParts, ",")
			}

			raw, err := svc.Patch(cmd.Context(), name, body, updateMask)
			if err != nil {
				return fmt.Errorf("updating notification settings: %w", err)
			}

			if formatter.IsJSON() {
				return formatter.PrintRaw(raw)
			}

			var setting struct {
				Name                string `json:"name"`
				NotificationSetting string `json:"notificationSetting"`
				MuteSetting         string `json:"muteSetting"`
			}
			if err := json.Unmarshal(raw, &setting); err != nil {
				return fmt.Errorf("parsing response: %w", err)
			}

			formatter.PrintSuccess("Notification setting updated.")
			fmt.Printf("Name:                  %s\n", setting.Name)
			fmt.Printf("Notification Setting:  %s\n", formatSettingValue(setting.NotificationSetting))
			fmt.Printf("Mute Setting:          %s\n", formatSettingValue(setting.MuteSetting))

			return nil
		},
	}

	cmd.Flags().String("notification-setting", "", "Notification setting (e.g. NOTIFICATION_SETTING_ALL, NOTIFICATION_SETTING_NONE)")
	cmd.Flags().String("mute-setting", "", "Mute setting (e.g. MUTE_SETTING_MUTED, MUTE_SETTING_UNMUTED)")
	cmd.Flags().String("update-mask", "", "Fields to update (auto-built from flags if not set)")

	return cmd
}

// formatSettingValue returns the value or a placeholder if empty.
func formatSettingValue(v string) string {
	if v == "" {
		return "(not set)"
	}
	return v
}
