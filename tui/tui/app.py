"""Main application class for the gchat TUI."""

from pathlib import Path

from textual import work
from textual.app import App, ComposeResult
from textual.containers import Horizontal, Vertical

from tui.widgets import ChatLog, ChatPanel, GroupsPanel, InputPanel, MessageInput


class ChatApp(App):
    """A Textual app for gchat with a Spotify-style minimal TUI."""

    CSS_PATH = Path(__file__).parent / "styles.css"

    BINDINGS = [
        ("q", "quit", "Quit"),
        ("r", "refresh_spaces", "Refresh"),
    ]

    current_space: str | None = None

    def compose(self) -> ComposeResult:
        """Create the application layout."""
        with Horizontal(id="main-container"):
            yield GroupsPanel(id="groups-panel")
            with Vertical(id="main-content"):
                yield ChatPanel(id="chat-panel")
                yield InputPanel(id="input-panel")

    def on_groups_panel_space_selected(self, event: GroupsPanel.SpaceSelected) -> None:
        """Handle space selection from the groups panel."""
        self.current_space = event.space_name

        # Update chat panel title
        chat_panel = self.query_one("#chat-panel", ChatPanel)
        chat_panel.border_title = event.display_name or "Chat"

        # Clear existing messages and show loading
        chat_log = self.query_one("#chat-log", ChatLog)
        chat_log.clear()
        chat_log.write_message("[dim]Loading messages...[/dim]")

        # Load messages in background
        self.load_messages(event.space_name)

    @work(thread=True)
    def load_messages(self, space_name: str) -> None:
        """Load messages from the selected space in a background thread."""
        from tui.cli import list_messages, list_members, build_user_name_map

        messages = list_messages(space_name)
        memberships = list_members(space_name)
        user_name_map = build_user_name_map(memberships)

        # Update UI from the worker thread
        self.call_from_thread(self._display_messages, messages, user_name_map)

    def _display_messages(
        self, messages: list[dict], user_name_map: dict[str, str] | None = None
    ) -> None:
        """Display messages in the chat log."""
        chat_log = self.query_one("#chat-log", ChatLog)
        chat_log.clear()

        if user_name_map is None:
            user_name_map = {}

        if not messages:
            chat_log.write_message("[dim]No messages in this space[/dim]")
            return

        # Display messages in chronological order (oldest first)
        # The API returns newest first, so we reverse
        for msg in messages:
            sender = msg.get("sender", {})
            sender_name = sender.get("displayName")
            if not sender_name:
                # Try to resolve from user_name_map
                user_id = sender.get("name", "")
                sender_name = user_name_map.get(user_id, user_id) or "Unknown"

            text = msg.get("text", "")

            # Format: [bold]Sender:[/bold] Message text
            chat_log.write_message(f"[bold]{sender_name}:[/bold] {text}")

        # Always select the newest (last) message and scroll to it
        chat_log.index = len(chat_log) - 1
        chat_log.scroll_visible()

    def on_message_input_submitted(self, event: MessageInput.Submitted) -> None:
        """Handle message submission from the input panel."""
        if not self.current_space:
            return
        self._send_message(self.current_space, event.value)

    @work(thread=True)
    def _send_message(self, space_name: str, text: str) -> None:
        """Send a message in a background thread and reload the chat."""
        from tui.cli import send_message

        if send_message(space_name, text):
            # Reload messages to show the sent message
            self.call_from_thread(self.load_messages, space_name)

    def action_refresh_spaces(self) -> None:
        """Refresh the spaces list."""
        groups_panel = self.query_one("#groups-panel", GroupsPanel)
        groups_panel.load_spaces()
