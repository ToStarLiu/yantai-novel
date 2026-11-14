"""Create the verified Windows release archive using only Python's standard library."""
from __future__ import annotations

import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import zipfile

ROOT = Path(__file__).resolve().parents[1]
DIST = ROOT / "desktop" / "dist"
PACKAGE = DIST / "Yantai-win32-x64"


def digest(file: Path) -> str:
    with file.open("rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


def git(*args: str) -> str:
    return subprocess.check_output(["git", *args], cwd=ROOT, text=True).strip()


def private_path(path: Path) -> bool:
    for part in path.parts:
        lower = part.lower()
        if lower in {".local", ".git", "pgdata", "node_modules", "provider-master.key", "login.txt", "model.env", "author-credentials.txt", "config.json", "pg_version", "postmaster.pid", "postmaster.opts"}:
            return True
        if lower == ".env" or lower.startswith(".env."):
            return True
        if lower.endswith((".dump", ".backup", ".pfx", ".p12", ".pem", ".key")):
            return True
    return False


def main() -> None:
    version = json.loads((ROOT / "desktop/package.json").read_text(encoding="utf-8"))["version"]
    if not re.fullmatch(r"\d+\.\d+\.\d+", version):
        raise RuntimeError("Unexpected release version")
    verification = json.loads((DIST / "package-verification.json").read_text(encoding="utf-8"))
    if verification["version"] != version or verification["sourceDirty"]:
        raise RuntimeError("Release archive requires a clean, verified package of this version")
    if verification["sourceTree"] != git("rev-parse", "HEAD^{tree}") or git("status", "--porcelain", "--untracked-files=normal"):
        raise RuntimeError("Commit final release sources before creating the archive")
    if PACKAGE.is_symlink() or PACKAGE.resolve() != DIST.resolve() / "Yantai-win32-x64":
        raise RuntimeError("Unexpected package directory")
    files = []
    for file in sorted(PACKAGE.rglob("*")):
        relative = file.relative_to(PACKAGE)
        if file.is_symlink() or private_path(relative):
            raise RuntimeError("Unexpected private path or link in package: " + relative.as_posix())
        if file.is_file():
            files.append(file)
    if not files:
        raise RuntimeError("Package is empty")
    target = DIST / f"Yantai-v{version}-windows-x64.zip"
    pending = target.with_suffix(".zip.pending")
    if target.exists() or pending.exists():
        raise RuntimeError("Archive already exists; inspect it instead of replacing a release artifact")
    with zipfile.ZipFile(pending, "w", zipfile.ZIP_DEFLATED, compresslevel=6) as archive:
        for file in files:
            archive.write(file, file.relative_to(PACKAGE.parent).as_posix())
    with zipfile.ZipFile(pending) as archive:
        bad = archive.testzip()
        if bad:
            raise RuntimeError("Archive CRC check failed: " + bad)
        if len(archive.infolist()) != len(files):
            raise RuntimeError("Archive inventory mismatch")
    os.replace(pending, target)
    manifest = json.loads((PACKAGE / "resources/backend/build-info.json").read_text(encoding="utf-8"))
    receipt = {
        "application": "Yantai", "version": version, "platform": "Windows x64",
        "archive": target.name, "bytes": target.stat().st_size,
        "sha256": digest(target), "entry": "Yantai-win32-x64/Yantai.exe",
        "schemaVersion": max(int(file.name.split("_", 1)[0]) for file in (ROOT / "migrations").glob("*.up.sql")), "source": manifest["source"],
        "fileCount": len(files), "zipCRCVerified": True,
        "privateWorkspaceDataIncluded": False,
        "buildVerification": verification,
        "license": "GPL-3.0-only; bundled components retain their own licenses",
    }
    receipt_file = DIST / "release.json"
    receipt_file.write_text(json.dumps(receipt, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    (DIST / "SHA256SUMS.txt").write_text(
        f"{receipt['sha256']}  {target.name}\n{digest(receipt_file)}  release.json\n",
        encoding="ascii",
    )
    print(json.dumps(receipt, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
