package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/cipher-shad0w/gogchat/internal/api"
	"github.com/cipher-shad0w/gogchat/internal/output"
)

// NewEventsCmd creates the top-level "events" command with list and get
// subcommands for space events.
func NewEventsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "events",
		Short: "Manage space events",
		Long:  "List and retrieve events from Google Chat spaces.",
	}

	cmd.AddCommand(
		newEventsListCmd(),
		newEventsGetCmd(),
	)

	return cmd
}

// newEventsListCmd creates the "events list" subcommand.
func newEventsListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list SPACE",
		Short: "List events in a space",
		Long:  "List events from the specified space. SPACE is the space name or ID. The --filter flag is required and must include an event_type filter.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newAPIClient()
			if err != nil {
				return err
			}
			formatter := getFormatter()
			svc := api.NewEventsService(client)

			parent := args[0]
			filter, _ := cmd.Flags().GetString("filter")
			pageSize, _ := cmd.Flags().GetInt("page-size")
			pageToken, _ := cmd.Flags().GetString("page-token")
			all, _ := cmd.Flags().GetBool("all")

			ctx := cmd.Context()

			var allEvents []json.RawMessage

			for {
				raw, err := svc.List(ctx, parent, filter, pageSize, pageToken)
				if err != nil {
					return fmt.Errorf("listing events: %w", err)
				}

				if formatter.IsJSON() && !all {
					return formatter.PrintRaw(raw)
				}

				var resp struct {
					SpaceEvents []json.RawMessage `json:"spaceEvents"`
					NextPage    string            `json:"nextPageToken"`
				}
				if err := json.Unmarshal(raw, &resp); err != nil {
					return fmt.Errorf("parsing response: %w", err)
				}

				allEvents = append(allEvents, resp.SpaceEvents...)

				if !all || resp.NextPage == "" {
					pageToken = resp.NextPage
					break
				}
				pageToken = resp.NextPage
			}

			if formatter.IsJSON() {
				// --all + --json: emit collected events as a JSON array.
				return formatter.Print(allEvents)
			}

			if len(allEvents) == 0 {
				formatter.PrintMessage("No events found.")
				return nil
			}

			table := output.NewTable("EVENT_NAME", "EVENT_TYPE", "EVENT_TIME")
			for _, e := range allEvents {
				var event struct {
					Name      string `json:"name"`
					EventType string `json:"eventType"`
					EventTime string `json:"eventTime"`
				}
				if err := json.Unmarshal(e, &event); err != nil {
					continue
				}

				table.AddRow(event.Name, event.EventType, output.FormatTime(event.EventTime))
			}

			fmt.Print(table.Render())

			if !all && pageToken != "" {
				formatter.PrintMessage(fmt.Sprintf("\nMore results available. Use --page-token %s to see the next page, or use --all to fetch everything.", pageToken))
			}

			return nil
		},
	}

	cmd.Flags().String("filter", "", "Filter for events (required, must include event_type)")
	_ = cmd.MarkFlagRequired("filter")
	cmd.Flags().Int("page-size", 0, "Maximum number of events to return per page")
	cmd.Flags().String("page-token", "", "Page token for pagination")
	cmd.Flags().Bool("all", false, "Fetch all pages of results")

	return cmd
}

// newEventsGetCmd creates the "events get" subcommand.
func newEventsGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get EVENT",
		Short: "Get a space event",
		Long:  "Retrieve details of a specific space event. EVENT is the full event resource name (spaces/{space}/spaceEvents/{spaceEvent}).",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newAPIClient()
			if err != nil {
				return err
			}
			formatter := getFormatter()
			svc := api.NewEventsService(client)

			name := args[0]

			raw, err := svc.Get(cmd.Context(), name)
			if err != nil {
				return fmt.Errorf("getting event: %w", err)
			}

			if formatter.IsJSON() {
				return formatter.PrintRaw(raw)
			}

			var event struct {
				Name      string          `json:"name"`
				EventType string          `json:"eventType"`
				EventTime string          `json:"eventTime"`
				Payload   json.RawMessage `json:"-"`
			}
			if err := json.Unmarshal(raw, &event); err != nil {
				return fmt.Errorf("parsing response: %w", err)
			}

			// Build a payload summary from any known payload fields.
			payloadSummary := summarizeEventPayload(raw)

			fmt.Printf("Name:        %s\n", event.Name)
			fmt.Printf("Event Type:  %s\n", event.EventType)
			fmt.Printf("Event Time:  %s\n", output.FormatTime(event.EventTime))
			if payloadSummary != "" {
				fmt.Printf("Payload:     %s\n", payloadSummary)
			}

			return nil
		},
	}

	return cmd
}

// summarizeEventPayload extracts a short summary from the event payload.
// Google Chat events embed their payload under a field keyed by the event
// category (e.g. "messageCreatedEventData", "membershipCreatedEventData").
func summarizeEventPayload(raw json.RawMessage) string {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return ""
	}

	// Known payload field suffixes used in the Chat API.
	payloadKeys := []string{
		"messageCreatedEventData",
		"messageUpdatedEventData",
		"messageDeletedEventData",
		"messageBatchCreatedEventData",
		"messageBatchUpdatedEventData",
		"messageBatchDeletedEventData",
		"spaceUpdatedEventData",
		"spaceBatchUpdatedEventData",
		"membershipCreatedEventData",
		"membershipUpdatedEventData",
		"membershipDeletedEventData",
		"membershipBatchCreatedEventData",
		"membershipBatchUpdatedEventData",
		"membershipBatchDeletedEventData",
		"reactionCreatedEventData",
		"reactionDeletedEventData",
		"reactionBatchCreatedEventData",
		"reactionBatchDeletedEventData",
	}

	for _, key := range payloadKeys {
		if data, ok := fields[key]; ok {
			return fmt.Sprintf("%s: %s", key, output.Truncate(string(data), 80))
		}
	}

	return ""
}
