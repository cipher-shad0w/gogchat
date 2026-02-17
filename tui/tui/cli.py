"""CLI wrapper module for the gchat TUI.

This module provides functions to interact with the gogchat CLI binary,
which is used to fetch data from Google Chat.
"""

import json
import logging
import shutil
import subprocess
from pathlib import Path

from tui.cache import get_cache
from tui.config import get_config

logger = logging.getLogger(__name__)


# ---------------------------------------------------------------------------
# Exception hierarchy
# ---------------------------------------------------------------------------


class GogchatError(Exception):
    """Base exception for gogchat CLI errors."""

    def __init__(self, message: str, stderr: str = ""):
        self.stderr = stderr
        super().__init__(message)


class BinaryNotFoundError(GogchatError):
    """The gogchat binary could not be found."""


class AuthenticationError(GogchatError):
    """Authentication with Google Chat failed (token expired/missing)."""


class ApiError(GogchatError):
    """A gogchat CLI command failed."""


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def _is_auth_error(stderr: str) -> bool:
    """Check if a stderr message indicates an authentication problem."""
    lower = stderr.lower()
    return any(
        kw in lower
        for kw in (
            "token",
            "auth",
            "credential",
            "login",
            "401",
            "403",
            "oauth",
            "unauthenticated",
        )
    )


def _names_file_path() -> Path:
    """Return the path to the user name overrides file."""
    return Path.home() / ".config" / "gogchat" / "names.json"


def load_name_overrides() -> dict[str, str]:
    """Load user display-name overrides from ~/.config/gogchat/names.json.

    Returns:
        Dict mapping "users/123" to a user-chosen display name.
        Returns an empty dict if the file is missing or cannot be parsed.
    """
    path = _names_file_path()
    if not path.exists():
        return {}
    try:
        data = json.loads(path.read_text(encoding="utf-8"))
        if isinstance(data, dict):
            return {
                k: v
                for k, v in data.items()
                if isinstance(k, str) and isinstance(v, str)
            }
    except (json.JSONDecodeError, OSError) as e:
        logger.warning("Could not load name overrides from %s: %s", path, e)
    return {}


def save_name_override(user_id: str, display_name: str) -> None:
    """Persist a single user display-name override to names.json.

    Args:
        user_id: The user resource name (e.g. "users/123456789").
        display_name: The human-readable name chosen by the user.
    """
    path = _names_file_path()
    path.parent.mkdir(parents=True, exist_ok=True)

    existing = load_name_overrides()
    existing[user_id] = display_name

    try:
        path.write_text(
            json.dumps(existing, indent=2, ensure_ascii=False), encoding="utf-8"
        )
    except OSError as e:
        logger.error("Could not save name override to %s: %s", path, e)


def get_gogchat_path() -> str:
    """Find the path to the gogchat binary.

    First checks if gogchat is available in PATH. If not, looks for it
    relative to the tui package (../../gogchat).

    Returns:
        The absolute path to the gogchat binary.

    Raises:
        FileNotFoundError: If the gogchat binary cannot be found.
    """
    # First, check if gogchat is in PATH
    path_result = shutil.which("gogchat")
    if path_result is not None:
        return path_result

    # Look for it relative to this package (../../gogchat)
    package_dir = Path(__file__).parent
    relative_path = package_dir / ".." / ".." / "gogchat"
    resolved_path = relative_path.resolve()

    if resolved_path.is_file():
        return str(resolved_path)

    raise FileNotFoundError(
        f"gogchat binary not found. Ensure it is in PATH or located at {resolved_path}"
    )


def check_binary() -> tuple[bool, str]:
    """Check if the gogchat binary is available.

    Returns:
        Tuple of (is_available, error_message).
    """
    try:
        get_gogchat_path()
        return (True, "")
    except FileNotFoundError as e:
        return (False, str(e))


def check_auth() -> tuple[bool, str]:
    """Check if the user is authenticated by attempting a simple API call.

    Returns:
        Tuple of (is_authenticated, error_message).
    """
    try:
        gogchat_path = get_gogchat_path()
    except FileNotFoundError as e:
        return (False, f"Binary not found: {e}")

    try:
        result = subprocess.run(
            [gogchat_path, "spaces", "list", "--json", "--page-size", "1"],
            capture_output=True,
            text=True,
            timeout=15,
        )
        if result.returncode != 0:
            stderr = result.stderr.lower()
            if any(
                kw in stderr
                for kw in (
                    "token",
                    "auth",
                    "credential",
                    "login",
                    "401",
                    "403",
                    "oauth",
                )
            ):
                return (
                    False,
                    "Authentication expired or missing. Run: gogchat auth login",
                )
            return (False, f"gogchat error: {result.stderr.strip()}")
        return (True, "")
    except subprocess.TimeoutExpired:
        return (False, "gogchat timed out — check network connection")
    except OSError as e:
        return (False, f"Failed to run gogchat: {e}")


def list_spaces() -> list[dict]:
    """List all Google Chat spaces.

    Runs: gogchat spaces list --all --json

    Returns:
        A list of space dictionaries, each containing:
        - name: The resource name of the space
        - displayName: The display name of the space
        - spaceType: The type of space (e.g., "SPACE", "GROUP_CHAT")
        - membershipCount: Number of members in the space
        - createTime: When the space was created

        Returns an empty list on error.
    """
    try:
        gogchat_path = get_gogchat_path()
    except FileNotFoundError as e:
        logger.error("Failed to find gogchat binary: %s", e)
        return []

    try:
        result = subprocess.run(
            [gogchat_path, "spaces", "list", "--all", "--json"],
            capture_output=True,
            text=True,
            check=True,
        )
    except subprocess.CalledProcessError as e:
        if _is_auth_error(e.stderr):
            logger.error("AUTH_ERROR: %s", e.stderr.strip())
        logger.error(
            "Failed to list spaces: %s (exit code %d, stderr: %s)",
            e,
            e.returncode,
            e.stderr,
        )
        return []

    try:
        data = json.loads(result.stdout)
        return data.get("spaces", [])
    except json.JSONDecodeError as e:
        logger.error("Failed to parse spaces JSON output: %s", e)
        return []


def list_messages(space_name: str, limit: int = 25) -> list[dict]:
    """List messages from a Google Chat space.

    Runs: gogchat messages list {space_name} --json --page-size {limit}

    Args:
        space_name: The resource name of the space (e.g., "spaces/AAAA").
        limit: Maximum number of messages to retrieve (default: 25).

    Returns:
        A list of message dictionaries, each containing:
        - name: The resource name of the message
        - text: The message text content
        - createTime: When the message was created
        - sender: A dict with displayName and name of the sender

        Returns an empty list on error.
    """
    try:
        gogchat_path = get_gogchat_path()
    except FileNotFoundError as e:
        logger.error("Failed to find gogchat binary: %s", e)
        return []

    try:
        result = subprocess.run(
            [
                gogchat_path,
                "messages",
                "list",
                space_name,
                "--json",
                "--page-size",
                str(limit),
            ],
            capture_output=True,
            text=True,
            check=True,
        )
    except subprocess.CalledProcessError as e:
        if _is_auth_error(e.stderr):
            logger.error("AUTH_ERROR: %s", e.stderr.strip())
        logger.error(
            "Failed to list messages for space %s: %s (exit code %d, stderr: %s)",
            space_name,
            e,
            e.returncode,
            e.stderr,
        )
        return []

    try:
        data = json.loads(result.stdout)
        return data.get("messages", [])
    except json.JSONDecodeError as e:
        logger.error("Failed to parse messages JSON output: %s", e)
        return []


def send_message(space_name: str, text: str) -> bool:
    """Send a message to a Google Chat space.

    Runs: gogchat messages send {space_name} --text {text}

    Args:
        space_name: The resource name of the space (e.g., "spaces/AAAA").
        text: The message text content to send.

    Returns:
        True if the message was sent successfully, False otherwise.
    """
    try:
        gogchat_path = get_gogchat_path()
    except FileNotFoundError as e:
        logger.error("Failed to find gogchat binary: %s", e)
        return False

    try:
        subprocess.run(
            [gogchat_path, "messages", "send", space_name, "--text", text],
            capture_output=True,
            text=True,
            check=True,
        )
    except subprocess.CalledProcessError as e:
        if _is_auth_error(e.stderr):
            logger.error("AUTH_ERROR: %s", e.stderr.strip())
        logger.error(
            "Failed to send message to space %s: %s (exit code %d, stderr: %s)",
            space_name,
            e,
            e.returncode,
            e.stderr,
        )
        return False

    return True


def delete_message(message_name: str) -> bool:
    """Delete a message from a Google Chat space.

    Runs: gogchat messages delete {message_name}

    Args:
        message_name: The resource name of the message
            (e.g., "spaces/AAAA/messages/BBBB").

    Returns:
        True if the message was deleted successfully, False otherwise.
    """
    try:
        gogchat_path = get_gogchat_path()
    except FileNotFoundError as e:
        logger.error("Failed to find gogchat binary: %s", e)
        return False

    try:
        subprocess.run(
            [gogchat_path, "messages", "delete", message_name],
            capture_output=True,
            text=True,
            check=True,
        )
    except subprocess.CalledProcessError as e:
        if _is_auth_error(e.stderr):
            logger.error("AUTH_ERROR: %s", e.stderr.strip())
        logger.error(
            "Failed to delete message %s: %s (exit code %d, stderr: %s)",
            message_name,
            e,
            e.returncode,
            e.stderr,
        )
        return False

    return True


def update_message(message_name: str, text: str) -> bool:
    """Update (edit) a message in a Google Chat space.

    Runs: gogchat messages update {message_name} --text {text}

    Args:
        message_name: The resource name of the message.
        text: The new message text content.

    Returns:
        True if the message was updated successfully, False otherwise.
    """
    try:
        gogchat_path = get_gogchat_path()
    except FileNotFoundError as e:
        logger.error("Failed to find gogchat binary: %s", e)
        return False

    try:
        subprocess.run(
            [gogchat_path, "messages", "update", message_name, "--text", text],
            capture_output=True,
            text=True,
            check=True,
        )
    except subprocess.CalledProcessError as e:
        if _is_auth_error(e.stderr):
            logger.error("AUTH_ERROR: %s", e.stderr.strip())
        logger.error(
            "Failed to update message %s: %s (exit code %d, stderr: %s)",
            message_name,
            e,
            e.returncode,
            e.stderr,
        )
        return False

    return True


def list_members(space_name: str) -> list[dict]:
    """List members of a Google Chat space.

    Runs: gogchat members list {space_name} --all --json

    Args:
        space_name: The resource name of the space (e.g., "spaces/AAAA").

    Returns:
        A list of member dictionaries, each containing:
        - name: The membership resource name
        - member: A dict with name and displayName of the user

        Returns an empty list on error.
    """
    try:
        gogchat_path = get_gogchat_path()
    except FileNotFoundError as e:
        logger.error("Failed to find gogchat binary: %s", e)
        return []

    try:
        result = subprocess.run(
            [
                gogchat_path,
                "members",
                "list",
                space_name,
                "--all",
                "--json",
            ],
            capture_output=True,
            text=True,
            check=True,
        )
    except subprocess.CalledProcessError as e:
        if _is_auth_error(e.stderr):
            logger.error("AUTH_ERROR: %s", e.stderr.strip())
        logger.error(
            "Failed to list members for space %s: %s (exit code %d, stderr: %s)",
            space_name,
            e,
            e.returncode,
            e.stderr,
        )
        return []

    try:
        data = json.loads(result.stdout)
        return data.get("memberships", [])
    except json.JSONDecodeError as e:
        logger.error("Failed to parse members JSON output: %s", e)
        return []


def build_user_name_map(memberships: list[dict]) -> dict[str, str]:
    """Build a mapping from user resource names to display names.

    Merges names from the API membership data with local overrides
    from ~/.config/gogchat/names.json.  Local overrides take priority.

    Args:
        memberships: List of membership objects from list_members.

    Returns:
        Dict mapping "users/123" to "Display Name".
    """
    name_map = {}
    for membership in memberships:
        member = membership.get("member", {})
        user_name = member.get("name", "")
        display_name = member.get("displayName", "")
        if user_name and display_name:
            name_map[user_name] = display_name

    # Local overrides take priority over API display names
    name_map.update(load_name_overrides())
    return name_map


def get_space_read_state(space_name: str) -> str | None:
    """Get the last-read time for a space.

    Runs: gogchat readstate get-space users/me/{space_name}/spaceReadState --json

    Args:
        space_name: The space resource name (e.g., "spaces/AAAA").

    Returns:
        The lastReadTime as an RFC3339 string, or None on error.
    """
    try:
        gogchat_path = get_gogchat_path()
    except FileNotFoundError as e:
        logger.error("Failed to find gogchat binary: %s", e)
        return None

    # Build the read state resource name: users/me/spaces/AAAA/spaceReadState
    read_state_name = f"users/me/{space_name}/spaceReadState"

    try:
        result = subprocess.run(
            [gogchat_path, "readstate", "get-space", read_state_name, "--json"],
            capture_output=True,
            text=True,
            check=True,
        )
    except subprocess.CalledProcessError as e:
        if _is_auth_error(e.stderr):
            logger.error("AUTH_ERROR: %s", e.stderr.strip())
        logger.error(
            "Failed to get read state for %s: %s (exit code %d, stderr: %s)",
            space_name,
            e,
            e.returncode,
            e.stderr,
        )
        return None

    try:
        data = json.loads(result.stdout)
        return data.get("lastReadTime")
    except json.JSONDecodeError as e:
        logger.error("Failed to parse read state JSON output: %s", e)
        return None


def update_space_read_state(space_name: str, last_read_time: str) -> bool:
    """Mark a space as read up to the given time.

    Runs: gogchat readstate update-space users/me/{space_name}/spaceReadState --last-read-time {last_read_time} --json

    Args:
        space_name: The space resource name (e.g., "spaces/AAAA").
        last_read_time: RFC3339 timestamp to mark as last read.

    Returns:
        True if successful, False otherwise.
    """
    try:
        gogchat_path = get_gogchat_path()
    except FileNotFoundError as e:
        logger.error("Failed to find gogchat binary: %s", e)
        return False

    read_state_name = f"users/me/{space_name}/spaceReadState"

    try:
        subprocess.run(
            [
                gogchat_path,
                "readstate",
                "update-space",
                read_state_name,
                "--last-read-time",
                last_read_time,
                "--json",
            ],
            capture_output=True,
            text=True,
            check=True,
        )
    except subprocess.CalledProcessError as e:
        if _is_auth_error(e.stderr):
            logger.error("AUTH_ERROR: %s", e.stderr.strip())
        logger.error(
            "Failed to update read state for %s: %s (exit code %d, stderr: %s)",
            space_name,
            e,
            e.returncode,
            e.stderr,
        )
        return False

    return True


def list_reactions(message_name: str) -> list[dict]:
    """List reactions on a message.

    Runs: gogchat reactions list {message_name} --json

    Args:
        message_name: The resource name of the message (e.g., "spaces/AAAA/messages/BBBB").

    Returns:
        A list of reaction dictionaries, each containing:
        - name: The reaction resource name
        - emoji: A dict with unicode field
        - user: The reacting user info

        Returns an empty list on error.
    """
    try:
        gogchat_path = get_gogchat_path()
    except FileNotFoundError as e:
        logger.error("Failed to find gogchat binary: %s", e)
        return []

    try:
        result = subprocess.run(
            [gogchat_path, "reactions", "list", message_name, "--json"],
            capture_output=True,
            text=True,
            check=True,
        )
    except subprocess.CalledProcessError as e:
        if _is_auth_error(e.stderr):
            logger.error("AUTH_ERROR: %s", e.stderr.strip())
        logger.error(
            "Failed to list reactions for %s: %s (exit code %d, stderr: %s)",
            message_name,
            e,
            e.returncode,
            e.stderr,
        )
        return []

    try:
        data = json.loads(result.stdout)
        return data.get("reactions", [])
    except json.JSONDecodeError as e:
        logger.error("Failed to parse reactions JSON output: %s", e)
        return []


def create_reaction(message_name: str, emoji: str) -> bool:
    """Add an emoji reaction to a message.

    Runs: gogchat reactions create {message_name} --emoji {emoji} --json

    Args:
        message_name: The resource name of the message.
        emoji: The unicode emoji to react with (e.g., "👍").

    Returns:
        True if the reaction was created successfully, False otherwise.
    """
    try:
        gogchat_path = get_gogchat_path()
    except FileNotFoundError as e:
        logger.error("Failed to find gogchat binary: %s", e)
        return False

    try:
        subprocess.run(
            [
                gogchat_path,
                "reactions",
                "create",
                message_name,
                "--emoji",
                emoji,
                "--json",
            ],
            capture_output=True,
            text=True,
            check=True,
        )
    except subprocess.CalledProcessError as e:
        if _is_auth_error(e.stderr):
            logger.error("AUTH_ERROR: %s", e.stderr.strip())
        logger.error(
            "Failed to create reaction on %s: %s (exit code %d, stderr: %s)",
            message_name,
            e,
            e.returncode,
            e.stderr,
        )
        return False

    return True


def list_spaces_cached() -> list[dict]:
    """Return spaces from cache if available, otherwise fetch from API."""
    cached = get_cache().get_spaces()
    if cached is not None:
        return cached
    spaces = list_spaces()
    get_cache().set_spaces(spaces)
    return spaces


def list_spaces_fresh() -> list[dict]:
    """Always fetch spaces from API and update the cache."""
    spaces = list_spaces()
    get_cache().set_spaces(spaces)
    return spaces


def list_messages_cached(space_name: str, limit: int | None = None) -> list[dict]:
    """Return messages from cache if available, otherwise fetch from API."""
    if limit is None:
        limit = get_config().message_page_size
    cached = get_cache().get_messages(space_name)
    if cached is not None:
        return cached
    messages = list_messages(space_name, limit)
    get_cache().set_messages(space_name, messages)
    return messages


def list_messages_fresh(space_name: str, limit: int | None = None) -> list[dict]:
    """Always fetch messages from API and update the cache."""
    if limit is None:
        limit = get_config().message_page_size
    messages = list_messages(space_name, limit)
    get_cache().set_messages(space_name, messages)
    return messages


def list_members_cached(space_name: str) -> list[dict]:
    """Return members from cache if available (with TTL), otherwise fetch from API."""
    cached = get_cache().get_members(space_name)
    if cached is not None:
        return cached
    members = list_members(space_name)
    get_cache().set_members(space_name, members)
    return members


def list_members_fresh(space_name: str) -> list[dict]:
    """Always fetch members from API and update the cache."""
    members = list_members(space_name)
    get_cache().set_members(space_name, members)
    return members
