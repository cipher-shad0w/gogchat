"""Custom widgets for the gchat TUI."""

from textual import events, work
from textual.app import ComposeResult
from textual.await_remove import AwaitRemove
from textual.containers import Vertical
from textual.message import Message
from textual.screen import ModalScreen
from textual.widgets import Input, Label, ListItem, ListView, Static, TextArea


class SpaceItem(ListItem):
    """A list item representing a chat space."""

    def __init__(self, space_name: str, display_name: str) -> None:
        """Initialize with space metadata.

        Args:
            space_name: The space resource name (e.g., "spaces/AAAA")
            display_name: Human-readable name to show in the list
        """
        # Create label with display_name, fallback to space_name
        label_text = display_name if display_name else space_name.split("/")[-1]
        super().__init__(Label(label_text))
        self.space_name = space_name
        self.display_name = display_name


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
        self.load_spaces()

    @work(thread=True)
    def load_spaces(self) -> None:
        """Load spaces from the CLI in a background thread."""
        from tui.cli import list_spaces  # Import here to avoid circular imports

        spaces = list_spaces()

        # Update UI from the worker thread
        self.app.call_from_thread(self._populate_spaces, spaces)

    def _populate_spaces(self, spaces: list[dict]) -> None:
        """Populate the ListView with space items."""
        groups_list = self.query_one("#groups-list", ListView)
        groups_list.clear()

        if not spaces:
            # Show a message if no spaces found
            groups_list.append(ListItem(Label("No spaces found")))
            return

        for space in spaces:
            space_name = space.get("name", "")
            display_name = space.get("displayName", "")
            groups_list.append(SpaceItem(space_name, display_name))

    def on_list_view_selected(self, event: ListView.Selected) -> None:
        """Handle space selection."""
        if isinstance(event.item, SpaceItem):
            self.post_message(
                self.SpaceSelected(event.item.space_name, event.item.display_name)
            )


class MessageItem(ListItem):
    """A list item representing a single chat message."""

    def __init__(
        self,
        content: str,
        sender_user_id: str | None = None,
        is_name_resolved: bool = True,
    ) -> None:
        """Initialize with message content (Rich markup string).

        Args:
            content: Rich-markup formatted message text.
            sender_user_id: The sender's resource name (e.g. "users/123").
            is_name_resolved: False when the displayed name is a raw user ID.
        """
        super().__init__(Label(content, markup=True))
        self.message_content = content
        self.sender_user_id = sender_user_id
        self.is_name_resolved = is_name_resolved


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
