"""Entry point for running the TUI as a module: python -m tui"""

import sys

__version__ = "0.1.0"


def main() -> None:
    """Run the gchat TUI application."""
    if "--version" in sys.argv or "-v" in sys.argv:
        print(f"gogchat-tui {__version__}")
        raise SystemExit(0)
    if "--help" in sys.argv or "-h" in sys.argv:
        print(f"gogchat-tui {__version__} — Terminal UI for Google Chat")
        print()
        print("Usage: gogchat-tui [options]")
        print()
        print("Options:")
        print("  -h, --help     Show this help message and exit")
        print("  -v, --version  Show version and exit")
        raise SystemExit(0)

    from tui.app import ChatApp

    app = ChatApp()
    app.run()


if __name__ == "__main__":
    main()
