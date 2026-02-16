package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/cipher-shad0w/gogchat/internal/api"
)

// NewAttachmentsCmd creates the top-level "attachments" command with the get
// subcommand.
func NewAttachmentsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "attachments",
		Short: "Manage message attachments",
		Long:  "View metadata for attachments on Google Chat messages.",
	}

	cmd.AddCommand(
		newAttachmentsGetCmd(),
	)

	return cmd
}

// newAttachmentsGetCmd creates the "attachments get" subcommand.
func newAttachmentsGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get ATTACHMENT",
		Short: "Get attachment metadata",
		Long:  "Get metadata for a message attachment. ATTACHMENT is the full attachment resource name (spaces/{space}/messages/{message}/attachments/{attachment}).",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newAPIClient()
			if err != nil {
				return err
			}
			formatter := getFormatter()
			svc := api.NewAttachmentsService(client)

			name := args[0]

			raw, err := svc.Get(cmd.Context(), name)
			if err != nil {
				return fmt.Errorf("getting attachment: %w", err)
			}

			if formatter.IsJSON() {
				return formatter.PrintRaw(raw)
			}

			// Parse the attachment for human-readable output.
			var attachment struct {
				Name              string `json:"name"`
				ContentName       string `json:"contentName"`
				ContentType       string `json:"contentType"`
				DownloadURI       string `json:"downloadUri"`
				Source            string `json:"source"`
				ThumbnailURI      string `json:"thumbnailUri"`
				AttachmentDataRef struct {
					ResourceName string `json:"resourceName"`
				} `json:"attachmentDataRef"`
				DriveDataRef struct {
					DriveFileID string `json:"driveFileId"`
				} `json:"driveDataRef"`
			}
			if err := json.Unmarshal(raw, &attachment); err != nil {
				return fmt.Errorf("parsing response: %w", err)
			}

			fmt.Printf("Name:          %s\n", attachment.Name)
			fmt.Printf("Content Name:  %s\n", attachment.ContentName)
			fmt.Printf("Content Type:  %s\n", attachment.ContentType)
			fmt.Printf("Download URI:  %s\n", attachment.DownloadURI)
			fmt.Printf("Source:        %s\n", attachment.Source)
			fmt.Printf("Thumbnail URI: %s\n", attachment.ThumbnailURI)

			// Show size if available from the raw JSON.
			var rawMap map[string]json.RawMessage
			if err := json.Unmarshal(raw, &rawMap); err == nil {
				if sizeRaw, ok := rawMap["sizeBytes"]; ok {
					fmt.Printf("Size:          %s bytes\n", string(sizeRaw))
				}
			}

			return nil
		},
	}

	return cmd
}
