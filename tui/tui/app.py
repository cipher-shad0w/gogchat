"""Main application class for the gchat TUI."""

from datetime import datetime
from pathlib import Path

from textual import work
from textual.app import App, ComposeResult
from textual.containers import Horizontal, Vertical
from textual.widgets import ListView

from tui.cache import get_cache
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

    async def on_mount(self) -> None:
        """Run startup checks when the app mounts."""
        self._check_startup()

    @work(thread=True)
    def _check_startup(self) -> None:
        """Check binary availability and authentication on startup."""
        from tui.cli import check_binary, check_auth

        ok, err = check_binary()
        if not ok:
            self.call_from_thread(
                self.notify,
                f"gogchat binary not found!\n{err}",
                severity="error",
                timeout=10,
            )
            return

        ok, err = check_auth()
        if not ok:
            self.call_from_thread(
                self.notify,
                err,
                severity="error",
                timeout=10,
            )

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

        # Mark space as read in the UI (remove the unread dot immediately)
        groups_panel = self.query_one("#groups-panel", GroupsPanel)
        groups_panel.mark_space_as_read(event.space_name)

        # Clear existing messages and show loading.
        # Await the clear so old children are fully removed before we add new ones.
        chat_log = self.query_one("#chat-log", ChatLog)
        await chat_log.clear()

        # Try to show cached messages immediately
        from tui.cli import build_user_name_map

        cached_messages = get_cache().get_messages(event.space_name)
        cached_members = get_cache().get_members(event.space_name)

        if cached_messages is not None:
            # Show cached data instantly
            user_name_map = (
                build_user_name_map(cached_members) if cached_members else {}
            )
            await self._display_messages(cached_messages, user_name_map)
        else:
            chat_log.write_message("[dim]Loading messages...[/dim]")

        # Always refresh in background (will silently update if data changed)
        self.load_messages(event.space_name)

    @work(thread=True)
    def load_messages(self, space_name: str) -> None:
        """Load messages from the selected space in a background thread."""
        from tui.cli import list_messages_fresh, list_members_fresh, build_user_name_map

        messages = list_messages_fresh(space_name)
        memberships = list_members_fresh(space_name)
        user_name_map = build_user_name_map(memberships)

        # Don't update if user has already switched to a different space
        if self.current_space != space_name:
            return

        # Skip re-render if the data hasn't changed
        if self._messages_unchanged(messages):
            return

        # Update UI from the worker thread
        self.call_from_thread(self._display_messages, messages, user_name_map)

    def _messages_unchanged(self, new_messages: list[dict]) -> bool:
        """Return True if new_messages matches the currently displayed messages."""
        old = self._current_messages
        if len(old) != len(new_messages):
            return False
        if not old:
            # Both empty
            return True
        # Compare first and last message resource names and text
        for idx in (0, -1):
            if old[idx].get("name") != new_messages[idx].get("name"):
                return False
            if old[idx].get("text") != new_messages[idx].get("text"):
                return False
        return True

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

        # First pass: resolve all sender names and find the longest one
        resolved_names: list[tuple[str, bool]] = []  # (display_name, is_resolved)
        for msg in messages:
            sender = msg.get("sender", {})
            sender_name = sender.get("displayName")
            user_id = sender.get("name", "")
            resolved = True

            if not sender_name:
                sender_name = user_name_map.get(user_id)
                if not sender_name:
                    sender_name = user_id or "Unknown"
                    resolved = False

            resolved_names.append((sender_name, resolved))

        # Calculate the max width of "name:" to align the text column
        max_name_len = max(
            (len(name) + 1 for name, _ in resolved_names), default=0
        )  # +1 for the colon

        # The timestamp prefix is always "[HH:MM] " = 8 visible chars when present
        time_prefix_len = 8  # len("[HH:MM] ") — visible chars of the dim timestamp

        # Second pass: render messages with aligned columns
        for msg, (sender_name, resolved) in zip(messages, resolved_names):
            sender = msg.get("sender", {})
            user_id = sender.get("name", "")
            text = msg.get("text", "")

            # Parse the send time from createTime (e.g. "2025-01-15T11:23:45.123Z")
            time_str = ""
            has_time = False
            create_time = msg.get("createTime", "")
            if create_time:
                try:
                    dt = datetime.fromisoformat(create_time.replace("Z", "+00:00"))
                    # Convert to local time
                    local_dt = dt.astimezone()
                    time_str = f"[dim][{local_dt.strftime('%H:%M')}][/dim] "
                    has_time = True
                except (ValueError, OSError):
                    pass

            # Pad "name:" to max_name_len so text column aligns
            name_with_colon = f"{sender_name}:"
            padding = " " * (max_name_len - len(name_with_colon) + 1)

            # Build prefix markup (timestamp + name + padding)
            prefix_markup = f"{time_str}[bold]{name_with_colon}[/bold]{padding}"

            # Calculate visual prefix width
            prefix_width = (time_prefix_len if has_time else 0) + max_name_len + 1

            # Full content string for raw storage
            content = f"{prefix_markup}{text}"

            item = MessageItem(
                content,
                sender_user_id=user_id if user_id else None,
                is_name_resolved=resolved,
                prefix_markup=prefix_markup,
                prefix_width=prefix_width,
                body_text=text,
            )
            chat_log._raw_entries.append(content)
            chat_log.append(item)

        # Always select the newest (last) message and scroll to it
        chat_log.index = len(chat_log) - 1
        chat_log.scroll_visible()

        # Mark the space as read in the background (use the latest message time)
        if messages and self.current_space:
            latest_time = messages[-1].get("createTime", "")
            if latest_time:
                self._mark_space_read(self.current_space, latest_time)

    @work(thread=True)
    def _mark_space_read(self, space_name: str, last_read_time: str) -> None:
        """Mark a space as read up to the given time in the background."""
        from tui.cli import update_space_read_state

        if not update_space_read_state(space_name, last_read_time):
            self.call_from_thread(
                self.notify,
                "Failed to update read state",
                severity="warning",
                timeout=3,
            )

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
            # Invalidate cached messages so reload fetches fresh data
            get_cache().invalidate_messages(space_name)
            # Reload messages to show the sent message
            self.call_from_thread(self.load_messages, space_name)
        else:
            self.call_from_thread(
                self.notify,
                "Failed to send message",
                severity="error",
                timeout=5,
            )

    def action_refresh_spaces(self) -> None:
        """Refresh the spaces list."""
        groups_panel = self.query_one("#groups-panel", GroupsPanel)
        groups_panel.load_spaces()
