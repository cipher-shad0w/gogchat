package cmd

import (
	"encoding/json"
	"fmt"
	"unicode"

	"github.com/spf13/cobra"

	"github.com/cipher-shad0w/gogchat/internal/api"
	"github.com/cipher-shad0w/gogchat/internal/output"
)

// NewReactionsCmd creates the top-level "reactions" command with list, add, and
// remove subcommands.
func NewReactionsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reactions",
		Short: "Manage reactions on messages",
		Long:  "List, add, and remove emoji reactions on Google Chat messages.",
	}

	cmd.AddCommand(
		newReactionsListCmd(),
		newReactionsAddCmd(),
		newReactionsRemoveCmd(),
	)

	return cmd
}

// newReactionsListCmd creates the "reactions list" subcommand.
func newReactionsListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list MESSAGE",
		Short: "List reactions on a message",
		Long:  "List emoji reactions on the specified message. MESSAGE is the full message resource name (spaces/{space}/messages/{message}).",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newAPIClient()
			if err != nil {
				return err
			}
			formatter := getFormatter()
			svc := api.NewReactionsService(client)

			parent := args[0]
			pageSize, _ := cmd.Flags().GetInt("page-size")
			pageToken, _ := cmd.Flags().GetString("page-token")
			filter, _ := cmd.Flags().GetString("filter")
			all, _ := cmd.Flags().GetBool("all")

			ctx := cmd.Context()

			// Collect all pages if --all is set; otherwise fetch a single page.
			var allReactions []json.RawMessage

			for {
				raw, err := svc.List(ctx, parent, pageSize, pageToken, filter)
				if err != nil {
					return fmt.Errorf("listing reactions: %w", err)
				}

				if formatter.IsJSON() && !all {
					return formatter.PrintRaw(raw)
				}

				var resp struct {
					Reactions []json.RawMessage `json:"reactions"`
					NextPage  string            `json:"nextPageToken"`
				}
				if err := json.Unmarshal(raw, &resp); err != nil {
					return fmt.Errorf("parsing response: %w", err)
				}

				allReactions = append(allReactions, resp.Reactions...)

				if !all || resp.NextPage == "" {
					pageToken = resp.NextPage
					break
				}
				pageToken = resp.NextPage
			}

			if formatter.IsJSON() {
				// --all + --json: emit collected reactions as a JSON array.
				return formatter.Print(allReactions)
			}

			if len(allReactions) == 0 {
				formatter.PrintMessage("No reactions found.")
				return nil
			}

			table := output.NewTable("REACTION_NAME", "EMOJI", "USER")
			for _, r := range allReactions {
				var reaction struct {
					Name  string `json:"name"`
					Emoji struct {
						Unicode     string `json:"unicode"`
						CustomEmoji struct {
							UID string `json:"uid"`
						} `json:"customEmoji"`
					} `json:"emoji"`
					User struct {
						Name        string `json:"name"`
						DisplayName string `json:"displayName"`
					} `json:"user"`
				}
				if err := json.Unmarshal(r, &reaction); err != nil {
					continue
				}

				emoji := reaction.Emoji.Unicode
				if emoji == "" {
					emoji = reaction.Emoji.CustomEmoji.UID
				}

				user := reaction.User.DisplayName
				if user == "" {
					user = reaction.User.Name
				}

				table.AddRow(reaction.Name, emoji, user)
			}

			fmt.Print(table.Render())

			if !all && pageToken != "" {
				formatter.PrintMessage(fmt.Sprintf("\nMore results available. Use --page-token %s to see the next page, or use --all to fetch everything.", pageToken))
			}

			return nil
		},
	}

	cmd.Flags().Int("page-size", 25, "Maximum number of reactions to return per page")
	cmd.Flags().String("page-token", "", "Page token for pagination")
	cmd.Flags().String("filter", "", "Filter reactions (e.g. by emoji or user)")
	cmd.Flags().Bool("all", false, "Fetch all pages of results")

	return cmd
}

// isUnicodeEmoji returns true if the string starts with a non-ASCII character,
// indicating it is likely a unicode emoji rather than a custom emoji UID.
func isUnicodeEmoji(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		// Custom emoji UIDs start with a letter or digit (ASCII).
		// Unicode emoji are non-ASCII characters.
		if r > unicode.MaxASCII {
			return true
		}
		// Only check the first rune.
		break
	}
	return false
}

// newReactionsAddCmd creates the "reactions add" subcommand.
func newReactionsAddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add MESSAGE",
		Short: "Add a reaction to a message",
		Long:  "Add an emoji reaction to the specified message. MESSAGE is the full message resource name (spaces/{space}/messages/{message}).",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newAPIClient()
			if err != nil {
				return err
			}
			formatter := getFormatter()
			svc := api.NewReactionsService(client)

			parent := args[0]
			emoji, _ := cmd.Flags().GetString("emoji")

			// Build the reaction body. If the emoji looks like unicode (starts
			// with a non-ASCII character), use the unicode field; otherwise treat
			// it as a custom emoji UID.
			var body map[string]interface{}
			if isUnicodeEmoji(emoji) {
				body = map[string]interface{}{
					"emoji": map[string]interface{}{
						"unicode": emoji,
					},
				}
			} else {
				body = map[string]interface{}{
					"emoji": map[string]interface{}{
						"customEmoji": map[string]interface{}{
							"uid": emoji,
						},
					},
				}
			}

			raw, err := svc.Create(cmd.Context(), parent, body)
			if err != nil {
				return fmt.Errorf("adding reaction: %w", err)
			}

			if formatter.IsJSON() {
				return formatter.PrintRaw(raw)
			}

			formatter.PrintSuccess(fmt.Sprintf("Reaction %s added to %s", emoji, parent))
			return nil
		},
	}

	cmd.Flags().String("emoji", "", "Emoji to react with (unicode emoji like \"👍\" or custom emoji UID)")
	_ = cmd.MarkFlagRequired("emoji")

	return cmd
}

// newReactionsRemoveCmd creates the "reactions remove" subcommand.
func newReactionsRemoveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove REACTION",
		Short: "Remove a reaction from a message",
		Long:  "Remove the specified reaction. REACTION is the full reaction resource name (spaces/{space}/messages/{message}/reactions/{reaction}).",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newAPIClient()
			if err != nil {
				return err
			}
			formatter := getFormatter()
			svc := api.NewReactionsService(client)

			name := args[0]
			force, _ := cmd.Flags().GetBool("force")

			if !force {
				fmt.Printf("Remove reaction %s? [y/N]: ", name)
				var answer string
				fmt.Scanln(&answer)
				if answer != "y" && answer != "Y" {
					formatter.PrintMessage("Cancelled.")
					return nil
				}
			}

			raw, err := svc.Delete(cmd.Context(), name)
			if err != nil {
				return fmt.Errorf("removing reaction: %w", err)
			}

			if formatter.IsJSON() {
				return formatter.PrintRaw(raw)
			}

			formatter.PrintSuccess(fmt.Sprintf("Reaction %s removed.", name))
			return nil
		},
	}

	cmd.Flags().Bool("force", false, "Skip confirmation prompt")

	return cmd
}
