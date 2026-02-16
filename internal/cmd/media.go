package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cipher-shad0w/gogchat/internal/api"
)

// NewMediaCmd creates the top-level "media" command with upload and download
// subcommands.
func NewMediaCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "media",
		Short: "Upload and download media",
		Long:  "Upload files as attachments to Google Chat spaces and download media resources.",
	}

	cmd.AddCommand(
		newMediaUploadCmd(),
		newMediaDownloadCmd(),
	)

	return cmd
}

// newMediaUploadCmd creates the "media upload" subcommand.
func newMediaUploadCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "upload SPACE",
		Short: "Upload a file to a space",
		Long:  "Upload a file as an attachment to the specified Google Chat space. SPACE is the space resource name (spaces/{space}) or just the space ID.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newAPIClient()
			if err != nil {
				return err
			}
			formatter := getFormatter()
			svc := api.NewMediaService(client)

			parent := args[0]
			filePath, _ := cmd.Flags().GetString("file")

			// Validate that the file exists before uploading.
			info, err := os.Stat(filePath)
			if err != nil {
				if os.IsNotExist(err) {
					return fmt.Errorf("file not found: %s", filePath)
				}
				return fmt.Errorf("checking file %s: %w", filePath, err)
			}
			if info.IsDir() {
				return fmt.Errorf("%s is a directory, not a file", filePath)
			}

			raw, err := svc.Upload(cmd.Context(), parent, filePath)
			if err != nil {
				return fmt.Errorf("uploading media: %w", err)
			}

			if formatter.IsJSON() {
				return formatter.PrintRaw(raw)
			}

			// Parse and display the upload result.
			var result struct {
				AttachmentDataRef struct {
					ResourceName string `json:"resourceName"`
				} `json:"attachmentDataRef"`
			}
			if err := json.Unmarshal(raw, &result); err != nil {
				// If the response doesn't match expected structure, show raw.
				formatter.PrintSuccess("File uploaded successfully!")
				return formatter.PrintRaw(raw)
			}

			formatter.PrintSuccess("File uploaded successfully!")
			fmt.Printf("Resource Name: %s\n", result.AttachmentDataRef.ResourceName)
			fmt.Printf("Source File:   %s\n", filePath)
			fmt.Printf("File Size:     %d bytes\n", info.Size())

			return nil
		},
	}

	cmd.Flags().String("file", "", "Path to the file to upload (required)")
	_ = cmd.MarkFlagRequired("file")

	return cmd
}

// newMediaDownloadCmd creates the "media download" subcommand.
func newMediaDownloadCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "download RESOURCE",
		Short: "Download a media resource",
		Long:  "Download media content by resource name and save it to a local file. RESOURCE is the full media resource name.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newAPIClient()
			if err != nil {
				return err
			}
			formatter := getFormatter()
			svc := api.NewMediaService(client)

			resourceName := args[0]
			outputPath, _ := cmd.Flags().GetString("output")

			// Derive the output file name if not specified.
			if outputPath == "" {
				outputPath = deriveOutputFilename(resourceName)
			}

			body, contentType, err := svc.Download(cmd.Context(), resourceName)
			if err != nil {
				return fmt.Errorf("downloading media: %w", err)
			}
			defer body.Close()

			// Create the output file.
			outFile, err := os.Create(outputPath)
			if err != nil {
				return fmt.Errorf("creating output file %s: %w", outputPath, err)
			}
			defer outFile.Close()

			written, err := io.Copy(outFile, body)
			if err != nil {
				return fmt.Errorf("writing to file %s: %w", outputPath, err)
			}

			if formatter.IsJSON() {
				result := map[string]interface{}{
					"outputFile":  outputPath,
					"size":        written,
					"contentType": contentType,
				}
				return formatter.Print(result)
			}

			formatter.PrintSuccess(fmt.Sprintf("Downloaded to %s (%d bytes, %s)", outputPath, written, contentType))

			return nil
		},
	}

	cmd.Flags().StringP("output", "o", "", "Output file path (defaults to derived name from resource)")

	return cmd
}

// deriveOutputFilename attempts to extract a reasonable filename from a
// resource name. If no meaningful name can be derived, it falls back to
// "download".
func deriveOutputFilename(resourceName string) string {
	// Resource names may look like:
	//   spaces/AAAA/messages/BBBB/attachments/CCCC
	// Try to use the last segment as the filename.
	parts := strings.Split(resourceName, "/")
	if len(parts) > 0 {
		last := parts[len(parts)-1]
		// If the last segment has an extension, use it directly.
		if filepath.Ext(last) != "" {
			return last
		}
		// Otherwise use it as a base name.
		if last != "" {
			return last
		}
	}
	return "download"
}
