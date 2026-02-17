# -*- mode: python ; coding: utf-8 -*-
"""PyInstaller spec for building gogchat-tui as a standalone binary."""

from PyInstaller.utils.hooks import collect_submodules

# Collect all submodules to handle lazy/dynamic imports
hiddenimports = (
    collect_submodules("textual")
    + collect_submodules("tui")
)

a = Analysis(
    ["tui/__main__.py"],
    pathex=[],
    binaries=[],
    datas=[("tui/styles.css", ".")],
    hiddenimports=hiddenimports,
    hookspath=[],
    hooksconfig={},
    runtime_hooks=[],
    excludes=[],
    noarchive=False,
)

pyz = PYZ(a.pure)

exe = EXE(
    pyz,
    a.scripts,
    a.binaries,
    a.datas,
    [],
    name="gogchat-tui",
    debug=False,
    bootloader_ignore_signals=False,
    # strip=False: stripping symbols on macOS arm64 can corrupt embedded archive offsets,
    # causing the one-file binary to hang silently on launch.
    strip=False,
    upx=False,
    upx_exclude=[],
    runtime_tmpdir=None,
    console=True,
    disable_windowed_traceback=False,
    argv_emulation=False,
    target_arch=None,
    # codesign_identity=None: skip ad-hoc signing here; CI handles signing after the build.
    # Double-signing (PyInstaller + CI) can invalidate the binary.
    codesign_identity=None,
    entitlements_file=None,
)
