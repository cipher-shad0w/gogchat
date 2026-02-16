package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cipher-shad0w/gogchat/internal/api"
	"github.com/cipher-shad0w/gogchat/internal/output"
)

// NewSpacesCmd creates the top-level "spaces" command with all subcommands.
func NewSpacesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "spaces",
		Aliases: []string{"space"},
		Short:   "Manage Google Chat spaces",
		Long:    "List, create, update, delete, and search Google Chat spaces.",
	}

	cmd.AddCommand(
		newSpacesListCmd(),
		newSpacesGetCmd(),
		newSpacesCreateCmd(),
		newSpacesUpdateCmd(),
		newSpacesDeleteCmd(),
		newSpacesSearchCmd(),
		newSpacesSetupCmd(),
		newSpacesFindDMCmd(),
		newSpacesCompleteImportCmd(),
	)

	return cmd
}

// ---------------------------------------------------------------------------
// spaces list
// ---------------------------------------------------------------------------

func newSpacesListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List spaces the caller is a member of",
		Long:  "List all Google Chat spaces the authenticated user is a member of.",
		RunE:  runSpacesList,
	}

	cmd.Flags().String("filter", "", "Filter spaces (e.g. spaceType = \"SPACE\")")
	cmd.Flags().Int("page-size", 100, "Maximum number of spaces to return per page")
	cmd.Flags().String("page-token", "", "Page token for pagination")
	cmd.Flags().Bool("all", false, "Automatically paginate through all results")

	return cmd
}

func runSpacesList(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient()
	if err != nil {
		return err
	}

	f := getFormatter()
	svc := api.NewSpacesService(client)
	ctx := context.Background()

	filter, _ := cmd.Flags().GetString("filter")
	pageSize, _ := cmd.Flags().GetInt("page-size")
	pageToken, _ := cmd.Flags().GetString("page-token")
	all, _ := cmd.Flags().GetBool("all")

	// When --all is set we collect every page into a single slice.
	var allSpaces []json.RawMessage

	for {
		raw, err := svc.List(ctx, filter, pageSize, pageToken)
		if err != nil {
			return fmt.Errorf("listing spaces: %w", err)
		}

		if f.IsJSON() && !all {
			return f.PrintRaw(raw)
		}

		var resp struct {
			Spaces        []json.RawMessage `json:"spaces"`
			NextPageToken string            `json:"nextPageToken"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return fmt.Errorf("parsing response: %w", err)
		}

		allSpaces = append(allSpaces, resp.Spaces...)

		if !all || resp.NextPageToken == "" {
			pageToken = resp.NextPageToken
			break
		}
		pageToken = resp.NextPageToken
	}

	// JSON mode with --all: emit aggregated result.
	if f.IsJSON() {
		return f.Print(map[string]interface{}{
			"spaces": allSpaces,
		})
	}

	if len(allSpaces) == 0 {
		f.PrintMessage("No spaces found.")
		return nil
	}

	table := output.NewTable("NAME", "DISPLAY_NAME", "TYPE", "MEMBER_COUNT", "CREATE_TIME")

	for _, raw := range allSpaces {
		var sp map[string]interface{}
		if err := json.Unmarshal(raw, &sp); err != nil {
			continue
		}
		name := spaceMapStr(sp, "name")
		displayName := spaceMapStr(sp, "displayName")
		spaceType := spaceMapStr(sp, "spaceType")
		memberCount := ""
		if mc, ok := sp["membershipCount"]; ok {
			memberCount = fmt.Sprintf("%v", mc)
		}
		createTime := output.FormatTime(spaceMapStr(sp, "createTime"))

		table.AddRow(name, displayName, spaceType, memberCount, createTime)
	}

	fmt.Print(table.Render())

	if !all && pageToken != "" {
		f.PrintMessage(fmt.Sprintf("\nMore results available. Use --page-token %s to see the next page, or use --all to fetch everything.", pageToken))
	}

	return nil
}

// ---------------------------------------------------------------------------
// spaces get
// ---------------------------------------------------------------------------

func newSpacesGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get SPACE",
		Short: "Get details about a space",
		Long:  "Get detailed information about a Google Chat space. SPACE can be a space ID or full resource name (spaces/XXXX).",
		Args:  cobra.ExactArgs(1),
		RunE:  runSpacesGet,
	}

	cmd.Flags().Bool("admin", false, "Use admin access")

	return cmd
}

func runSpacesGet(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient()
	if err != nil {
		return err
	}

	f := getFormatter()
	svc := api.NewSpacesService(client)
	ctx := context.Background()

	admin, _ := cmd.Flags().GetBool("admin")

	raw, err := svc.Get(ctx, args[0], admin)
	if err != nil {
		return fmt.Errorf("getting space: %w", err)
	}

	if f.IsJSON() {
		return f.PrintRaw(raw)
	}

	var sp map[string]interface{}
	if err := json.Unmarshal(raw, &sp); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}

	printSpaceDetail(sp)
	return nil
}

func printSpaceDetail(sp map[string]interface{}) {
	pairs := []struct{ label, key string }{
		{"Name", "name"},
		{"Display Name", "displayName"},
		{"Type", "spaceType"},
		{"Space Type", "type"},
		{"Description", "spaceDetails.description"},
		{"Guidelines", "spaceDetails.guidelines"},
		{"Threading State", "spaceThreadingState"},
		{"History State", "spaceHistoryState"},
		{"External Access", "externalUserAllowed"},
		{"Admin Installed", "adminInstalled"},
		{"Member Count", "membershipCount"},
		{"Create Time", "createTime"},
	}

	for _, p := range pairs {
		val := spaceExtractNested(sp, p.key)
		if val == "" {
			continue
		}
		if p.key == "createTime" {
			val = output.FormatTime(val)
		}
		fmt.Printf("%-20s %s\n", p.label+":", val)
	}
}

// ---------------------------------------------------------------------------
// spaces create
// ---------------------------------------------------------------------------

func newSpacesCreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new space",
		Long:  "Create a new Google Chat space with the given display name and type.",
		RunE:  runSpacesCreate,
	}

	cmd.Flags().String("display-name", "", "Display name for the space (required)")
	cmd.Flags().String("space-type", "SPACE", "Space type (SPACE, GROUP_CHAT, DIRECT_MESSAGE)")
	cmd.Flags().String("description", "", "Description for the space")
	cmd.Flags().String("request-id", "", "Unique request ID for idempotency")

	_ = cmd.MarkFlagRequired("display-name")

	return cmd
}

func runSpacesCreate(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient()
	if err != nil {
		return err
	}

	f := getFormatter()
	svc := api.NewSpacesService(client)
	ctx := context.Background()

	displayName, _ := cmd.Flags().GetString("display-name")
	spaceType, _ := cmd.Flags().GetString("space-type")
	description, _ := cmd.Flags().GetString("description")
	requestID, _ := cmd.Flags().GetString("request-id")

	space := map[string]interface{}{
		"displayName": displayName,
		"spaceType":   spaceType,
	}

	if description != "" {
		space["spaceDetails"] = map[string]interface{}{
			"description": description,
		}
	}

	raw, err := svc.Create(ctx, space, requestID)
	if err != nil {
		return fmt.Errorf("creating space: %w", err)
	}

	if f.IsJSON() {
		return f.PrintRaw(raw)
	}

	var sp map[string]interface{}
	if err := json.Unmarshal(raw, &sp); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}

	f.PrintSuccess(fmt.Sprintf("Space created: %s", spaceMapStr(sp, "name")))
	printSpaceDetail(sp)
	return nil
}

// ---------------------------------------------------------------------------
// spaces update
// ---------------------------------------------------------------------------

func newSpacesUpdateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update SPACE",
		Short: "Update an existing space",
		Long:  "Update fields of an existing Google Chat space. SPACE can be a space ID or full resource name (spaces/XXXX).",
		Args:  cobra.ExactArgs(1),
		RunE:  runSpacesUpdate,
	}

	cmd.Flags().String("display-name", "", "New display name")
	cmd.Flags().String("description", "", "New description")
	cmd.Flags().String("history-state", "", "History state (HISTORY_ON or HISTORY_OFF)")
	cmd.Flags().String("update-mask", "", "Comma-separated field mask (auto-detected if not set)")
	cmd.Flags().Bool("admin", false, "Use admin access")

	return cmd
}

func runSpacesUpdate(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient()
	if err != nil {
		return err
	}

	f := getFormatter()
	svc := api.NewSpacesService(client)
	ctx := context.Background()

	admin, _ := cmd.Flags().GetBool("admin")
	updateMask, _ := cmd.Flags().GetString("update-mask")

	space := map[string]interface{}{}
	var maskParts []string

	if cmd.Flags().Changed("display-name") {
		displayName, _ := cmd.Flags().GetString("display-name")
		space["displayName"] = displayName
		maskParts = append(maskParts, "displayName")
	}

	if cmd.Flags().Changed("description") {
		description, _ := cmd.Flags().GetString("description")
		if _, ok := space["spaceDetails"]; !ok {
			space["spaceDetails"] = map[string]interface{}{}
		}
		details := space["spaceDetails"].(map[string]interface{})
		details["description"] = description
		maskParts = append(maskParts, "spaceDetails.description")
	}

	if cmd.Flags().Changed("history-state") {
		historyState, _ := cmd.Flags().GetString("history-state")
		space["spaceHistoryState"] = historyState
		maskParts = append(maskParts, "spaceHistoryState")
	}

	// Auto-build update mask from changed flags if not explicitly provided.
	if updateMask == "" && len(maskParts) > 0 {
		updateMask = strings.Join(maskParts, ",")
	}

	if updateMask == "" {
		return fmt.Errorf("no fields to update; use --display-name, --description, --history-state, or --update-mask")
	}

	raw, err := svc.Patch(ctx, args[0], space, updateMask, admin)
	if err != nil {
		return fmt.Errorf("updating space: %w", err)
	}

	if f.IsJSON() {
		return f.PrintRaw(raw)
	}

	var sp map[string]interface{}
	if err := json.Unmarshal(raw, &sp); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}

	f.PrintSuccess(fmt.Sprintf("Space updated: %s", spaceMapStr(sp, "name")))
	printSpaceDetail(sp)
	return nil
}

// ---------------------------------------------------------------------------
// spaces delete
// ---------------------------------------------------------------------------

func newSpacesDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete SPACE",
		Short: "Delete a space",
		Long:  "Delete a Google Chat space. SPACE can be a space ID or full resource name (spaces/XXXX).",
		Args:  cobra.ExactArgs(1),
		RunE:  runSpacesDelete,
	}

	cmd.Flags().Bool("admin", false, "Use admin access")
	cmd.Flags().Bool("force", false, "Skip confirmation prompt")

	return cmd
}

func runSpacesDelete(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient()
	if err != nil {
		return err
	}

	f := getFormatter()
	svc := api.NewSpacesService(client)
	ctx := context.Background()

	admin, _ := cmd.Flags().GetBool("admin")
	force, _ := cmd.Flags().GetBool("force")

	spaceName := api.NormalizeName(args[0], "spaces/")

	if !force {
		fmt.Printf("Are you sure you want to delete space %s? [y/N]: ", spaceName)
		reader := bufio.NewReader(os.Stdin)
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(answer)
		if answer != "y" && answer != "Y" {
			fmt.Println("Delete cancelled.")
			return nil
		}
	}

	raw, err := svc.Delete(ctx, spaceName, admin)
	if err != nil {
		return fmt.Errorf("deleting space: %w", err)
	}

	if f.IsJSON() {
		return f.PrintRaw(raw)
	}

	f.PrintSuccess(fmt.Sprintf("Space deleted: %s", spaceName))
	return nil
}

// ---------------------------------------------------------------------------
// spaces search
// ---------------------------------------------------------------------------

func newSpacesSearchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "search",
		Short: "Search for spaces (admin)",
		Long:  "Search for Google Chat spaces visible to the caller. Requires admin access.",
		RunE:  runSpacesSearch,
	}

	cmd.Flags().String("query", "", "Search query (required)")
	cmd.Flags().Int("page-size", 100, "Maximum number of spaces per page")
	cmd.Flags().String("page-token", "", "Page token for pagination")
	cmd.Flags().String("order-by", "", "Order results (e.g. \"membershipCount desc\")")
	cmd.Flags().Bool("admin", true, "Use admin access (default true for search)")

	_ = cmd.MarkFlagRequired("query")

	return cmd
}

func runSpacesSearch(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient()
	if err != nil {
		return err
	}

	f := getFormatter()
	svc := api.NewSpacesService(client)
	ctx := context.Background()

	query, _ := cmd.Flags().GetString("query")
	pageSize, _ := cmd.Flags().GetInt("page-size")
	pageToken, _ := cmd.Flags().GetString("page-token")
	orderBy, _ := cmd.Flags().GetString("order-by")
	admin, _ := cmd.Flags().GetBool("admin")

	raw, err := svc.Search(ctx, query, pageSize, pageToken, orderBy, admin)
	if err != nil {
		return fmt.Errorf("searching spaces: %w", err)
	}

	if f.IsJSON() {
		return f.PrintRaw(raw)
	}

	var resp struct {
		Spaces        []json.RawMessage `json:"spaces"`
		NextPageToken string            `json:"nextPageToken"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}

	if len(resp.Spaces) == 0 {
		f.PrintMessage("No spaces found.")
		return nil
	}

	table := output.NewTable("NAME", "DISPLAY_NAME", "TYPE", "MEMBER_COUNT", "CREATE_TIME")

	for _, raw := range resp.Spaces {
		var sp map[string]interface{}
		if err := json.Unmarshal(raw, &sp); err != nil {
			continue
		}
		name := spaceMapStr(sp, "name")
		displayName := spaceMapStr(sp, "displayName")
		spaceType := spaceMapStr(sp, "spaceType")
		memberCount := ""
		if mc, ok := sp["membershipCount"]; ok {
			memberCount = fmt.Sprintf("%v", mc)
		}
		createTime := output.FormatTime(spaceMapStr(sp, "createTime"))

		table.AddRow(name, displayName, spaceType, memberCount, createTime)
	}

	fmt.Print(table.Render())

	if resp.NextPageToken != "" {
		f.PrintMessage(fmt.Sprintf("\nMore results available. Use --page-token %s to see the next page.", resp.NextPageToken))
	}

	return nil
}

// ---------------------------------------------------------------------------
// spaces setup
// ---------------------------------------------------------------------------

func newSpacesSetupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Create a space and add members in one call",
		Long:  "Set up a Google Chat space and add initial members in a single API call.",
		RunE:  runSpacesSetup,
	}

	cmd.Flags().String("display-name", "", "Display name for the space")
	cmd.Flags().String("space-type", "SPACE", "Space type (SPACE, GROUP_CHAT, DIRECT_MESSAGE)")
	cmd.Flags().StringSlice("members", nil, "User resource names to add (e.g. users/12345)")

	return cmd
}

func runSpacesSetup(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient()
	if err != nil {
		return err
	}

	f := getFormatter()
	svc := api.NewSpacesService(client)
	ctx := context.Background()

	displayName, _ := cmd.Flags().GetString("display-name")
	spaceType, _ := cmd.Flags().GetString("space-type")
	members, _ := cmd.Flags().GetStringSlice("members")

	space := map[string]interface{}{
		"spaceType": spaceType,
	}
	if displayName != "" {
		space["displayName"] = displayName
	}

	request := map[string]interface{}{
		"space": space,
	}

	if len(members) > 0 {
		memberships := make([]map[string]interface{}, 0, len(members))
		for _, m := range members {
			memberships = append(memberships, map[string]interface{}{
				"member": map[string]interface{}{
					"name": m,
					"type": "HUMAN",
				},
			})
		}
		request["memberships"] = memberships
	}

	raw, err := svc.Setup(ctx, request)
	if err != nil {
		return fmt.Errorf("setting up space: %w", err)
	}

	if f.IsJSON() {
		return f.PrintRaw(raw)
	}

	var sp map[string]interface{}
	if err := json.Unmarshal(raw, &sp); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}

	f.PrintSuccess(fmt.Sprintf("Space created: %s", spaceMapStr(sp, "name")))
	printSpaceDetail(sp)
	return nil
}

// ---------------------------------------------------------------------------
// spaces find-dm
// ---------------------------------------------------------------------------

func newSpacesFindDMCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "find-dm",
		Short: "Find a direct message space with a user",
		Long:  "Find an existing direct message space between the authenticated user and the specified user.",
		RunE:  runSpacesFindDM,
	}

	cmd.Flags().String("user", "", "User resource name (e.g. users/12345) (required)")

	_ = cmd.MarkFlagRequired("user")

	return cmd
}

func runSpacesFindDM(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient()
	if err != nil {
		return err
	}

	f := getFormatter()
	svc := api.NewSpacesService(client)
	ctx := context.Background()

	user, _ := cmd.Flags().GetString("user")

	raw, err := svc.FindDirectMessage(ctx, user)
	if err != nil {
		return fmt.Errorf("finding direct message: %w", err)
	}

	if f.IsJSON() {
		return f.PrintRaw(raw)
	}

	var sp map[string]interface{}
	if err := json.Unmarshal(raw, &sp); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}

	printSpaceDetail(sp)
	return nil
}

// ---------------------------------------------------------------------------
// spaces complete-import
// ---------------------------------------------------------------------------

func newSpacesCompleteImportCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "complete-import SPACE",
		Short: "Complete the import process for a space",
		Long:  "Complete the import process for a Google Chat space, making it visible to users and allowing new messages.",
		Args:  cobra.ExactArgs(1),
		RunE:  runSpacesCompleteImport,
	}
}

func runSpacesCompleteImport(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient()
	if err != nil {
		return err
	}

	f := getFormatter()
	svc := api.NewSpacesService(client)
	ctx := context.Background()

	raw, err := svc.CompleteImport(ctx, args[0])
	if err != nil {
		return fmt.Errorf("completing import: %w", err)
	}

	if f.IsJSON() {
		return f.PrintRaw(raw)
	}

	var sp map[string]interface{}
	if err := json.Unmarshal(raw, &sp); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}

	spaceName := api.NormalizeName(args[0], "spaces/")
	f.PrintSuccess(fmt.Sprintf("Import completed for space: %s", spaceName))
	printSpaceDetail(sp)
	return nil
}

// ---------------------------------------------------------------------------
// helpers (spaces-specific)
// ---------------------------------------------------------------------------

// spaceMapStr safely extracts a string value from a map[string]interface{}.
func spaceMapStr(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
		return fmt.Sprintf("%v", v)
	}
	return ""
}

// spaceExtractNested extracts a value from a map supporting dot-notation for
// one level of nesting (e.g. "spaceDetails.description").
func spaceExtractNested(m map[string]interface{}, key string) string {
	parts := strings.SplitN(key, ".", 2)
	if len(parts) == 1 {
		return spaceMapStr(m, key)
	}

	nested, ok := m[parts[0]]
	if !ok {
		return ""
	}

	nestedMap, ok := nested.(map[string]interface{})
	if !ok {
		return ""
	}

	return spaceMapStr(nestedMap, parts[1])
}
