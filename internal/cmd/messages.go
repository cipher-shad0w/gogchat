package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/cipher-shad0w/gogchat/internal/api"
	"github.com/cipher-shad0w/gogchat/internal/output"
	"github.com/spf13/cobra"
)

// NewMessagesCmd returns the top-level "messages" command with all subcommands.
func NewMessagesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "messages",
		Aliases: []string{"msg"},
		Short:   "Manage messages in Google Chat spaces",
		Long:    "List, get, send, update, replace, and delete messages in Google Chat spaces.",
	}

	cmd.AddCommand(
		newMessagesListCmd(),
		newMessagesGetCmd(),
		newMessagesSendCmd(),
		newMessagesUpdateCmd(),
		newMessagesDeleteCmd(),
		newMessagesReplaceCmd(),
	)

	return cmd
}

// ---------------------------------------------------------------------------
// messages list
// ---------------------------------------------------------------------------

func newMessagesListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list SPACE",
		Short: "List messages in a space",
		Long:  "List messages in a Google Chat space. SPACE can be a space ID or full resource name.",
		Args:  cobra.ExactArgs(1),
		RunE:  runMessagesList,
	}

	flags := cmd.Flags()
	flags.Int("page-size", 25, "Maximum number of messages to return per page")
	flags.String("page-token", "", "Token for retrieving the next page of results")
	flags.String("filter", "", "Filter expression for messages")
	flags.String("order-by", "", "Order results (e.g. 'createTime desc')")
	flags.Bool("show-deleted", false, "Include deleted messages in results")
	flags.Bool("all", false, "Auto-paginate through all results")

	return cmd
}

func runMessagesList(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient()
	if err != nil {
		return err
	}
	f := getFormatter()
	svc := api.NewMessagesService(client)
	ctx := context.Background()

	parent := args[0]
	pageSize, _ := cmd.Flags().GetInt("page-size")
	pageToken, _ := cmd.Flags().GetString("page-token")
	filter, _ := cmd.Flags().GetString("filter")
	orderBy, _ := cmd.Flags().GetString("order-by")
	showDeleted, _ := cmd.Flags().GetBool("show-deleted")
	all, _ := cmd.Flags().GetBool("all")

	// Collect all pages when --all is set, otherwise fetch a single page.
	var allMessages []json.RawMessage

	for {
		raw, err := svc.List(ctx, parent, pageSize, pageToken, filter, orderBy, showDeleted)
		if err != nil {
			return fmt.Errorf("listing messages: %w", err)
		}

		if f.IsJSON() && !all {
			return f.PrintRaw(raw)
		}

		var resp struct {
			Messages      []json.RawMessage `json:"messages"`
			NextPageToken string            `json:"nextPageToken"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return fmt.Errorf("parsing response: %w", err)
		}

		allMessages = append(allMessages, resp.Messages...)

		if !all || resp.NextPageToken == "" {
			pageToken = resp.NextPageToken
			break
		}
		pageToken = resp.NextPageToken
	}

	// JSON mode with --all: emit aggregated result.
	if f.IsJSON() {
		return f.Print(map[string]interface{}{
			"messages": allMessages,
		})
	}

	if len(allMessages) == 0 {
		f.PrintMessage("No messages found.")
		return nil
	}

	table := output.NewTable("NAME", "SENDER", "TEXT", "CREATE_TIME")

	for _, raw := range allMessages {
		var msg struct {
			Name       string `json:"name"`
			Text       string `json:"text"`
			CreateTime string `json:"createTime"`
			Sender     struct {
				DisplayName string `json:"displayName"`
				Name        string `json:"name"`
			} `json:"sender"`
		}
		if err := json.Unmarshal(raw, &msg); err != nil {
			continue
		}

		sender := msg.Sender.DisplayName
		if sender == "" {
			sender = msg.Sender.Name
		}

		table.AddRow(
			msg.Name,
			sender,
			output.Truncate(msg.Text, 60),
			output.FormatTime(msg.CreateTime),
		)
	}

	f.PrintMessage(table.Render())
	return nil
}

// ---------------------------------------------------------------------------
// messages get
// ---------------------------------------------------------------------------

func newMessagesGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get MESSAGE",
		Short: "Get a message by name",
		Long:  "Get a single message. MESSAGE must be the full resource name (spaces/{space}/messages/{message}).",
		Args:  cobra.ExactArgs(1),
		RunE:  runMessagesGet,
	}

	return cmd
}

func runMessagesGet(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient()
	if err != nil {
		return err
	}
	f := getFormatter()
	svc := api.NewMessagesService(client)

	raw, err := svc.Get(context.Background(), args[0])
	if err != nil {
		return fmt.Errorf("getting message: %w", err)
	}

	if f.IsJSON() {
		return f.PrintRaw(raw)
	}

	var msg struct {
		Name           string `json:"name"`
		Text           string `json:"text"`
		CreateTime     string `json:"createTime"`
		LastUpdateTime string `json:"lastUpdateTime"`
		Sender         struct {
			DisplayName string `json:"displayName"`
			Name        string `json:"name"`
		} `json:"sender"`
		Thread struct {
			Name string `json:"name"`
		} `json:"thread"`
	}
	if err := json.Unmarshal(raw, &msg); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}

	sender := msg.Sender.DisplayName
	if sender == "" {
		sender = msg.Sender.Name
	}

	f.PrintMessage(fmt.Sprintf("Name:             %s", msg.Name))
	f.PrintMessage(fmt.Sprintf("Sender:           %s", sender))
	f.PrintMessage(fmt.Sprintf("Text:             %s", msg.Text))
	f.PrintMessage(fmt.Sprintf("Create Time:      %s", output.FormatTime(msg.CreateTime)))
	f.PrintMessage(fmt.Sprintf("Last Update Time: %s", output.FormatTime(msg.LastUpdateTime)))
	f.PrintMessage(fmt.Sprintf("Thread Name:      %s", msg.Thread.Name))

	return nil
}

// ---------------------------------------------------------------------------
// messages send
// ---------------------------------------------------------------------------

func newMessagesSendCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "send SPACE",
		Short: "Send a message to a space",
		Long:  "Send a new message to a Google Chat space. SPACE can be a space ID or full resource name.",
		Args:  cobra.ExactArgs(1),
		RunE:  runMessagesSend,
	}

	flags := cmd.Flags()
	flags.String("text", "", "Message text content (required)")
	flags.String("thread-key", "", "Thread key for threading messages")
	flags.String("request-id", "", "Unique request ID for idempotency")
	flags.String("message-id", "", "Custom message ID")
	flags.String("reply-option", "", "Reply option (REPLY_MESSAGE_FALLBACK_TO_NEW_THREAD or REPLY_MESSAGE_OR_FAIL)")
	_ = cmd.MarkFlagRequired("text")

	return cmd
}

func runMessagesSend(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient()
	if err != nil {
		return err
	}
	f := getFormatter()
	svc := api.NewMessagesService(client)

	text, _ := cmd.Flags().GetString("text")
	threadKey, _ := cmd.Flags().GetString("thread-key")
	requestID, _ := cmd.Flags().GetString("request-id")
	messageID, _ := cmd.Flags().GetString("message-id")
	replyOption, _ := cmd.Flags().GetString("reply-option")

	body := map[string]interface{}{
		"text": text,
	}

	raw, err := svc.Create(context.Background(), args[0], body, threadKey, requestID, messageID, replyOption)
	if err != nil {
		return fmt.Errorf("sending message: %w", err)
	}

	if f.IsJSON() {
		return f.PrintRaw(raw)
	}

	var msg struct {
		Name       string `json:"name"`
		Text       string `json:"text"`
		CreateTime string `json:"createTime"`
		Sender     struct {
			DisplayName string `json:"displayName"`
			Name        string `json:"name"`
		} `json:"sender"`
		Thread struct {
			Name string `json:"name"`
		} `json:"thread"`
	}
	if err := json.Unmarshal(raw, &msg); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}

	sender := msg.Sender.DisplayName
	if sender == "" {
		sender = msg.Sender.Name
	}

	f.PrintSuccess("Message sent")
	f.PrintMessage(fmt.Sprintf("Name:        %s", msg.Name))
	f.PrintMessage(fmt.Sprintf("Sender:      %s", sender))
	f.PrintMessage(fmt.Sprintf("Text:        %s", output.Truncate(msg.Text, 80)))
	f.PrintMessage(fmt.Sprintf("Create Time: %s", output.FormatTime(msg.CreateTime)))
	if msg.Thread.Name != "" {
		f.PrintMessage(fmt.Sprintf("Thread:      %s", msg.Thread.Name))
	}

	return nil
}

// ---------------------------------------------------------------------------
// messages update (PATCH)
// ---------------------------------------------------------------------------

func newMessagesUpdateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update MESSAGE",
		Short: "Update a message",
		Long:  "Partially update a message using PATCH. MESSAGE must be the full resource name (spaces/{space}/messages/{message}).",
		Args:  cobra.ExactArgs(1),
		RunE:  runMessagesUpdate,
	}

	flags := cmd.Flags()
	flags.String("text", "", "New message text (required)")
	flags.String("update-mask", "text", "Comma-separated list of fields to update")
	flags.Bool("allow-missing", false, "Allow updating a message that may not exist yet")
	_ = cmd.MarkFlagRequired("text")

	return cmd
}

func runMessagesUpdate(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient()
	if err != nil {
		return err
	}
	f := getFormatter()
	svc := api.NewMessagesService(client)

	text, _ := cmd.Flags().GetString("text")
	updateMask, _ := cmd.Flags().GetString("update-mask")
	allowMissing, _ := cmd.Flags().GetBool("allow-missing")

	body := map[string]interface{}{
		"text": text,
	}

	raw, err := svc.Patch(context.Background(), args[0], body, updateMask, allowMissing)
	if err != nil {
		return fmt.Errorf("updating message: %w", err)
	}

	if f.IsJSON() {
		return f.PrintRaw(raw)
	}

	var msg struct {
		Name           string `json:"name"`
		Text           string `json:"text"`
		LastUpdateTime string `json:"lastUpdateTime"`
	}
	if err := json.Unmarshal(raw, &msg); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}

	f.PrintSuccess("Message updated")
	f.PrintMessage(fmt.Sprintf("Name:             %s", msg.Name))
	f.PrintMessage(fmt.Sprintf("Text:             %s", output.Truncate(msg.Text, 80)))
	f.PrintMessage(fmt.Sprintf("Last Update Time: %s", output.FormatTime(msg.LastUpdateTime)))

	return nil
}

// ---------------------------------------------------------------------------
// messages delete
// ---------------------------------------------------------------------------

func newMessagesDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete MESSAGE",
		Short: "Delete a message",
		Long:  "Delete a message. MESSAGE must be the full resource name (spaces/{space}/messages/{message}).",
		Args:  cobra.ExactArgs(1),
		RunE:  runMessagesDelete,
	}

	flags := cmd.Flags()
	flags.Bool("force", false, "Skip confirmation prompt")
	flags.Bool("force-threads", false, "Also delete threaded replies (API force parameter)")

	return cmd
}

func runMessagesDelete(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient()
	if err != nil {
		return err
	}
	f := getFormatter()
	svc := api.NewMessagesService(client)

	force, _ := cmd.Flags().GetBool("force")
	forceThreads, _ := cmd.Flags().GetBool("force-threads")
	name := args[0]

	// Confirmation prompt unless --force is set.
	if !force {
		fmt.Fprintf(os.Stderr, "Delete message %s? [y/N] ", name)
		reader := bufio.NewReader(os.Stdin)
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer != "y" && answer != "yes" {
			f.PrintMessage("Cancelled.")
			return nil
		}
	}

	raw, err := svc.Delete(context.Background(), name, forceThreads)
	if err != nil {
		return fmt.Errorf("deleting message: %w", err)
	}

	if f.IsJSON() {
		return f.PrintRaw(raw)
	}

	f.PrintSuccess(fmt.Sprintf("Message %s deleted.", name))
	return nil
}

// ---------------------------------------------------------------------------
// messages replace (PUT)
// ---------------------------------------------------------------------------

func newMessagesReplaceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "replace MESSAGE",
		Short: "Replace a message",
		Long:  "Fully replace a message using PUT. MESSAGE must be the full resource name (spaces/{space}/messages/{message}).",
		Args:  cobra.ExactArgs(1),
		RunE:  runMessagesReplace,
	}

	flags := cmd.Flags()
	flags.String("text", "", "New message text (required)")
	flags.String("update-mask", "", "Comma-separated list of fields to update")
	flags.Bool("allow-missing", false, "Allow replacing a message that may not exist yet")
	_ = cmd.MarkFlagRequired("text")

	return cmd
}

func runMessagesReplace(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient()
	if err != nil {
		return err
	}
	f := getFormatter()
	svc := api.NewMessagesService(client)

	text, _ := cmd.Flags().GetString("text")
	updateMask, _ := cmd.Flags().GetString("update-mask")
	allowMissing, _ := cmd.Flags().GetBool("allow-missing")

	body := map[string]interface{}{
		"text": text,
	}

	raw, err := svc.Update(context.Background(), args[0], body, updateMask, allowMissing)
	if err != nil {
		return fmt.Errorf("replacing message: %w", err)
	}

	if f.IsJSON() {
		return f.PrintRaw(raw)
	}

	var msg struct {
		Name           string `json:"name"`
		Text           string `json:"text"`
		LastUpdateTime string `json:"lastUpdateTime"`
	}
	if err := json.Unmarshal(raw, &msg); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}

	f.PrintSuccess("Message replaced")
	f.PrintMessage(fmt.Sprintf("Name:             %s", msg.Name))
	f.PrintMessage(fmt.Sprintf("Text:             %s", output.Truncate(msg.Text, 80)))
	f.PrintMessage(fmt.Sprintf("Last Update Time: %s", output.FormatTime(msg.LastUpdateTime)))

	return nil
}
