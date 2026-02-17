"""Main application class for the gchat TUI."""

from pathlib import Path

from textual import work
from textual.app import App, ComposeResult
from textual.containers import Horizontal, Vertical
from textual.widgets import ListView

from tui.widgets import (
    ChatLog,
    ChatPanel,
    GroupsPanel,
    InputPanel,
    MessageInput,
    MessageItem,
    NameInputScreen,
)


class ChatApp(App):
    """A Textual app for gchat with a Spotify-style minimal TUI."""

    CSS_PATH = Path(__file__).parent / "styles.css"

    BINDINGS = [
        ("q", "quit", "Quit"),
        ("r", "refresh_spaces", "Refresh"),
    ]

    current_space: str | None = None
    _current_messages: list[dict]
    _current_user_name_map: dict[str, str]

    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self._current_messages = []
        self._current_user_name_map = {}

    def compose(self) -> ComposeResult:
        """Create the application layout."""
        with Horizontal(id="main-container"):
            yield GroupsPanel(id="groups-panel")
            with Vertical(id="main-content"):
                yield ChatPanel(id="chat-panel")
                yield InputPanel(id="input-panel")

    async def on_groups_panel_space_selected(
        self, event: GroupsPanel.SpaceSelected
    ) -> None:
        """Handle space selection from the groups panel."""
        self.current_space = event.space_name

        # Update chat panel title
        chat_panel = self.query_one("#chat-panel", ChatPanel)
        chat_panel.border_title = event.display_name or "Chat"

        # Clear existing messages and show loading.
        # Await the clear so old children are fully removed before we add new ones.
        chat_log = self.query_one("#chat-log", ChatLog)
        await chat_log.clear()
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

    async def _display_messages(
        self, messages: list[dict], user_name_map: dict[str, str] | None = None
    ) -> None:
        """Display messages in the chat log."""
        chat_log = self.query_one("#chat-log", ChatLog)
        await chat_log.clear()

        if user_name_map is None:
            user_name_map = {}

        # Store for later refresh after name assignment
        self._current_messages = messages
        self._current_user_name_map = user_name_map

        if not messages:
            chat_log.write_message("[dim]No messages in this space[/dim]")
            return

        # Display messages in chronological order (oldest first)
        # The API returns newest first, so we reverse
        for msg in messages:
            sender = msg.get("sender", {})
            sender_name = sender.get("displayName")
            user_id = sender.get("name", "")
            resolved = True

            if not sender_name:
                # Try to resolve from user_name_map
                sender_name = user_name_map.get(user_id)
                if not sender_name:
                    sender_name = user_id or "Unknown"
                    resolved = False

            text = msg.get("text", "")

            # Create MessageItem directly with metadata
            content = f"[bold]{sender_name}:[/bold] {text}"
            item = MessageItem(
                content,
                sender_user_id=user_id if user_id else None,
                is_name_resolved=resolved,
            )
            chat_log._raw_entries.append(content)
            chat_log.append(item)

        # Always select the newest (last) message and scroll to it
        chat_log.index = len(chat_log) - 1
        chat_log.scroll_visible()

    def on_list_view_selected(self, event: ListView.Selected) -> None:
        """Handle Enter on a chat message to assign a name to unknown senders."""
        if not isinstance(event.item, MessageItem):
            return
        item: MessageItem = event.item
        # Only prompt for unresolved names
        if item.is_name_resolved or not item.sender_user_id:
            return

        def _handle_name_result(name: str | None) -> None:
            if name is None:
                return
            from tui.cli import save_name_override

            save_name_override(item.sender_user_id, name)
            # Update the in-memory map and refresh display
            self._current_user_name_map[item.sender_user_id] = name
            # Schedule async _display_messages on the event loop
            # (this callback is invoked synchronously by Screen.dismiss)
            self.run_worker(
                self._display_messages(
                    self._current_messages, self._current_user_name_map
                ),
                exclusive=False,
            )

        self.push_screen(
            NameInputScreen(item.sender_user_id), callback=_handle_name_result
        )

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
