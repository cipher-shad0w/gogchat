package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/cipher-shad0w/gogchat/internal/api"
	"github.com/cipher-shad0w/gogchat/internal/output"
	"github.com/spf13/cobra"
)

// NewMembersCmd creates the top-level "members" command with subcommands for
// listing, getting, adding, updating, and removing space members.
func NewMembersCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "members",
		Aliases: []string{"member"},
		Short:   "Manage members of Google Chat spaces",
		Long:    "List, get, add, update, and remove members in Google Chat spaces.",
	}

	cmd.AddCommand(
		newMembersListCmd(),
		newMembersGetCmd(),
		newMembersAddCmd(),
		newMembersUpdateCmd(),
		newMembersRemoveCmd(),
	)

	return cmd
}

// newMembersListCmd creates the "members list" subcommand.
func newMembersListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list SPACE",
		Short: "List members of a space",
		Long:  "List all members of a Google Chat space. SPACE can be a space ID or full resource name (spaces/XXXX).",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newAPIClient()
			if err != nil {
				return err
			}
			f := getFormatter()
			svc := api.NewMembersService(client)

			space := args[0]
			pageSize, _ := cmd.Flags().GetInt("page-size")
			pageToken, _ := cmd.Flags().GetString("page-token")
			filter, _ := cmd.Flags().GetString("filter")
			showInvited, _ := cmd.Flags().GetBool("show-invited")
			showGroups, _ := cmd.Flags().GetBool("show-groups")
			admin, _ := cmd.Flags().GetBool("admin")
			all, _ := cmd.Flags().GetBool("all")

			if all {
				return membersListAll(cmd, svc, f, space, pageSize, filter, showInvited, showGroups, admin)
			}

			result, err := svc.List(cmd.Context(), space, pageSize, pageToken, filter, showInvited, showGroups, admin)
			if err != nil {
				return fmt.Errorf("listing members: %w", err)
			}

			if f.IsJSON() {
				return f.PrintRaw(result)
			}

			return printMembersList(f, result)
		},
	}

	cmd.Flags().Int("page-size", 100, "Maximum number of members to return")
	cmd.Flags().String("page-token", "", "Page token for pagination")
	cmd.Flags().String("filter", "", "Filter query for members")
	cmd.Flags().Bool("show-invited", false, "Include invited members")
	cmd.Flags().Bool("show-groups", false, "Include Google Groups members")
	cmd.Flags().Bool("all", false, "Fetch all pages of results")

	return cmd
}

// membersListAll fetches all pages of members and prints them.
func membersListAll(cmd *cobra.Command, svc *api.MembersService, f *output.Formatter, space string, pageSize int, filter string, showInvited, showGroups, admin bool) error {
	var allMemberships []json.RawMessage
	pageToken := ""

	for {
		result, err := svc.List(cmd.Context(), space, pageSize, pageToken, filter, showInvited, showGroups, admin)
		if err != nil {
			return fmt.Errorf("listing members: %w", err)
		}

		var page struct {
			Memberships   []json.RawMessage `json:"memberships"`
			NextPageToken string            `json:"nextPageToken"`
		}
		if err := json.Unmarshal(result, &page); err != nil {
			return fmt.Errorf("parsing response: %w", err)
		}

		allMemberships = append(allMemberships, page.Memberships...)

		if page.NextPageToken == "" {
			break
		}
		pageToken = page.NextPageToken
	}

	if f.IsJSON() {
		combined := map[string]interface{}{
			"memberships": allMemberships,
		}
		return f.Print(combined)
	}

	// Build a synthetic response for the human-readable printer.
	combined, err := json.Marshal(map[string]interface{}{
		"memberships": allMemberships,
	})
	if err != nil {
		return fmt.Errorf("marshaling combined results: %w", err)
	}

	return printMembersList(f, json.RawMessage(combined))
}

// printMembersList renders the memberships list as a human-readable table.
func printMembersList(f *output.Formatter, raw json.RawMessage) error {
	var data struct {
		Memberships []struct {
			Name   string `json:"name"`
			Member struct {
				Name        string `json:"name"`
				DisplayName string `json:"displayName"`
				Type        string `json:"type"`
			} `json:"member"`
			Role  string      `json:"role"`
			State interface{} `json:"state"`
		} `json:"memberships"`
		NextPageToken string `json:"nextPageToken"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return fmt.Errorf("parsing memberships: %w", err)
	}

	if len(data.Memberships) == 0 {
		f.PrintMessage("No members found.")
		return nil
	}

	table := output.NewTable("NAME", "MEMBER_NAME", "DISPLAY_NAME", "ROLE", "TYPE", "STATE")
	for _, m := range data.Memberships {
		state := formatMemberState(m.State)
		table.AddRow(
			m.Name,
			m.Member.Name,
			m.Member.DisplayName,
			m.Role,
			m.Member.Type,
			state,
		)
	}

	fmt.Print(table.Render())

	if data.NextPageToken != "" {
		f.PrintMessage(fmt.Sprintf("\nNext page token: %s", data.NextPageToken))
	}

	return nil
}

// formatMemberState converts a membership state value to a string.
// The state may be a string or an enum integer from the API.
func formatMemberState(state interface{}) string {
	if state == nil {
		return ""
	}
	switch v := state.(type) {
	case string:
		return v
	case float64:
		// Google API sometimes returns enum values as integers.
		switch int(v) {
		case 0:
			return "MEMBER_STATE_UNSPECIFIED"
		case 1:
			return "JOINED"
		case 2:
			return "INVITED"
		case 3:
			return "NOT_A_MEMBER"
		default:
			return fmt.Sprintf("%d", int(v))
		}
	default:
		return fmt.Sprintf("%v", v)
	}
}

// newMembersGetCmd creates the "members get" subcommand.
func newMembersGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get MEMBER",
		Short: "Get details of a space member",
		Long:  "Get detailed information about a member. MEMBER is the full resource name (e.g. spaces/XXXX/members/YYYY).",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newAPIClient()
			if err != nil {
				return err
			}
			f := getFormatter()
			svc := api.NewMembersService(client)

			name := args[0]
			admin, _ := cmd.Flags().GetBool("admin")

			result, err := svc.Get(cmd.Context(), name, admin)
			if err != nil {
				return fmt.Errorf("getting member: %w", err)
			}

			if f.IsJSON() {
				return f.PrintRaw(result)
			}

			return printMemberDetail(result)
		},
	}

	return cmd
}

// printMemberDetail renders a single membership as a detailed key-value display.
func printMemberDetail(raw json.RawMessage) error {
	var data struct {
		Name       string `json:"name"`
		CreateTime string `json:"createTime"`
		DeleteTime string `json:"deleteTime"`
		Member     struct {
			Name        string `json:"name"`
			DisplayName string `json:"displayName"`
			Type        string `json:"type"`
			DomainID    string `json:"domainId"`
		} `json:"member"`
		GroupMember struct {
			Name string `json:"name"`
		} `json:"groupMember"`
		Role  string      `json:"role"`
		State interface{} `json:"state"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return fmt.Errorf("parsing membership: %w", err)
	}

	fmt.Printf("Name:          %s\n", data.Name)
	fmt.Printf("Role:          %s\n", data.Role)
	fmt.Printf("State:         %s\n", formatMemberState(data.State))

	if data.Member.Name != "" {
		fmt.Printf("Member Name:   %s\n", data.Member.Name)
		fmt.Printf("Display Name:  %s\n", data.Member.DisplayName)
		fmt.Printf("Type:          %s\n", data.Member.Type)
		if data.Member.DomainID != "" {
			fmt.Printf("Domain ID:     %s\n", data.Member.DomainID)
		}
	}

	if data.GroupMember.Name != "" {
		fmt.Printf("Group Member:  %s\n", data.GroupMember.Name)
	}

	if data.CreateTime != "" {
		fmt.Printf("Created:       %s\n", output.FormatTime(data.CreateTime))
	}
	if data.DeleteTime != "" {
		fmt.Printf("Deleted:       %s\n", output.FormatTime(data.DeleteTime))
	}

	return nil
}

// newMembersAddCmd creates the "members add" subcommand.
func newMembersAddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add SPACE",
		Short: "Add a member to a space",
		Long:  "Add a user as a member to a Google Chat space. SPACE can be a space ID or full resource name (spaces/XXXX).",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newAPIClient()
			if err != nil {
				return err
			}
			f := getFormatter()
			svc := api.NewMembersService(client)

			space := args[0]
			user, _ := cmd.Flags().GetString("user")
			role, _ := cmd.Flags().GetString("role")
			admin, _ := cmd.Flags().GetBool("admin")

			membership := map[string]interface{}{
				"member": map[string]interface{}{
					"name": user,
					"type": "HUMAN",
				},
				"role": role,
			}

			result, err := svc.Create(cmd.Context(), space, membership, admin)
			if err != nil {
				return fmt.Errorf("adding member: %w", err)
			}

			if f.IsJSON() {
				return f.PrintRaw(result)
			}

			f.PrintSuccess(fmt.Sprintf("Member added to space %s", space))
			return printMemberDetail(result)
		},
	}

	cmd.Flags().String("user", "", "User resource name (e.g. users/123456)")
	cmd.Flags().String("role", "ROLE_MEMBER", "Member role (ROLE_MEMBER or ROLE_MANAGER)")
	_ = cmd.MarkFlagRequired("user")

	return cmd
}

// newMembersUpdateCmd creates the "members update" subcommand.
func newMembersUpdateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update MEMBER",
		Short: "Update a space member",
		Long:  "Update a member's role in a Google Chat space. MEMBER is the full resource name (e.g. spaces/XXXX/members/YYYY).",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newAPIClient()
			if err != nil {
				return err
			}
			f := getFormatter()
			svc := api.NewMembersService(client)

			name := args[0]
			role, _ := cmd.Flags().GetString("role")
			updateMask, _ := cmd.Flags().GetString("update-mask")
			admin, _ := cmd.Flags().GetBool("admin")

			membership := map[string]interface{}{
				"role": role,
			}

			result, err := svc.Patch(cmd.Context(), name, membership, updateMask, admin)
			if err != nil {
				return fmt.Errorf("updating member: %w", err)
			}

			if f.IsJSON() {
				return f.PrintRaw(result)
			}

			f.PrintSuccess(fmt.Sprintf("Member %s updated", name))
			return printMemberDetail(result)
		},
	}

	cmd.Flags().String("role", "", "Member role (ROLE_MEMBER or ROLE_MANAGER)")
	cmd.Flags().String("update-mask", "role", "Fields to update (comma-separated)")
	_ = cmd.MarkFlagRequired("role")

	return cmd
}

// newMembersRemoveCmd creates the "members remove" subcommand.
func newMembersRemoveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove MEMBER",
		Short: "Remove a member from a space",
		Long:  "Remove a member from a Google Chat space. MEMBER is the full resource name (e.g. spaces/XXXX/members/YYYY).",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newAPIClient()
			if err != nil {
				return err
			}
			f := getFormatter()
			svc := api.NewMembersService(client)

			name := args[0]
			admin, _ := cmd.Flags().GetBool("admin")
			force, _ := cmd.Flags().GetBool("force")

			if !force {
				fmt.Fprintf(os.Stderr, "Remove member %s? [y/N]: ", name)
				reader := bufio.NewReader(os.Stdin)
				answer, err := reader.ReadString('\n')
				if err != nil {
					return fmt.Errorf("reading confirmation: %w", err)
				}
				answer = strings.TrimSpace(answer)
				if answer != "y" && answer != "Y" {
					fmt.Fprintln(os.Stderr, "Cancelled.")
					return nil
				}
			}

			result, err := svc.Delete(cmd.Context(), name, admin)
			if err != nil {
				return fmt.Errorf("removing member: %w", err)
			}

			if f.IsJSON() {
				return f.PrintRaw(result)
			}

			f.PrintSuccess(fmt.Sprintf("Member %s removed", name))
			return nil
		},
	}

	cmd.Flags().Bool("force", false, "Skip confirmation prompt")

	return cmd
}
