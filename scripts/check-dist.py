#!/usr/bin/env python3
"""Check all five editor packages before publishing a tagged release."""

import hashlib
import io
import json
from pathlib import Path
import re
import sys
import xml.etree.ElementTree as ET
import zipfile


def check_distribution(tag, dist):
    if not re.fullmatch(r"v\d+\.\d+\.\d+", tag):
        raise ValueError("release tag must be vMAJOR.MINOR.PATCH")
    version = tag[1:]
    packages = {
        "goland-plugin": "zip",
        "vscode": "vsix",
        "sublime": "sublime-package",
        "vim": "zip",
        "neovim": "zip",
    }
    expected = {f"go-clue-{editor}-{version}.{suffix}" for editor, suffix in packages.items()}
    actual = {path.name for path in dist.iterdir() if path.is_file()}
    if actual != expected | {"SHA256SUMS.txt"}:
        raise ValueError(f"unexpected release files: {sorted(actual ^ (expected | {'SHA256SUMS.txt'}))}")

    checksums = {}
    for line in (dist / "SHA256SUMS.txt").read_text().splitlines():
        digest, name = line.split(maxsplit=1)
        name = name.removeprefix("*").removeprefix("./")
        if name in checksums:
            raise ValueError(f"duplicate checksum: {name}")
        checksums[name] = digest
    if set(checksums) != expected:
        raise ValueError("checksums must cover exactly the five editor packages")

    for editor, suffix in packages.items():
        name = f"go-clue-{editor}-{version}.{suffix}"
        path = dist / name
        if hashlib.sha256(path.read_bytes()).hexdigest() != checksums[name]:
            raise ValueError(f"checksum mismatch: {name}")
        with zipfile.ZipFile(path) as archive:
            if archive.testzip() is not None:
                raise ValueError(f"corrupt archive: {name}")
            members = archive.namelist()
            if any(re.search(r"go[-_]doc|godoc|gotmpls", member, re.IGNORECASE) for member in members):
                raise ValueError(f"old plugin name in archive: {name}")
            if editor == "goland-plugin":
                descriptors = []
                for member in members:
                    if member.endswith(".jar"):
                        with zipfile.ZipFile(io.BytesIO(archive.read(member))) as jar:
                            if "META-INF/plugin.xml" in jar.namelist():
                                descriptors.append(ET.fromstring(jar.read("META-INF/plugin.xml")))
                if len(descriptors) != 1:
                    raise ValueError("expected one GoLand plugin descriptor")
                plugin = descriptors[0]
                if (plugin.findtext("id"), plugin.findtext("name"), plugin.findtext("version")) != (
                    "com.donseba.goclue", "go-clue", version
                ):
                    raise ValueError("incorrect GoLand plugin identity or version")
            elif editor == "vscode":
                plugin = json.loads(archive.read("extension/package.json"))
                if (plugin["publisher"], plugin["name"], plugin["displayName"], plugin["version"]) != (
                    "donseba", "go-clue-vscode", "go-clue", version
                ):
                    raise ValueError("incorrect VS Code plugin identity or version")
            elif editor == "sublime":
                plugin = json.loads(archive.read("sublime-package.json"))
                if (plugin["name"], plugin["version"]) != ("go-clue-sublime", version):
                    raise ValueError("incorrect Sublime Text package identity or version")
            elif editor == "vim":
                archive.getinfo("plugin/go_clue_lsp.vim")
            elif editor == "neovim":
                archive.getinfo("lua/go-clue/init.lua")
        print(f"verified {name}")


if __name__ == "__main__":
    if len(sys.argv) != 2:
        sys.exit("usage: python3 scripts/check-dist.py vMAJOR.MINOR.PATCH")
    check_distribution(sys.argv[1], Path(__file__).resolve().parent.parent / "dist")
