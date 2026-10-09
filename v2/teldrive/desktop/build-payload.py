"""Build the embedded bundle from prepared, licensed Windows components."""
from pathlib import Path
import argparse
import hashlib
import json
import zipfile

parser = argparse.ArgumentParser()
parser.add_argument("--components", type=Path, required=True)
parser.add_argument("--server", type=Path, required=True)
parser.add_argument("--rclone", type=Path, required=True)
parser.add_argument("--output", type=Path, default=Path(__file__).resolve().parent / "Payload.zip")
args = parser.parse_args()
files = {"teldrive.exe": args.server, "rclone.exe": args.rclone}
for name in ("ffmpeg.exe", "postgres.zip", "winfsp-2.1.25156.msi", "WebView2RuntimeInstaller.exe", "VC_redist.x64.exe"):
    files[name] = args.components / name
for directory in ("mpv", "licenses"):
    for path in (args.components / directory).rglob("*"):
        if path.is_file(): files[path.relative_to(args.components).as_posix()] = path
if not any(name.endswith("mpv.exe") for name in files): parser.error("Falta el reproductor mpv.")
if not any(name.startswith("licenses/") for name in files): parser.error("Faltan las licencias de los componentes.")
manifest = {}
for name, path in files.items():
    with path.open("rb") as source: digest = hashlib.file_digest(source, "sha256").hexdigest()
    manifest[name] = {"sha256": digest, "size": path.stat().st_size}
args.output.parent.mkdir(parents=True, exist_ok=True)
with zipfile.ZipFile(args.output, "w", zipfile.ZIP_DEFLATED, compresslevel=6) as package:
    for name, path in files.items(): package.write(path, name)
    package.writestr("components.json", json.dumps(manifest, indent=2))
with zipfile.ZipFile(args.output) as package:
    if package.testzip() is not None: raise RuntimeError("El paquete está dañado.")
print(f"Paquete verificado: {len(files)} componentes, {args.output.stat().st_size} bytes")
