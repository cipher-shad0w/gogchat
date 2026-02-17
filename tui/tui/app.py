"""Main application class for the gchat TUI."""

import sys
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
    ConfirmDeleteScreen,
    EditMessageScreen,
    GroupsPanel,
    InputPanel,
    MessageActionScreen,
    MessageInput,
    MessageItem,
    NameInputScreen,
    ReactionScreen,
)


def _format_reactions(reactions: list[dict]) -> str:
    """Format reaction data into a display string.

    Groups reactions by emoji and shows counts.

    Args:
        reactions: List of reaction dicts from the API.

    Returns:
        Rich-markup string like "👍 3  ❤️ 1" or empty string.
    """
    if not reactions:
        return ""

    # Count reactions by emoji
    counts: dict[str, int] = {}
    for reaction in reactions:
        emoji_data = reaction.get("emoji", {})
        unicode_emoji = emoji_data.get("unicode", "")
        if unicode_emoji:
            counts[unicode_emoji] = counts.get(unicode_emoji, 0) + 1

    if not counts:
        return ""

    parts = [f"{emoji} {count}" for emoji, count in counts.items()]
    return "[dim]" + "  ".join(parts) + "[/dim]"


def _resource_path(relative: str) -> Path:
    """Resolve a resource path, compatible with PyInstaller bundles.

    When running inside a PyInstaller bundle, ``sys._MEIPASS`` points to the
    temporary directory where bundled files are extracted.  Outside of a
    bundle the path is resolved relative to *this* source file.
    """
    base = getattr(sys, "_MEIPASS", None)
    if base is not None:
        return Path(base) / relative
    return Path(__file__).parent / relative


class ChatApp(App):
    """A Textual app for gchat with a Spotify-style minimal TUI."""

    CSS_PATH = _resource_path("styles.css")

    BINDINGS = [
        ("q", "quit", "Quit"),
        ("r", "refresh_spaces", "Refresh"),
        ("e", "add_reaction", "React"),
        ("a", "message_action", "Actions"),
        ("h", "focus_spaces", "Spaces"),
        ("l", "focus_chat", "Chat"),
        ("slash", "focus_search", "Search"),
        ("f", "toggle_message_search", "Find"),
        ("t", "cycle_space_category", "Category"),
    ]

    current_space: str | None = None
    _current_messages: list[dict]
    _current_user_name_map: dict[str, str]

    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self._current_messages = []
        self._current_user_name_map = {}
        self._poll_timer = None
        self._current_user_id: str | None = None

    async def on_mount(self) -> None:
        """Run startup checks when the app mounts."""
        self._check_startup()
        self._start_polling()

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
        else:
            # Detect current user ID for ownership checks
            from tui.cli import get_self_user_id

            user_id = get_self_user_id()
            if user_id:
                self._current_user_id = user_id

    def compose(self) -> ComposeResult:
        """Create the application layout."""
        with Horizontal(id="main-container"):
            yield GroupsPanel(id="groups-panel")
            with Vertical(id="main-content"):
                yield ChatPanel(id="chat-panel")
                yield InputPanel(id="input-panel")

    def _start_polling(self) -> None:
        """Start the message polling timer if enabled in config."""
        from tui.config import get_config

        interval = get_config().poll_interval
        if interval <= 0:
            return

        # Cancel any existing timer
        self._stop_polling()
        self._poll_timer = self.set_interval(interval, self._poll_messages)

    def _stop_polling(self) -> None:
        """Stop the message polling timer."""
        if self._poll_timer is not None:
            self._poll_timer.stop()
            self._poll_timer = None

    def _restart_polling(self) -> None:
        """Restart the polling timer (called when switching spaces)."""
        self._start_polling()

    def _poll_messages(self) -> None:
        """Timer callback: refresh messages for the current space."""
        if self.current_space:
            self.load_messages(self.current_space)

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

        # Start/restart polling for the new space
        self._restart_polling()

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

        # Check for new messages (for notifications)
        self._check_new_message_notifications(messages, user_name_map)

        # Update UI from the worker thread
        self.call_from_thread(self._display_messages, messages, user_name_map)

    def _messages_unchanged(self, new_messages: list[dict]) -> bool:
        """Return True if new_messages matches the currently displayed messages."""
        old = self._current_messages
        if len(old) != len(new_messages):
            return False
        if not old:
            return True
        # Compare all messages by resource name and text content
        for old_msg, new_msg in zip(old, new_messages):
            if old_msg.get("name") != new_msg.get("name"):
                return False
            if old_msg.get("text") != new_msg.get("text"):
                return False
        return True

    def _check_new_message_notifications(
        self, messages: list[dict], user_name_map: dict[str, str]
    ) -> None:
        """Send notifications for new messages detected during polling.

        Only notifies if there are more messages than before (i.e., new
        messages arrived), not on initial load or space switch.
        """
        from tui.config import get_config
        from tui.notify import send_notification

        config = get_config()
        if config.notifications == "off":
            return

        old = self._current_messages
        if not old:
            # First load — don't notify
            return

        # Only notify if new messages appeared (more messages than before)
        if len(messages) <= len(old):
            return

        # Find the newest message that wasn't in the old set
        old_names = {m.get("name") for m in old}
        new_msgs = [m for m in messages if m.get("name") not in old_names]

        if not new_msgs:
            return

        # Get the latest new message for the notification
        latest = new_msgs[-1]
        sender = latest.get("sender", {})
        sender_name = sender.get("displayName") or user_name_map.get(
            sender.get("name", ""), "Someone"
        )
        text = latest.get("text", "")
        # Truncate long messages
        preview = text[:80] + "\u2026" if len(text) > 80 else text

        # Use the sender name as the notification title
        space_title = sender_name

        send_notification(
            title=space_title,
            body=preview,
            mode=config.notifications,
        )

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

            # Format rich text (bold, italic, code, links)
            from tui.richtext import format_message_text, format_attachments

            formatted_text = format_message_text(text)

            # Add attachment indicators
            attachment_str = format_attachments(msg)
            if attachment_str:
                formatted_text = (
                    f"{formatted_text}\n{attachment_str}"
                    if formatted_text
                    else attachment_str
                )

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
            content = f"{prefix_markup}{formatted_text}"

            # Format reactions if present
            reactions = msg.get("reactions", [])
            reactions_markup = _format_reactions(reactions)

            item = MessageItem(
                content,
                sender_user_id=user_id if user_id else None,
                is_name_resolved=resolved,
                prefix_markup=prefix_markup,
                prefix_width=prefix_width,
                body_text=formatted_text,
                message_name=msg.get("name", ""),
                reactions_markup=reactions_markup,
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

    def action_add_reaction(self) -> None:
        """Open the reaction dialog for the currently highlighted message."""
        chat_log = self.query_one("#chat-log", ChatLog)
        if chat_log.index is None:
            return

        highlighted = chat_log.highlighted_child
        if not isinstance(highlighted, MessageItem):
            return

        if not highlighted.message_name:
            return

        msg_name = highlighted.message_name

        def _handle_reaction(emoji: str | None) -> None:
            if emoji is None:
                return
            self._create_reaction(msg_name, emoji)

        self.push_screen(ReactionScreen(), callback=_handle_reaction)

    @work(thread=True)
    def _create_reaction(self, message_name: str, emoji: str) -> None:
        """Create a reaction in a background thread."""
        from tui.cli import create_reaction

        if create_reaction(message_name, emoji):
            # Reload messages to show the new reaction
            if self.current_space:
                self.call_from_thread(self.load_messages, self.current_space)
        else:
            self.call_from_thread(
                self.notify,
                "Failed to add reaction",
                severity="error",
                timeout=3,
            )

    def action_message_action(self) -> None:
        """Open the message action menu for the highlighted message."""
        chat_log = self.query_one("#chat-log", ChatLog)
        if chat_log.index is None:
            return

        highlighted = chat_log.highlighted_child
        if not isinstance(highlighted, MessageItem):
            return

        if not highlighted.message_name:
            return

        msg_name = highlighted.message_name
        body_text = highlighted.body_text
        is_own = (
            self._current_user_id is not None
            and highlighted.sender_user_id == self._current_user_id
        )

        def _handle_action(action: str | None) -> None:
            if action is None:
                return
            if action == "edit":
                self._open_edit_dialog(msg_name, body_text)
            elif action == "delete":
                self._open_delete_confirm(msg_name)
            elif action == "quote":
                self._quote_reply(body_text)

        self.push_screen(
            MessageActionScreen(msg_name, body_text, is_own_message=is_own),
            callback=_handle_action,
        )

    def _open_edit_dialog(self, message_name: str, current_text: str) -> None:
        """Open the edit message dialog."""

        def _handle_edit(new_text: str | None) -> None:
            if new_text is None:
                return
            self._update_message(message_name, new_text)

        self.push_screen(EditMessageScreen(current_text), callback=_handle_edit)

    def _open_delete_confirm(self, message_name: str) -> None:
        """Open the delete confirmation dialog."""

        def _handle_delete(confirmed: bool) -> None:
            if confirmed:
                self._delete_message(message_name)

        self.push_screen(ConfirmDeleteScreen(), callback=_handle_delete)

    def _quote_reply(self, body_text: str) -> None:
        """Pre-fill the message input with a quote of the selected message."""
        message_input = self.query_one("#message-input", MessageInput)
        # Format as blockquote
        quoted_lines = [f"> {line}" for line in body_text.split("\n")]
        quoted = "\n".join(quoted_lines) + "\n"
        message_input.load_text(quoted)
        message_input.focus()

    @work(thread=True)
    def _delete_message(self, message_name: str) -> None:
        """Delete a message in a background thread."""
        from tui.cli import delete_message

        if delete_message(message_name):
            if self.current_space:
                get_cache().invalidate_messages(self.current_space)
                self.call_from_thread(self.load_messages, self.current_space)
            self.call_from_thread(
                self.notify, "Message deleted", severity="information", timeout=3
            )
        else:
            self.call_from_thread(
                self.notify, "Failed to delete message", severity="error", timeout=5
            )

    @work(thread=True)
    def _update_message(self, message_name: str, new_text: str) -> None:
        """Update a message in a background thread."""
        from tui.cli import update_message

        if update_message(message_name, new_text):
            if self.current_space:
                get_cache().invalidate_messages(self.current_space)
                self.call_from_thread(self.load_messages, self.current_space)
            self.call_from_thread(
                self.notify, "Message updated", severity="information", timeout=3
            )
        else:
            self.call_from_thread(
                self.notify, "Failed to update message", severity="error", timeout=5
            )

    def action_focus_spaces(self) -> None:
        """Focus the spaces list panel (h)."""
        try:
            groups_list = self.query_one("#groups-list", ListView)
            groups_list.focus()
        except Exception:
            pass

    def action_focus_chat(self) -> None:
        """Focus the chat log panel (l)."""
        try:
            chat_log = self.query_one("#chat-log", ChatLog)
            chat_log.focus()
        except Exception:
            pass

    def action_focus_search(self) -> None:
        """Focus the message input (/)."""
        try:
            message_input = self.query_one("#message-input", MessageInput)
            message_input.focus()
        except Exception:
            pass

    def action_toggle_message_search(self) -> None:
        """Toggle the message search bar."""
        chat_panel = self.query_one("#chat-panel", ChatPanel)
        chat_panel.toggle_search()

    def action_refresh_spaces(self) -> None:
        """Refresh the spaces list."""
        groups_panel = self.query_one("#groups-panel", GroupsPanel)
        groups_panel.load_spaces()

    def action_cycle_space_category(self) -> None:
        """Cycle through space category filters: all -> spaces -> dms -> all."""
        groups_panel = self.query_one("#groups-panel", GroupsPanel)
        current = groups_panel._category_filter
        cycle = {"all": "spaces", "spaces": "dms", "dms": "all"}
        new_category = cycle.get(current, "all")
        groups_panel._category_filter = new_category

        # Update the panel title to show current filter
        labels = {"all": "Groups", "spaces": "Groups [Spaces]", "dms": "Groups [DMs]"}
        groups_panel.border_title = labels.get(new_category, "Groups")

        # Re-populate with the new filter
        groups_panel._apply_space_filter()
