"""Custom widgets for the gchat TUI."""

import logging

from textual import events, work
from textual.app import ComposeResult
from textual.await_remove import AwaitRemove
from textual.containers import Horizontal, Vertical
from textual.message import Message
from textual.screen import ModalScreen
from textual.widgets import Input, Label, ListItem, ListView, Static, TextArea

logger = logging.getLogger(__name__)


class SpaceItem(ListItem):
    """A list item representing a chat space."""

    def __init__(
        self, space_name: str, display_name: str, has_unread: bool = False
    ) -> None:
        """Initialize with space metadata.

        Args:
            space_name: The space resource name (e.g., "spaces/AAAA")
            display_name: Human-readable name to show in the list
            has_unread: Whether this space has unread messages
        """
        label_text = display_name if display_name else space_name.split("/")[-1]
        self.space_name = space_name
        self.display_name = display_name
        self.has_unread = has_unread
        super().__init__()
        self._label_text = label_text

    def compose(self) -> ComposeResult:
        """Create the space item layout with optional unread dot."""
        with Horizontal(classes="space-row"):
            yield Label("● " if self.has_unread else "  ", classes="unread-dot")
            yield Label(self._label_text, classes="space-name")


class GroupsPanel(Static):
    """Left panel displaying the list of chat groups."""

    class SpaceSelected(Message):
        """Posted when a space is selected."""

        def __init__(self, space_name: str, display_name: str) -> None:
            self.space_name = space_name
            self.display_name = display_name
            super().__init__()

    def compose(self) -> ComposeResult:
        """Create the groups list view."""
        yield ListView(id="groups-list")

    def on_mount(self) -> None:
        """Set the border title and load spaces."""
        self.border_title = "Groups"
        self._current_spaces: list[dict] = []
        self.load_spaces()

    def load_spaces(self) -> None:
        """Load spaces — show cached data instantly, then refresh from API."""
        # Phase 0: Show cached data immediately (runs on the main thread)
        from tui.cache import get_cache

        cached_spaces = get_cache().get_spaces()
        if cached_spaces:
            # Show cached spaces immediately
            cached_unread = get_cache().get_unread_states()
            unread_set: set[str] = set()
            if cached_unread:
                # Spaces with entries are the ones that had unread messages last time
                unread_set = set(cached_unread.keys())
            self._populate_spaces(cached_spaces, unread_set)

        self._refresh_spaces()

    def _spaces_unchanged(self, new_spaces: list[dict]) -> bool:
        """Return True if new_spaces matches the currently displayed spaces."""
        old = self._current_spaces
        if len(old) != len(new_spaces):
            return False
        # Compare space resource names in order
        return all(o.get("name") == n.get("name") for o, n in zip(old, new_spaces))

    @work(thread=True)
    def _refresh_spaces(self) -> None:
        """Fetch fresh spaces from API and update the UI only if changed."""
        from tui.cli import list_spaces_fresh

        spaces = list_spaces_fresh()

        if not spaces and not self._current_spaces:
            # No spaces returned and nothing cached — might be an error
            self.app.call_from_thread(
                self.app.notify,
                "No spaces found — check connection or auth",
                severity="warning",
                timeout=5,
            )

        # Only re-render if the spaces list has actually changed
        if not self._spaces_unchanged(spaces):
            self.app.call_from_thread(self._populate_spaces, spaces)

        # Always kick off the unread check (unread states may have changed)
        self.app.call_from_thread(self._start_unread_check, spaces)

    def _start_unread_check(self, spaces: list[dict]) -> None:
        """Start the background unread state check."""
        self._load_unread_states(spaces)

    @work(thread=True)
    def _load_unread_states(self, spaces: list[dict]) -> None:
        """Check unread status for all spaces in parallel, then update UI."""
        from concurrent.futures import ThreadPoolExecutor, as_completed
        from tui.cli import get_space_read_state, list_messages

        unread_spaces: set[str] = set()

        def check_space(space_name: str) -> tuple[str, bool]:
            last_read_time = get_space_read_state(space_name)
            if last_read_time is None:
                return (space_name, False)
            messages = list_messages(space_name, limit=1)
            if messages:
                latest_msg_time = messages[-1].get("createTime", "")
                if latest_msg_time and latest_msg_time > last_read_time:
                    return (space_name, True)
            return (space_name, False)

        space_names = [s.get("name", "") for s in spaces if s.get("name")]

        from tui.config import get_config

        with ThreadPoolExecutor(
            max_workers=get_config().unread_check_workers
        ) as executor:
            futures = {executor.submit(check_space, name): name for name in space_names}
            for future in as_completed(futures):
                try:
                    name, is_unread = future.result()
                    if is_unread:
                        unread_spaces.add(name)
                except Exception as exc:
                    logger.warning(
                        "Failed to check unread state for %s: %s",
                        futures[future],
                        exc,
                    )

        # Cache the unread states for instant display next time
        from tui.cache import get_cache

        get_cache().set_unread_states({name: "unread" for name in unread_spaces})

        if unread_spaces:
            self.app.call_from_thread(self._update_unread_indicators, unread_spaces)

    def _update_unread_indicators(self, unread_spaces: set[str]) -> None:
        """Update unread dot indicators for spaces that have unread messages."""
        groups_list = self.query_one("#groups-list", ListView)
        for item in groups_list.children:
            if isinstance(item, SpaceItem) and item.space_name in unread_spaces:
                item.has_unread = True
                try:
                    dot_label = item.query_one(".unread-dot", Label)
                    dot_label.update("● ")
                except Exception:
                    pass

    def _populate_spaces(
        self, spaces: list[dict], unread_spaces: set[str] | None = None
    ) -> None:
        """Populate the ListView with space items."""
        groups_list = self.query_one("#groups-list", ListView)
        groups_list.clear()

        if unread_spaces is None:
            unread_spaces = set()

        if not spaces:
            groups_list.append(ListItem(Label("No spaces found")))
            return

        for space in spaces:
            space_name = space.get("name", "")
            display_name = space.get("displayName", "")
            has_unread = space_name in unread_spaces
            groups_list.append(
                SpaceItem(space_name, display_name, has_unread=has_unread)
            )

        self._current_spaces = spaces

    def on_list_view_selected(self, event: ListView.Selected) -> None:
        """Handle space selection."""
        if isinstance(event.item, SpaceItem):
            self.post_message(
                self.SpaceSelected(event.item.space_name, event.item.display_name)
            )

    def mark_space_as_read(self, space_name: str) -> None:
        """Remove the unread indicator from a specific space item."""
        groups_list = self.query_one("#groups-list", ListView)
        for item in groups_list.children:
            if isinstance(item, SpaceItem) and item.space_name == space_name:
                item.has_unread = False
                try:
                    dot_label = item.query_one(".unread-dot", Label)
                    dot_label.update("  ")
                except Exception:
                    pass
                break


class MessageItem(ListItem):
    """A list item representing a single chat message."""

    def __init__(
        self,
        content: str,
        sender_user_id: str | None = None,
        is_name_resolved: bool = True,
        prefix_markup: str = "",
        prefix_width: int = 0,
        body_text: str = "",
    ) -> None:
        """Initialize with message content.

        Args:
            content: Full Rich-markup formatted message text (kept for raw storage).
            sender_user_id: The sender's resource name (e.g. "users/123").
            is_name_resolved: False when the displayed name is a raw user ID.
            prefix_markup: Rich markup for the prefix (timestamp + name + padding).
            prefix_width: The visual character width of the prefix.
            body_text: The plain message body text (will wrap independently).
        """
        # If prefix_markup and body_text are provided, use two-column layout
        if prefix_markup and prefix_width > 0:
            prefix_label = Static(prefix_markup, markup=True, classes="msg-prefix")
            prefix_label.styles.width = prefix_width
            prefix_label.styles.min_width = prefix_width
            prefix_label.styles.max_width = prefix_width
            body_label = Label(body_text, markup=False, classes="msg-body")
            container = Horizontal(prefix_label, body_label, classes="msg-row")
            super().__init__(container)
        else:
            # Fallback for system/status messages
            super().__init__(Label(content, markup=True))
        self.message_content = content
        self.sender_user_id = sender_user_id
        self.is_name_resolved = is_name_resolved
        self.prefix_width = prefix_width


class ChatLog(ListView):
    """A ListView that displays chat messages as selectable items."""

    def __init__(self, **kwargs) -> None:
        # Remove any kwargs that are specific to RichLog and not applicable to ListView
        kwargs.pop("highlight", None)
        kwargs.pop("markup", None)
        kwargs.pop("wrap", None)
        kwargs.pop("min_width", None)
        super().__init__(**kwargs)
        self._raw_entries: list[str] = []

    def write_message(self, content: str) -> None:
        """Write a message as a new selectable list item."""
        self._raw_entries.append(content)
        self.append(MessageItem(content))
        self.index = len(self._raw_entries) - 1

    def clear(self) -> AwaitRemove:
        """Clear all messages."""
        self._raw_entries.clear()
        return super().clear()


class ChatPanel(Static):
    """Main panel displaying chat messages."""

    def compose(self) -> ComposeResult:
        """Create the chat log view."""
        yield ChatLog(id="chat-log")

    def on_mount(self) -> None:
        """Set the border title when mounted."""
        self.border_title = "Chat"


class MessageInput(TextArea):
    """A TextArea that sends on Enter and inserts newline on Shift+Enter."""

    class Submitted(Message):
        """Posted when the user presses Enter to send a message."""

        def __init__(self, value: str) -> None:
            self.value = value
            super().__init__()

    def _on_key(self, event: events.Key) -> None:
        if event.key == "enter":
            event.prevent_default()
            event.stop()
            text = self.text.strip()
            if text:
                self.post_message(self.Submitted(text))
                self.clear()
            return
        super()._on_key(event)


class InputPanel(Static):
    """Bottom panel for composing messages."""

    def compose(self) -> ComposeResult:
        """Create the message input area."""
        yield MessageInput(id="message-input")

    def on_mount(self) -> None:
        """Set the border title when mounted."""
        self.border_title = "Message"


class NameInputScreen(ModalScreen[str | None]):
    """Modal dialog to assign a display name to an unknown user."""

    BINDINGS = [("escape", "cancel", "Cancel")]

    def __init__(self, user_id: str) -> None:
        self.user_id = user_id
        super().__init__()

    def compose(self) -> ComposeResult:
        with Vertical(id="name-dialog"):
            yield Label(
                f"Enter name for [bold]{self.user_id}[/bold]:",
                id="name-dialog-title",
            )
            yield Input(placeholder="Display name…", id="name-input")

    def on_mount(self) -> None:
        self.query_one("#name-input", Input).focus()

    def on_input_submitted(self, event: Input.Submitted) -> None:
        value = event.value.strip()
        if value:
            self.dismiss(value)
        else:
            self.dismiss(None)

    def action_cancel(self) -> None:
        self.dismiss(None)
