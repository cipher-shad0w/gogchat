package cmd

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/cipher-shad0w/gogchat/internal/api"
	"github.com/cipher-shad0w/gogchat/internal/output"
)

// NewEmojiCmd creates the top-level "emoji" command with list, get, create,
// and delete subcommands.
func NewEmojiCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "emoji",
		Aliases: []string{"emojis", "custom-emoji"},
		Short:   "Manage custom emojis",
		Long:    "List, get, create, and delete custom emojis in Google Chat.",
	}

	cmd.AddCommand(
		newEmojiListCmd(),
		newEmojiGetCmd(),
		newEmojiCreateCmd(),
		newEmojiDeleteCmd(),
	)

	return cmd
}

// newEmojiListCmd creates the "emoji list" subcommand.
func newEmojiListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List custom emojis",
		Long:  "List custom emojis available in Google Chat.",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newAPIClient()
			if err != nil {
				return err
			}
			formatter := getFormatter()
			svc := api.NewEmojiService(client)

			pageSize, _ := cmd.Flags().GetInt("page-size")
			pageToken, _ := cmd.Flags().GetString("page-token")
			filter, _ := cmd.Flags().GetString("filter")
			all, _ := cmd.Flags().GetBool("all")

			ctx := cmd.Context()

			var allEmojis []json.RawMessage

			for {
				raw, err := svc.List(ctx, filter, pageSize, pageToken)
				if err != nil {
					return fmt.Errorf("listing emojis: %w", err)
				}

				if formatter.IsJSON() && !all {
					return formatter.PrintRaw(raw)
				}

				var resp struct {
					CustomEmojis []json.RawMessage `json:"customEmojis"`
					NextPage     string            `json:"nextPageToken"`
				}
				if err := json.Unmarshal(raw, &resp); err != nil {
					return fmt.Errorf("parsing response: %w", err)
				}

				allEmojis = append(allEmojis, resp.CustomEmojis...)

				if !all || resp.NextPage == "" {
					pageToken = resp.NextPage
					break
				}
				pageToken = resp.NextPage
			}

			if formatter.IsJSON() {
				// --all + --json: emit collected emojis as a JSON array.
				return formatter.Print(allEmojis)
			}

			if len(allEmojis) == 0 {
				formatter.PrintMessage("No custom emojis found.")
				return nil
			}

			table := output.NewTable("NAME", "SHORT_NAME", "CREATOR", "CREATE_TIME")
			for _, e := range allEmojis {
				var emoji struct {
					Name         string `json:"name"`
					UID          string `json:"uid"`
					EmojiName    string `json:"emojiName"`
					TemporaryURI string `json:"temporaryImageUri"`
					Creator      struct {
						Name        string `json:"name"`
						DisplayName string `json:"displayName"`
					} `json:"creator"`
					CreateTime string `json:"createTime"`
				}
				if err := json.Unmarshal(e, &emoji); err != nil {
					continue
				}

				shortName := emoji.EmojiName
				creator := emoji.Creator.DisplayName
				if creator == "" {
					creator = emoji.Creator.Name
				}
				createTime := output.FormatTime(emoji.CreateTime)

				table.AddRow(emoji.Name, shortName, creator, createTime)
			}

			fmt.Print(table.Render())

			if !all && pageToken != "" {
				formatter.PrintMessage(fmt.Sprintf("\nMore results available. Use --page-token %s to see the next page, or use --all to fetch everything.", pageToken))
			}

			return nil
		},
	}

	cmd.Flags().Int("page-size", 25, "Maximum number of emojis to return per page")
	cmd.Flags().String("page-token", "", "Page token for pagination")
	cmd.Flags().String("filter", "", "Filter expression for custom emojis")
	cmd.Flags().Bool("all", false, "Fetch all pages of results")

	return cmd
}

// newEmojiGetCmd creates the "emoji get" subcommand.
func newEmojiGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get EMOJI",
		Short: "Get a custom emoji",
		Long:  "Get details of a custom emoji by name or ID. EMOJI is the emoji resource name (customEmojis/{emoji}) or just the emoji ID.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newAPIClient()
			if err != nil {
				return err
			}
			formatter := getFormatter()
			svc := api.NewEmojiService(client)

			raw, err := svc.Get(cmd.Context(), args[0])
			if err != nil {
				return fmt.Errorf("getting emoji: %w", err)
			}

			if formatter.IsJSON() {
				return formatter.PrintRaw(raw)
			}

			// Parse the emoji for human-readable output.
			var emoji struct {
				Name         string `json:"name"`
				UID          string `json:"uid"`
				EmojiName    string `json:"emojiName"`
				TemporaryURI string `json:"temporaryImageUri"`
				Creator      struct {
					Name        string `json:"name"`
					DisplayName string `json:"displayName"`
				} `json:"creator"`
				Payload struct {
					FileContent string `json:"fileContent"`
					Filename    string `json:"filename"`
				} `json:"payload"`
				CreateTime string `json:"createTime"`
			}
			if err := json.Unmarshal(raw, &emoji); err != nil {
				return fmt.Errorf("parsing emoji: %w", err)
			}

			creator := emoji.Creator.DisplayName
			if creator == "" {
				creator = emoji.Creator.Name
			}

			payloadInfo := "(none)"
			if emoji.Payload.Filename != "" {
				payloadInfo = emoji.Payload.Filename
			}
			if emoji.TemporaryURI != "" {
				payloadInfo = emoji.TemporaryURI
			}

			fmt.Printf("Name:        %s\n", emoji.Name)
			fmt.Printf("Short Name:  %s\n", emoji.EmojiName)
			fmt.Printf("Emoji ID:    %s\n", emoji.UID)
			fmt.Printf("Creator:     %s\n", creator)
			fmt.Printf("Payload:     %s\n", payloadInfo)
			fmt.Printf("Create Time: %s\n", output.FormatTime(emoji.CreateTime))

			return nil
		},
	}

	return cmd
}

// newEmojiCreateCmd creates the "emoji create" subcommand.
func newEmojiCreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a custom emoji",
		Long:  "Create a new custom emoji by uploading an image file.",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newAPIClient()
			if err != nil {
				return err
			}
			formatter := getFormatter()
			svc := api.NewEmojiService(client)

			shortName, _ := cmd.Flags().GetString("name")
			imageFile, _ := cmd.Flags().GetString("image-file")

			// Read the image file and base64-encode it.
			data, err := os.ReadFile(imageFile)
			if err != nil {
				return fmt.Errorf("reading image file %s: %w", imageFile, err)
			}
			encoded := base64.StdEncoding.EncodeToString(data)
			filename := filepath.Base(imageFile)

			body := map[string]interface{}{
				"shortName": shortName,
				"payload": map[string]interface{}{
					"fileContent": encoded,
					"filename":    filename,
				},
			}

			raw, err := svc.Create(cmd.Context(), body)
			if err != nil {
				return fmt.Errorf("creating emoji: %w", err)
			}

			if formatter.IsJSON() {
				return formatter.PrintRaw(raw)
			}

			// Parse and display the created emoji.
			var emoji struct {
				Name      string `json:"name"`
				UID       string `json:"uid"`
				EmojiName string `json:"emojiName"`
				Creator   struct {
					Name        string `json:"name"`
					DisplayName string `json:"displayName"`
				} `json:"creator"`
				CreateTime string `json:"createTime"`
			}
			if err := json.Unmarshal(raw, &emoji); err != nil {
				return fmt.Errorf("parsing response: %w", err)
			}

			creator := emoji.Creator.DisplayName
			if creator == "" {
				creator = emoji.Creator.Name
			}

			formatter.PrintSuccess("Custom emoji created!")
			fmt.Printf("Name:        %s\n", emoji.Name)
			fmt.Printf("Short Name:  %s\n", emoji.EmojiName)
			fmt.Printf("Emoji ID:    %s\n", emoji.UID)
			fmt.Printf("Creator:     %s\n", creator)
			fmt.Printf("Create Time: %s\n", output.FormatTime(emoji.CreateTime))

			return nil
		},
	}

	cmd.Flags().String("name", "", "Short name for the custom emoji (required)")
	cmd.Flags().String("image-file", "", "Path to image file for the emoji (required)")
	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("image-file")

	return cmd
}

// newEmojiDeleteCmd creates the "emoji delete" subcommand.
func newEmojiDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete EMOJI",
		Short: "Delete a custom emoji",
		Long:  "Delete a custom emoji by name or ID. EMOJI is the emoji resource name (customEmojis/{emoji}) or just the emoji ID.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newAPIClient()
			if err != nil {
				return err
			}
			formatter := getFormatter()
			svc := api.NewEmojiService(client)

			name := args[0]
			force, _ := cmd.Flags().GetBool("force")

			if !force {
				fmt.Printf("Delete custom emoji %s? [y/N]: ", name)
				var answer string
				fmt.Scanln(&answer)
				if answer != "y" && answer != "Y" {
					formatter.PrintMessage("Cancelled.")
					return nil
				}
			}

			raw, err := svc.Delete(cmd.Context(), name)
			if err != nil {
				return fmt.Errorf("deleting emoji: %w", err)
			}

			if formatter.IsJSON() {
				return formatter.PrintRaw(raw)
			}

			formatter.PrintSuccess(fmt.Sprintf("Custom emoji %s deleted.", name))
			return nil
		},
	}

	cmd.Flags().Bool("force", false, "Skip confirmation prompt")

	return cmd
}
