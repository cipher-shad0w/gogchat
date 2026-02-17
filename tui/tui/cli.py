"""CLI wrapper module for the gchat TUI.

This module provides functions to interact with the gogchat CLI binary,
which is used to fetch data from Google Chat.
"""

import json
import logging
import shutil
import subprocess
from pathlib import Path

logger = logging.getLogger(__name__)


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
        logger.error(
            "Failed to send message to space %s: %s (exit code %d, stderr: %s)",
            space_name,
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
